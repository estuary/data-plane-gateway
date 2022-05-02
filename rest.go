package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"syscall"
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

func NewRestServer() *GracefulServer {
	var ctx context.Context = context.Background()
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

	err = registerJournalService(ctx, mux, *brokerAddr)
	if err != nil {
		log.Fatalf("Failed to initialize rest gateway: %v", err)
	}

	err = registerShardService(ctx, mux, *consumerAddr)
	if err != nil {
		log.Fatalf("Failed to initialize rest gateway: %v", err)
	}

	n := negroni.Classic()
	n.Use(negroni.HandlerFunc(cors))
	n.UseHandler(mux)

	log.Printf("Listening: %s\n", *gatewayAddr)
	log.Printf("Connecting to broker: %s\n", *brokerAddr)
	log.Printf("Connecting to consumer: %s\n", *consumerAddr)

	return NewGracefulServer(n)
}

func cors(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
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

type GracefulServer struct {
	Server   *http.Server
	shutdown chan bool
}

func NewGracefulServer(handler http.Handler) *GracefulServer {
	return &GracefulServer{
		Server:   &http.Server{Handler: handler},
		shutdown: make(chan bool, 0),
	}
}

func (s *GracefulServer) Serve(addr string) error {
	listener, err := listen(addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	err = s.Server.Serve(listener)
	if err == http.ErrServerClosed {
		// This is expected, not really an error.
	} else if err != nil {
		return err
	}

	// Wait for shutdown to complete
	<-s.shutdown

	return nil
}

func (s *GracefulServer) TrapShutdownSignal(timeout time.Duration) error {
	var signaled = make(chan os.Signal, 1)
	signal.Notify(signaled, syscall.SIGTERM, syscall.SIGINT)
	<-signaled

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := s.Server.Shutdown(ctx)
	if err != nil {
		return err
	}

	// Signal that we've finished shutting down
	close(s.shutdown)

	return nil
}
