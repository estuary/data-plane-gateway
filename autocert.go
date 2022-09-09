package main

import (
	context "context"
	"crypto/tls"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"golang.org/x/crypto/acme/autocert"
)

func NewCertProvider(hostname string, etcdUrls []string, certificateEmail string) (*CertProvider, error) {
	var etcdClient, err = etcd.New(etcd.Config{
		Endpoints: etcdUrls,
	})
	if err != nil {
		return nil, fmt.Errorf("creating etcd client: %w", err)
	}
	return &CertProvider{
		hostname: hostname,
		acManager: autocert.Manager{
			Prompt: autocert.AcceptTOS,
			Cache: &etcdCache{
				client: etcdClient,
				prefix: etcdCachePrefix(hostname),
			},
			HostPolicy:  autocert.HostWhitelist(hostname),
			Email:       certificateEmail,
			RenewBefore: CertRenewBefore,
		},
		etcdClient: etcdClient,
	}, nil
}

type CertProvider struct {
	hostname    string
	acManager   autocert.Manager
	etcdClient  *etcd.Client
	certPointer atomic.Pointer[tls.Certificate]
}

// TLSConfig returns a `*tls.Config` that can be used to create a tls listener from a plain tcp
// listener. It uses the `GetCertificate` callback to return a certificate that gets provisioned and
// renewed automatically.
func (p *CertProvider) TLSConfig() *tls.Config {
	return &tls.Config{
		// We don't use acManager.TLSConfig() because that function adds the ACME challenge protocol
		// to NextProtos, and that challenge method won't work for us.
		NextProtos:     []string{"h2", "http/1.1"},
		GetCertificate: p.acManager.GetCertificate,
	}
}

// GetCertificate implements the function signature that's required for use as
// `tls.Config.GetCertificate`. It keeps a local cached copy of the certificate, and delegates to
// `autocert.Manager.GetCertificate` as needed to acquire or renew the cached one. It acquires a
// lock on a distributed mutex in ETCD before doing any delegation to autocert. It does so because:
//
//   - We want to avoid multiple raced attempts to acquire certificates by mutliple data-plane-gateway
//     processes, and even after a brief read of the autocert source code, I'm not convinced that
//     those processes couldn't interfere with each others' attempts.
//
//   - The autocert API doesn't allow the lock to be any more fine grained, such as to allow only
//     locking if there isn't already a cached cert in etcd.
//
//     That said, I'm also not entirely convinced that all this weird crap is necessary, so if you're
//     here to rip it out because you know more about autocert than I do, please have at it.
func (p *CertProvider) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	var cachedCert = p.getCachedCert()
	if cachedCert != nil {
		return cachedCert, nil
	}
	log.Printf("no locally cached TLS cert")

	var ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	var session, err = concurrency.NewSession(p.etcdClient, concurrency.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("creating etcd session: %w", err)
	}
	var mutex = concurrency.NewMutex(session, p.etcdMutexName())
	if err = mutex.Lock(ctx); err != nil {
		return nil, fmt.Errorf("locking etcd mutex: %w", err)
	}
	defer mutex.Unlock(ctx)

	// It's possible that another thread was holding the mutex and acquiring a cert while our call
	// to lock the mutex was blocked, so check the local cache again, just in case we can avoid
	// another network round trip.
	cachedCert = p.getCachedCert()
	if cachedCert != nil {
		return cachedCert, nil
	}
	// Now it's time to ask autocert for the certificate. This may cause it to start acquiring a
	// new certificate, or it may return a cached one from ETCD.

	// The autocert package was designed to be usable in situations where a single server manages
	// connections for a variety of hostnames, such as with a k8s ingress reverse proxy. The
	// hostname is taken from the `server_name` (SNI) of the ClientHello message, and autocert will
	// try to provision a certificate for each distinct `server_name`. A side effect of that is
	// that, at least in our case, no certificate can be generated in cases where the client either
	// connects using the IP address instead of the hostname, or doesn't use SNI at all. It's pretty
	// rare, in practice, for a client not to support SNI, but we _do_ have at least one important
	// case where a client connects using an IP address: when the REST server proxies to the GRPC
	// server (a whole separate can of worms, that one). In any case, we certainly don't need to
	// support generating certificates for multiple domains, and we don't really have any reason to
	// prevent clients from connecting using our (in practice, static) IP address, so we patch in
	// our configured hostname onto every ClientHello before handing it off to autocert. This is
	// also consistent with the behavior of cacheing only a single certificate instead of one per
	// domain.
	hello.ServerName = p.hostname
	certificate, err := p.acManager.GetCertificate(hello)
	if err != nil {
		return nil, fmt.Errorf("acquiring TLS certificate: %w", err)
	}
	log.Printf("successfully obtained TLS certificate")
	p.certPointer.Store(certificate)

	return certificate, nil
}

func (p *CertProvider) getCachedCert() *tls.Certificate {
	var cert = p.certPointer.Load()
	if cert != nil {
		var invalidateAt = cert.Leaf.NotAfter.Add(-CertRenewBefore - CertRenewBuffer)
		if !time.Now().After(invalidateAt) {
			return cert
		} else {
			// We don't actually set certPointer to nil to avoid a race with another goroutine that
			// may be updating the cached cert after renewing it.
			log.Printf("ignoring locally cached certificate because it should be renewed by now")
		}
	}
	return nil
}

// Implements the autocert.Cache interface, which is used for persistence of both the TLS
// certificate and any intermediate data that's used during the acquisition and renewal of said
// certificate.
type etcdCache struct {
	client *etcd.Client
	prefix string
}

func (c *etcdCache) fullKey(part string) string {
	return c.prefix + part
}

// Get implements the autocert.Cache interface
func (c *etcdCache) Get(ctx context.Context, key string) ([]byte, error) {
	var fullKey = c.fullKey(key)
	var resp, err = c.client.Get(ctx, fullKey)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		log.Printf("cache miss for key: '%s'", fullKey)
		return nil, autocert.ErrCacheMiss
	} else {
		log.Printf("cache hit for key: '%s', modRevision: %d", fullKey, resp.Kvs[0].ModRevision)
		return resp.Kvs[0].Value, nil
	}
}

// Put implements the autocert.Cache interface
func (c *etcdCache) Put(ctx context.Context, key string, data []byte) error {
	var fullKey = c.fullKey(key)
	var _, err = c.client.Put(ctx, fullKey, string(data))
	if err == nil {
		log.Printf("sucessfully put etcd cache key: '%s'", fullKey)
	}
	return err
}

// Put implements the autocert.Cache interface
func (c *etcdCache) Delete(ctx context.Context, key string) error {
	var fullKey = c.fullKey(key)
	var resp, err = c.client.Delete(ctx, fullKey)
	if err != nil {
		return err
	}
	if resp.Deleted == 1 {
		log.Printf("sucessfully deleted etcd cache key: '%s'", fullKey)
	} else {
		log.Printf("deletion of etcd cache key: '%s' was a no-op (key not present)", fullKey)
	}
	return nil
}

func (p *CertProvider) etcdMutexName() string {
	return fmt.Sprintf("%s/%s/mutex/", BaseEtcdPrefix, p.hostname)
}

func etcdCachePrefix(hostname string) string {
	return fmt.Sprintf("%s/%s/cache/", BaseEtcdPrefix, hostname)
}

const BaseEtcdPrefix = "/data-plane-gateway"

// Determines when to start trying to renew the certificate. This duration is subtracted from the
// expiration datetime of the certificate, and the result is roughly when autocert will attempt to
// renew it.
const CertRenewBefore = 30 * 24 * time.Hour

// Certificate renewal is a little tricky because of the extra layer of cacheing we have going on,
// so this time buffer is used to augment CertRenewBefore in order to determine when to invalidate
// our cached certificate. We don't want to invalidate our cache before the renewal is complete
// because we need to lock an expensive distributed mutex in order to even attempt to get the new
// certificate from the `autocert.Manager`. So we wait this amount of time _after_ autocert is
// supposed to try the renewal before we start asking it for the new certificate.
const CertRenewBuffer = time.Hour
