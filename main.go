package main

import (
	context "context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"strings"

	"github.com/estuary/data-plane-gateway/proxy"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/jamiealquiza/envy"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	pb "go.gazette.dev/core/broker/protocol"
	pc "go.gazette.dev/core/consumer/protocol"
	"go.gazette.dev/core/task"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
)

var (
	logLevel           = flag.String("log.level", "info", "Verbosity of logging")
	brokerAddr         = flag.String("broker-address", "localhost:8080", "Target broker address")
	consumerAddr       = flag.String("consumer-address", "localhost:9000", "Target consumer address")
	inferenceAddr      = flag.String("inference-address", "localhost:9090", "Target schema inference service address")
	corsOrigin         = flag.String("cors-origin", "*", "CORS Origin")
	jwtVerificationKey = flag.String("verification-key", "supersecret", "Key used to verify JWTs signed by the Flow Control Plane")
	// Plain port is meant to be exposed to the public internet. It serves the REST endpoints, so that
	// it's usable for local development without shenanigans for dealing with the self-signed cert.
	// It also serves the ACME challenges for provisioning TLS certs. It does not serve gRPC.
	plainPort = flag.String("plain-port", "28317", "Port for unencrypted communication")
	// TLS port serves the REST endpoints and gRPC. The bread and butter, if you will.
	tlsPort = flag.String("port", "28318", "Service port for HTTPS and gRPC requests. Port may also take the form 'unix:///path/to/socket' to use a Unix Domain Socket")
	// We listen on 3 separate ports because the "plain-port" needs to be exposed to the public internet, and we
	// don't want to serve metrics or debug endpoints to just anyone. This port serves metrics and debug
	// endpoints only. It is not intended to ever be exposed to the public internet.
	debugPort = flag.String("debug-port", "28316", "Port for serving metrics and debug endpoints")
	zone      = flag.String("zone", "local", "Availability zone within which this process is running")

	hostname = flag.String("hostname", "localhost", "The hostname that clients use to connect to the gateway")

	// Args for providing the tls certificate the old fashioned way
	tls_cert        = flag.String("tls-certificate", "", "Path to the TLS certificate (.crt) to use.")
	tls_private_key = flag.String("tls-private-key", "", "The private key for the TLS certificate")
)

var corsConfig *corsSettings

func main() {
	flag.Parse()
	envy.Parse("GATEWAY")
	var lvl, err = log.ParseLevel(*logLevel)
	if err != nil {
		panic(fmt.Sprintf("failed to parse log level: '%s', %v", *logLevel, err))
	}
	log.SetLevel(lvl)
	log.SetFormatter(&log.JSONFormatter{})

	grpc.EnableTracing = true
	grpc_prometheus.EnableHandlingTimeHistogram()
	grpc_prometheus.EnableClientHandlingTimeHistogram()

	if *tls_cert == "" {
		log.Fatal("TLS is required in order to run data-plane-gateway. Missing TLS arguments")
	}
	if *tls_private_key == "" {
		log.Fatal("must supply --tls-private-key with --tls-certificate")
	}

	crt, err := tls.LoadX509KeyPair(*tls_cert, *tls_private_key)
	if err != nil {
		log.Fatalf("failed to load tls certificate: %v", err)
	}
	log.Info("loaded tls certificate")
	var certificates = []tls.Certificate{crt}

	tlsPortNum, err := strconv.ParseUint(*tlsPort, 10, 16)
	if err != nil {
		log.Fatalf("invalid tls port number: '%s': %w", *tlsPort, err)
	}

	// TODO: this sets a global that's used in rest.go :( Can we fix that?
	corsConfig = NewCorsSettings(*corsOrigin)

	pb.RegisterGRPCDispatcher(*zone)
	var grpcServer = grpc.NewServer(
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	)
	grpc_prometheus.Register(grpcServer)

	ctx := pb.WithDispatchDefault(context.Background())
	var tasks = task.NewGroup(ctx)

	journalServer := NewJournalAuthServer(ctx, []byte(*jwtVerificationKey))
	shardServer := NewShardAuthServer(ctx, []byte(*jwtVerificationKey))
	pb.RegisterJournalServer(grpcServer, journalServer)
	pc.RegisterShardServer(grpcServer, shardServer)

	// Will be used with all listeners.
	var healthHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK\n"))
	})

	restHandler := NewRestServer(ctx, fmt.Sprintf("localhost:%s", *tlsPort))
	schemaInferenceHandler := NewSchemaInferenceServer(ctx)

	// These routes will be exposed to the public internet and used for handling both http and https requests.
	publicMux := http.NewServeMux()
	publicMux.Handle("/healthz", healthHandler)
	publicMux.Handle("/infer_schema", schemaInferenceHandler)
	publicMux.Handle("/", restHandler)

	proxyServer, tappedListener, err := proxy.NewTlsProxyServer(*hostname, uint16(tlsPortNum), certificates, shardServer.shardClient, []byte(*jwtVerificationKey))

	// compose both http and grpc into a single handler, that dispatches each request based on
	// the content-type. It's important that we do this on a per-request basis instead of a
	// per-connection basis because this server may be behind a load balancer, which will maintain
	// persistent connections that get re-used for both protocols. This approach was taken from:
	// https://ahmet.im/blog/grpc-http-mux-go/
	var mixedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("content-type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			publicMux.ServeHTTP(w, r)
		}
	})

	// These routes will be exposed on an internal network port
	debugMux := http.NewServeMux()
	debugMux.Handle("/metrics", promhttp.Handler())
	debugMux.Handle("/healthz", healthHandler)
	debugMux.HandleFunc("/debug/pprof/", pprof.Index)
	debugMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	debugMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	debugMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	debugMux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	var debugServer = &http.Server{
		Handler: debugMux,
		Addr:    fmt.Sprintf(":%s", *debugPort),
	}

	tasks.Queue("http server", func() error {
		log.WithField("port", plainPort).Info("started HTTP server")
		log.Printf("Started HTTP server")
		var http2Server = &http2.Server{}

		// Use h2c with the plain listener to allow grpc to work without https.
		// This is at best completely unnecessary in production, but it helps remove friction for local development
		// because it allows configuring a single gateway URL in control-plane that work for both the UI, which
		// cannot easily trust self-signed certs, and for flowctl, which must use gRPC (h2).
		var handler = h2c.NewHandler(mixedHandler, http2Server)
		return http.ListenAndServe(fmt.Sprintf(":%s", *plainPort), handler)
	})

	tasks.Queue("proxy server", func() error {
		log.WithField("port", tlsPort).Info("started TLS proxy server")
		return proxyServer.Run()
	})
	tasks.Queue("https server", func() error {
		log.WithField("port", tlsPort).Info("started HTTPS/GRPC server")
		return http.Serve(tappedListener, mixedHandler)
	})
	tasks.Queue("debug server", func() error {
		log.WithField("port", debugPort).Info("started debug server")
		return debugServer.ListenAndServe()
	})

	tasks.GoRun()

	log.Printf("Listening on %s\n", tappedListener.Addr().String())

	err = tasks.Wait()
	if err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	log.Println("goodbye")
	os.Exit(0)
}
