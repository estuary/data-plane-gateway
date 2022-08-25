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

	"github.com/jamiealquiza/envy"
	pb "go.gazette.dev/core/broker/protocol"
	pc "go.gazette.dev/core/consumer/protocol"
	"go.gazette.dev/core/server"
	"go.gazette.dev/core/task"
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

	srv, err := server.NewFromListener(listener)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	tasks := task.NewGroup(context.Background())
	ctx := pb.WithDispatchDefault(tasks.Context())
	journalServer := NewJournalAuthServer(ctx)
	shardServer := NewShardAuthServer(ctx)
	restServer := NewRestServer(ctx, srv.Endpoint().GRPCAddr())

	pb.RegisterJournalServer(srv.GRPCServer, journalServer)
	pc.RegisterShardServer(srv.GRPCServer, shardServer)

	srv.HTTPMux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK\n"))
	}))
	srv.HTTPMux.Handle("/", restServer)

	srv.QueueTasks(tasks)
	tasks.GoRun()

	log.Printf("Listening on %s\n", srv.Endpoint().URL())

	if err := tasks.Wait(); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	log.Println("goodbye")
	os.Exit(0)
}
