package main

import (
	context "context"
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"github.com/ReneKroon/ttlcache"
	etcd "go.etcd.io/etcd/client/v3"
	"golang.org/x/crypto/acme/autocert"
)

func NewCertProvider(hostname string, etcdUrl string, certificateEmail string, certRenewBefore time.Duration) (*CertProvider, error) {
	var etcdClient, err = etcd.NewFromURL(etcdUrl)
	if err != nil {
		return nil, fmt.Errorf("creating etcd client: %w", err)
	}
	if err = etcdClient.Sync(context.Background()); err != nil {
		return nil, fmt.Errorf("syncing etcd members: %w", err)
	}
	return &CertProvider{
		hostname: hostname,
		acManager: autocert.Manager{
			Prompt:      autocert.AcceptTOS,
			Cache:       newEtcdCache(hostname, etcdClient),
			HostPolicy:  autocert.HostWhitelist(hostname),
			Email:       certificateEmail,
			RenewBefore: certRenewBefore,
		},
	}, nil
}

type CertProvider struct {
	hostname  string
	acManager autocert.Manager
}

// TLSConfig returns a `*tls.Config` that can be used to create a tls listener from a plain tcp
// listener. It uses the `GetCertificate` callback to return a certificate that gets provisioned and
// renewed automatically.
func (p *CertProvider) TLSConfig() *tls.Config {
	return &tls.Config{
		// We don't use acManager.TLSConfig() because that function adds the ACME challenge protocol
		// to NextProtos, and that challenge method won't work for us.
		NextProtos:     []string{"h2", "http/1.1"},
		GetCertificate: p.GetCertificate,
	}
}

// GetCertificate implements the function signature that's required for use as
// `tls.Config.GetCertificate`. It wraps autocert.Manager.GetCertificate in order to hard-code the
// hostname that's used to get the certificate.
//
// The autocert package was designed to be usable in situations where a single server manages
// connections for a variety of hostnames, such as with a k8s ingress reverse proxy. The hostname is
// taken from the `server_name` (SNI) of the ClientHello message, and autocert will try to provision
// a certificate for each distinct `server_name`. A side effect of that is that, at least in our
// case, no certificate can be generated in cases where the client either connects using the IP
// address instead of the hostname, or doesn't use SNI at all. It's pretty rare, in practice, for a
// client not to support SNI, but we _do_ have at least one important case where a client connects
// using an IP address: when the REST server proxies to the GRPC server (a whole separate can of
// worms, that one). In any case, we certainly don't need to support generating certificates for
// multiple domains, and we don't really have any reason to prevent clients from connecting using
// our (in practice, static) IP address, so we patch in our configured hostname onto every
// ClientHello before handing it off to autocert.
func (p *CertProvider) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	hello.ServerName = p.hostname
	return p.acManager.GetCertificate(hello)
}

// Implements the autocert.Cache interface, which is used for persistence of both the TLS
// certificate and any intermediate data that's used during the acquisition and renewal of said
// certificate.
type etcdCache struct {
	localCache *ttlcache.Cache
	client     *etcd.Client
	prefix     string
}

func newEtcdCache(hostname string, client *etcd.Client) *etcdCache {
	var localCache = ttlcache.NewCache()
	localCache.SetTTL(15 * time.Minute)
	localCache.SkipTtlExtensionOnHit(true)

	return &etcdCache{
		localCache: localCache,
		client:     client,
		prefix:     fmt.Sprintf("/data-plane-gateway/%s/cache/", hostname),
	}
}

func (c *etcdCache) fullKey(part string) string {
	return c.prefix + part
}

// Get implements the autocert.Cache interface
func (c *etcdCache) Get(ctx context.Context, key string) ([]byte, error) {
	if local, ok := c.localCache.Get(key); ok {
		return local.([]byte), nil
	}

	var fullKey = c.fullKey(key)
	var resp, err = c.client.Get(ctx, fullKey)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		log.Printf("cache miss for key: '%s'", fullKey)
		return nil, autocert.ErrCacheMiss
	} else {
		log.Printf("etcd cache hit for key: '%s', modRevision: %d", fullKey, resp.Kvs[0].ModRevision)
		c.localCache.Set(key, resp.Kvs[0].Value)
		return resp.Kvs[0].Value, nil
	}
}

// Put implements the autocert.Cache interface
func (c *etcdCache) Put(ctx context.Context, key string, data []byte) error {
	var fullKey = c.fullKey(key)
	var _, err = c.client.Put(ctx, fullKey, string(data))
	if err == nil {
		log.Printf("sucessfully put etcd cache key: '%s'", fullKey)
		c.localCache.Set(key, data)
	}
	return err
}

// Put implements the autocert.Cache interface
func (c *etcdCache) Delete(ctx context.Context, key string) error {
	c.localCache.Remove(key)
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
