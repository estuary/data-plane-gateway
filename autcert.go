package main

import (
	context "context"
	"crypto/tls"
	"fmt"
	"log"

	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"golang.org/x/crypto/acme/autocert"
)

func InitAutocert(hostname string, etcdUrls []string, certificateEmail string) (*CertProvider, error) {
	var etcdClient, err = etcd.New(etcd.Config{
		Endpoints: etcdUrls,
	})
	if err != nil {
		return nil, fmt.Errorf("creating etcd client: %w", err)
	}

	var cache = &EtcdCache{
		client: etcdClient,
		prefix: etcdCachePrefix(hostname),
	}

	var manager = autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      cache,
		HostPolicy: autocert.HostWhitelist(hostname),
		Email:      certificateEmail,
	}

	return &CertProvider{
		hostname:   hostname,
		acManager:  manager,
		etcdClient: etcdClient,
	}, nil
}

type CertProvider struct {
	hostname   string
	acManager  autocert.Manager
	etcdClient *etcd.Client
	cert       *tls.Certificate
}

const BaseEtcdPrefix = "/data-plane-gateway"

func (p *CertProvider) etcdMutexName() string {
	return fmt.Sprintf("%s/%s/mutex/", BaseEtcdPrefix, p.hostname)
}

func etcdCachePrefix(hostname string) string {
	return fmt.Sprintf("%s/%s/cache/", BaseEtcdPrefix, hostname)
}

func (p *CertProvider) TLSConfig() *tls.Config {
	return &tls.Config{
		// We don't use acManager.TLSConfig() because that function adds the ACME challenge protocol
		// to NextProtos, and that challenge method won't work for us.
		NextProtos:     []string{"h2", "http/1.1"},
		GetCertificate: p.acManager.GetCertificate,
	}
}

func (p *CertProvider) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if p.cert != nil {
		return p.cert, nil
	}

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
	// to lock the mutex was blocked, so check the cert again.
	if p.cert != nil {
		return p.cert, nil
	}

	certificate, err := p.acManager.GetCertificate(hello)
	if err != nil {
		return nil, fmt.Errorf("acquiring TLS certificate: %w", err)
	}
	p.cert = certificate

	return certificate, nil
}

type EtcdCache struct {
	client *etcd.Client
	prefix string
}

func (c *EtcdCache) fullKey(part string) string {
	return c.prefix + part
}

// Get implements the autocert.Cache interface
func (c *EtcdCache) Get(ctx context.Context, key string) ([]byte, error) {
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
func (c *EtcdCache) Put(ctx context.Context, key string, data []byte) error {
	var fullKey = c.fullKey(key)
	var _, err = c.client.Put(ctx, fullKey, string(data))
	if err == nil {
		log.Printf("sucessfully put etcd cache key: '%s'", fullKey)
	}
	return err
}

// Put implements the autocert.Cache interface
func (c *EtcdCache) Delete(ctx context.Context, key string) error {
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
