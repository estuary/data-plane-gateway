package main

import (
	context "context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/jamiealquiza/envy"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	pb "go.gazette.dev/core/broker/protocol"
	pc "go.gazette.dev/core/consumer/protocol"
	"go.gazette.dev/core/task"
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

	// Args for providing the tls certificate the old fashioned way
	tls_cert        = flag.String("tls-certificate", "", "Path to the TLS certificate (.crt) to use.")
	tls_private_key = flag.String("tls-private-key", "", "The private key for the TLS certificate")

	// Args that are required for automatically acquiring TLS certificates
	autoAcquireCert      = flag.String("auto-tls-cert", "", "Automatically acquire TLS certificate from Let's Encrypt for the given domain using the ACME protocol")
	etcdEndpoint         = flag.String("etcd-endpoint", "localhost:2379", "ETCD URL to connect to for managing TLS certificates. Only used when auto-tls-cert argument is provided")
	autoAcquireCertEmail = flag.String("tls-cert-email", "", "email address to associate with the automatically acquired TLS certificate")
	autoRenewCertBefore  = flag.Int("tls-renew-before-days", 30, "attempt to renew the certificate this many days before it expires")
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
	corsConfig = NewCorsSettings(*corsOrigin)

	pb.RegisterGRPCDispatcher(*zone)
	var grpcServer = grpc.NewServer(
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	)
	grpc_prometheus.Register(grpcServer)

	ctx := pb.WithDispatchDefault(context.Background())
	var tasks = task.NewGroup(ctx)

	journalServer := NewJournalAuthServer(ctx)
	shardServer := NewShardAuthServer(ctx)
	pb.RegisterJournalServer(grpcServer, journalServer)
	pc.RegisterShardServer(grpcServer, shardServer)

	// Will be used with all listeners.
	var healthHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK\n"))
	})

	restHandler := NewRestServer(ctx, fmt.Sprintf("localhost:%s", *tlsPort))
	schemaInferenceHandler := NewSchemaInferenceServer(ctx)

	plainMux := http.NewServeMux()
	plainMux.Handle("/healthz", healthHandler)
	plainMux.Handle("/infer_schema", schemaInferenceHandler)
	plainMux.Handle("/", restHandler)

	httpsMux := http.NewServeMux()
	httpsMux.Handle("/healthz", healthHandler)
	httpsMux.Handle("/infer_schema", schemaInferenceHandler)
	httpsMux.Handle("/", restHandler)

	debugMux := http.NewServeMux()
	debugMux.Handle("/metrics", promhttp.Handler())
	debugMux.Handle("/healthz", healthHandler)
	debugMux.HandleFunc("/debug/pprof/", pprof.Index)
	debugMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	debugMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	debugMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	debugMux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// compose both http and grpc into a single handler, that dispatches each request based on
	// the content-type. It's important that we do this on a per-request basis instead of a
	// per-connection basis because this server may be behind a load balancer, which will maintain
	// persistent connections that get re-used for both protocols. This approach was taken from:
	// https://ahmet.im/blog/grpc-http-mux-go/
	var mixedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("content-type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			httpsMux.ServeHTTP(w, r)
		}
	})

	var plainServer = &http.Server{
		Handler: plainMux,
		Addr:    fmt.Sprintf(":%s", *plainPort),
	}
	var httpsServer = &http.Server{
		Handler: mixedHandler,
	}
	var debugServer = &http.Server{
		Handler: debugMux,
		Addr:    fmt.Sprintf(":%s", *debugPort),
	}

	// This plain listener will be wrapped in one that does TLS termination. The implementation will
	// depend on whether the TLS cert was provided directly versus us provisioning it automatically.
	tlsListener, err := net.Listen("tcp", fmt.Sprintf(":%s", *tlsPort))
	if err != nil {
		log.Fatalf("failed to start listener: %v", err)
	}

	if *tls_cert != "" {
		if *tls_private_key == "" {
			log.Fatalf("must supply --tls-private-key with --tls-certificate")
		}
		log.Info("using TLS with provided certificate and key")

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
		tlsListener = tls.NewListener(tlsListener, &config)
	} else if *autoAcquireCert != "" {
		if *autoRenewCertBefore < 0 {
			log.Fatalf("--tls-renew-before-days must not be negative")
		}
		log.Info("using autocert to provision TLS certificates")
		var certRenewBefore = 24 * time.Hour * time.Duration(*autoRenewCertBefore)
		certProvider, err := NewCertProvider(*autoAcquireCert, *etcdEndpoint, *autoAcquireCertEmail, certRenewBefore)
		if err != nil {
			log.Fatalf("initializing autocert: %v", err)
		}
		tlsListener = tls.NewListener(tlsListener, certProvider.TLSConfig())
		// Plain http will respond to ACME http-01 challenges and health checks.
		plainServer.Handler = certProvider.acManager.HTTPHandler(plainMux)
	} else {
		panic("TLS is required in order to run data-plane-gateway. Missing TLS arguments")
	}

	tasks.Queue("http server", func() error {
		log.WithField("port", plainPort).Info("started HTTP server")
		log.Printf("Started HTTP server")
		return plainServer.ListenAndServe()
	})
	tasks.Queue("https server", func() error {
		log.WithField("port", tlsPort).Info("started HTTPS/GRPC server")
		return httpsServer.Serve(tlsListener)
	})
	tasks.Queue("debug server", func() error {
		log.WithField("port", debugPort).Info("started debug server")
		return debugServer.ListenAndServe()
	})

	tasks.GoRun()

	log.Printf("Listening on %s\n", tlsListener.Addr().String())

	err = tasks.Wait()
	if err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	log.Println("goodbye")
	os.Exit(0)
}
