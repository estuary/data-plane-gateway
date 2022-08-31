package main

import (
	context "context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/jamiealquiza/envy"
	pb "go.gazette.dev/core/broker/protocol"
	pc "go.gazette.dev/core/consumer/protocol"
	"go.gazette.dev/core/task"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
)

var (
	brokerAddr         = flag.String("broker-address", "localhost:8080", "Target broker address")
	consumerAddr       = flag.String("consumer-address", "localhost:9000", "Target consumer address")
	corsOrigin         = flag.String("cors-origin", "*", "CORS Origin")
	jwtVerificationKey = flag.String("verification-key", "supersecret", "Key used to verify JWTs signed by the Flow Control Plane")
	port               = flag.String("port", "28318", "Service port for HTTP and gRPC requests. A random port is used if not set. Port may also take the form 'unix:///path/to/socket' to use a Unix Domain Socket")
	zone               = flag.String("zone", "local", "Availability zone within which this process is running")
	tls_cert           = flag.String("tls-certificate", "", "Path to the TLS certificate (.crt) to use.")
	tls_private_key    = flag.String("tls-private-key", "", "The private key for the TLS certificate")
)

var corsConfig *corsSettings

func main() {
	flag.Parse()
	envy.Parse("GATEWAY")
	corsConfig = NewCorsSettings(*corsOrigin)

	pb.RegisterGRPCDispatcher(*zone)

	var listener, err = net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		log.Fatalf("failed to start listener: %v", err)
	}
	// If TLS is enabled, then wrap the above listener in one that handles TLS termination.
	if *tls_cert != "" {
		if *tls_private_key == "" {
			log.Fatalf("must supply --tls-private-key with --tls-certificate")
		}

		crt, err := tls.LoadX509KeyPair(*tls_cert, *tls_private_key)
		if err != nil {
			log.Fatalf("failed to load tls certificate: %v", err)
		}

		var config = tls.Config{
			Certificates: []tls.Certificate{crt},
			// NextProtos is someone's clever name for the list of supported ALPN protocols.
			// We need to explicitly configure support for HTTP2 here, or else it won't be offered.
			NextProtos: []string{"h2", "http/1.1"},
		}
		listener = tls.NewListener(listener, &config)
	} else {
		if *tls_private_key != "" {
			log.Fatalf("must supply --tls-certificate with --tls-private-key")
		}
	}
	var grpcServer = grpc.NewServer(
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	)

	tasks := task.NewGroup(context.Background())
	ctx := pb.WithDispatchDefault(tasks.Context())

	journalServer := NewJournalAuthServer(ctx)
	shardServer := NewShardAuthServer(ctx)
	pb.RegisterJournalServer(grpcServer, journalServer)
	pc.RegisterShardServer(grpcServer, shardServer)

	httpMux := http.NewServeMux()
	restHandler := NewRestServer(ctx, listener.Addr().String())

	httpMux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK\n"))
	}))
	httpMux.Handle("/", restHandler)

	// compose both http and grpc into a single handler, that dispatches each request based on
	// the content-type. It's important that we do this on a per-request basis instead of a
	// per-connection basis because this server may be behind a load balancer, which will maintain
	// persistent connections that get re-used for both protocols. This approach was taken from:
	// https://ahmet.im/blog/grpc-http-mux-go/
	var mixedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("content-type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			httpMux.ServeHTTP(w, r)
		}
	})

	// Whether we're running with TLS or not, we need to use http2 in order for grpc to work.
	// The `h2c.NewHandler` enables us to use http2 without tls.
	var http2Server = http2.Server{}
	var http1Server = &http.Server{Handler: h2c.NewHandler(mixedHandler, &http2Server)}

	log.Printf("Listening on %s\n", listener.Addr().String())

	err = http1Server.Serve(listener)
	if err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	log.Println("goodbye")
	os.Exit(0)
}
