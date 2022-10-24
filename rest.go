package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gogo/gateway"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/urfave/negroni"
	_ "go.gazette.dev/core/broker/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	bgw "github.com/estuary/data-plane-gateway/gen/broker/protocol"
	cgw "github.com/estuary/data-plane-gateway/gen/consumer/protocol"
)

// NewRestServer  returns an http.Handler that proxies REST requests to broker and consumer gRPC
// services. The `gatewayAddr` parameter, surprisingly, is what it proxies _to_. It establishes a
// connection to `gatewayAddr` immediately, because grpc-gateway requires an established connection
// before it can register the handlers. There's no good reason for that. It's just how it do. So
// because grpc-gateway requires an established connection, we proxy to _ourselves_ instead of the
// actual broker and consumer endpoints, so that we are able to start this thing even when the
// broker or consumer are unavailable. If you know of a way to avoid this nonsense and lazily
// connect to the broker/consumer endpoints, then please feel free to implement that and delete this
// nauseating comment.
func NewRestServer(ctx context.Context, gatewayAddr string) http.Handler {
	var err error

	jsonpb := &gateway.JSONPb{
		EmitDefaults: true,
		Indent:       "",
		OrigName:     false,
	}
	var mux *runtime.ServeMux = runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, jsonpb),
		runtime.WithProtoErrorHandler(runtime.DefaultHTTPProtoErrorHandler),
	)

	// Since we dial ourselves on the loopback address, the hostname won't match the TLS cert.
	opts := []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	}))}

	err = bgw.RegisterJournalHandlerFromEndpoint(ctx, mux, gatewayAddr, opts)
	if err != nil {
		log.Fatalf("Failed to initialize journal rest gateway: %v", err)
	}

	err = cgw.RegisterShardHandlerFromEndpoint(ctx, mux, gatewayAddr, opts)
	if err != nil {
		log.Fatalf("Failed to initialize shard rest gateway: %v", err)
	}

	n := negroni.Classic()
	n.Use(negroni.HandlerFunc(cors))
	n.UseHandler(mux)

	return n
}

func cors(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if corsConfig.IsAllowed(r.Header.Get("Origin")) {
		rw.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		rw.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		rw.Header().Set("Access-Control-Allow-Headers", "Cache-Control, Content-Language, Content-Length, Content-Type, Expires, Last-Modified, Pragma, Authorization")
	}

	if r.Method == "OPTIONS" {
		return
	}

	next(rw, r)
}

type corsSettings struct {
	allowedOrigins []string
}

func NewCorsSettings(rawOriginFlag string) *corsSettings {
	return &corsSettings{
		allowedOrigins: strings.Split(rawOriginFlag, ","),
	}
}

func (c *corsSettings) IsAllowed(origin string) bool {
	if c.allowWildcard() {
		return true
	}

	for _, allowed := range c.allowedOrigins {
		if matched, _ := regexp.MatchString(allowed, origin); matched {
			return true
		}
	}

	return false
}

func (c *corsSettings) allowWildcard() bool {
	return len(c.allowedOrigins) == 1 && c.allowedOrigins[0] == "*"
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}