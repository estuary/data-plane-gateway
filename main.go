package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/gogo/gateway"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/urfave/negroni"
	_ "go.gazette.dev/core/broker/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	bgw "github.com/estuary/data-plane-gateway/gen/broker/protocol"
	cgw "github.com/estuary/data-plane-gateway/gen/consumer/protocol"
)

var (
	brokerAddr   = flag.String("broker-address", "localhost:8080", "Target broker address")
	consumerAddr = flag.String("consumer-address", "localhost:9000", "Target consumer address")
	corsOrigin   = flag.String("cors-origin", "*", "CORS Origin")
	gatewayAddr  = flag.String("gateway-address", "localhost:8081", "Gateway address")
)

func main() {
	flag.Parse()
	var ctx context.Context = context.Background()
	var err error
	var listener net.Listener

	jsonpb := &gateway.JSONPb{
		EmitDefaults: true,
		Indent:       "",
		OrigName:     false,
	}
	var mux *runtime.ServeMux = runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, jsonpb),
		runtime.WithProtoErrorHandler(runtime.DefaultHTTPProtoErrorHandler),
	)

	err = registerJournalService(ctx, mux, *brokerAddr)
	if err != nil {
		log.Fatalf("Failed to initialize rest gateway: %v", err)
	}

	err = registerShardService(ctx, mux, *consumerAddr)
	if err != nil {
		log.Fatalf("Failed to initialize rest gateway: %v", err)
	}

	n := negroni.Classic()
	n.Use(negroni.HandlerFunc(Cors))
	n.UseHandler(mux)

	log.Printf("Listening: %s\n", *gatewayAddr)
	log.Printf("Connecting to broker: %s\n", *brokerAddr)
	log.Printf("Connecting to consumer: %s\n", *consumerAddr)

	listener, err = listen(*gatewayAddr)
	if err != nil {
		log.Fatalf("Failed to listen for rest gateway: %v", err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			log.Fatalf("Failed to close gateway listener %s: %v", *gatewayAddr, err)
		}
	}()

	// Start HTTP server (and proxy calls to gRPC server endpoint)
	err = http.Serve(listener, n)
	if err != nil {
		log.Fatalf("Failed to serve rest gateway: %v", err)
	}
}

func Cors(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if allowedOrigin(r.Header.Get("Origin")) {
		rw.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		rw.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		rw.Header().Set("Access-Control-Allow-Headers", "Cache-Control, Content-Language, Content-Length, Content-Type, Expires, Last-Modified, Pragma, Authorization")
	}

	if r.Method == "OPTIONS" {
		return
	}

	next(rw, r)
}

func allowedOrigin(origin string) bool {
	if *corsOrigin == "*" {
		return true
	}
	matched, _ := regexp.MatchString(*corsOrigin, origin)
	return matched
}

func registerJournalService(ctx context.Context, mux *runtime.ServeMux, addr string) error {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if url, err := url.Parse(addr); err != nil {
		return err
	} else if url.Scheme == "unix" {
		opts = append(opts, grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
		return bgw.RegisterJournalHandlerFromEndpoint(ctx, mux, url.Path, opts)
	} else {
		return bgw.RegisterJournalHandlerFromEndpoint(ctx, mux, url.String(), opts)
	}
}

func registerShardService(ctx context.Context, mux *runtime.ServeMux, addr string) error {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if url, err := url.Parse(addr); err != nil {
		return err
	} else if url.Scheme == "unix" {
		opts = append(opts, grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
		return cgw.RegisterShardHandlerFromEndpoint(ctx, mux, url.Path, opts)
	} else {
		return cgw.RegisterShardHandlerFromEndpoint(ctx, mux, url.String(), opts)
	}
}

func listen(addr string) (net.Listener, error) {
	if url, err := url.Parse(addr); err != nil {
		return nil, err
	} else if url.Scheme == "unix" {
		return net.Listen("unix", url.Path)
	} else {
		return net.Listen("tcp", url.String())
	}
}
