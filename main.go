package main

import (
	context "context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/jamiealquiza/envy"
	pb "go.gazette.dev/core/broker/protocol"
	pc "go.gazette.dev/core/consumer/protocol"
	"go.gazette.dev/core/task"
	"google.golang.org/grpc"
)

var (
	brokerAddr         = flag.String("broker-address", "localhost:8080", "Target broker address")
	consumerAddr       = flag.String("consumer-address", "localhost:9000", "Target consumer address")
	inferenceAddr      = flag.String("inference-address", "localhost:9090", "Target schema inference service address")
	corsOrigin         = flag.String("cors-origin", "*", "CORS Origin")
	jwtVerificationKey = flag.String("verification-key", "supersecret", "Key used to verify JWTs signed by the Flow Control Plane")
	plainPort          = flag.String("plain-port", "28317", "Port for unencrypted communication")
	tlsPort            = flag.String("port", "28318", "Service port for HTTPS and gRPC requests. Port may also take the form 'unix:///path/to/socket' to use a Unix Domain Socket")
	zone               = flag.String("zone", "local", "Availability zone within which this process is running")

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
	corsConfig = NewCorsSettings(*corsOrigin)

	pb.RegisterGRPCDispatcher(*zone)
	var grpcServer = grpc.NewServer(
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	)

	ctx := pb.WithDispatchDefault(context.Background())
	var tasks = task.NewGroup(ctx)

	journalServer := NewJournalAuthServer(ctx)
	shardServer := NewShardAuthServer(ctx)
	pb.RegisterJournalServer(grpcServer, journalServer)
	pc.RegisterShardServer(grpcServer, shardServer)

	// Will be used with both http and https
	var healthHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK\n"))
	})

	// Will be used with both http and https
	// Inspired partially by https://gist.github.com/yowu/f7dc34bd4736a65ff28d
	var schemaInferenceHandler = http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		// Do auth
			// Pull JWT from authz header
				// See auth.go:authorized()
			// decodeJWT(that bearer token) -> AuthorizedClaims
		claims, err := authorized_req(req)
		if err != nil {
			return 401, err
		}

		collection_name := req.URL.Query().Get("collection_name")
		authorization_error := enforcePrefix(claims, collection_name)

		// enforcePrefix(claims, collection_name)
			// collection_name comes from actual inference request
		if authorization_error != nil {
			return 403, authorization_error
		}

		// Call inference
		inference_response, inference_error := http.Get(fmt.Sprintf("http://%s/infer_schema?%s", *inferenceAddr, *req.URL.RawQuery))

		if inference_error != nil {
			// An error is returned if there were too many redirects or if there was an HTTP protocol error. 
			// A non-2xx response doesn't cause an error.
			return 500, inference_error
		}

		defer resp.Body.Close()
		// Return result

		copyHeader(writer.Header(), inference_response.Header)
		wr.WriteHeader(inference_response.StatusCode)
		io.Copy(writer, inference_response.Body)
	})

	plainMux := http.NewServeMux()
	plainMux.Handle("/healthz", healthHandler)
	plainMux.Handle("/infer_schema", schemaInferenceHandler)
	plainMux.Handle("/", restHandler)

	httpsMux := http.NewServeMux()
	restHandler := NewRestServer(ctx, fmt.Sprintf("localhost:%s", *tlsPort))
	httpsMux.Handle("/healthz", healthHandler)
	httpsMux.Handle("/infer_schema", schemaInferenceHandler)
	httpsMux.Handle("/", restHandler)

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
		Addr: fmt.Sprintf(":%s", *plainPort),
	}
	var httpsServer = &http.Server{
		Handler: mixedHandler,
	}

	// This plain listener will be wrapped in one that does TLS termination. The implementation will
	// depend on whether the TLS cert was provided directly versus us provisioning it automatically.
	var tlsListener, err = net.Listen("tcp", fmt.Sprintf(":%s", *tlsPort))
	if err != nil {
		log.Fatalf("failed to start listener: %v", err)
	}

	// TODO: allow running without TLS, which means getting rid of the default value for `autoAcquireCert`

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
		tlsListener = tls.NewListener(tlsListener, &config)
	} else if *autoAcquireCert != "" {
		if *autoRenewCertBefore < 0 {
			log.Fatalf("--tls-renew-before-days must not be negative")
		}
		var certRenewBefore = 24 * time.Hour * time.Duration(*autoRenewCertBefore)
		certProvider, err := NewCertProvider(*autoAcquireCert, *etcdEndpoint, *autoAcquireCertEmail, certRenewBefore)
		if err != nil {
			log.Fatalf("initializing autocert: %v", err)
		}
		tlsListener = tls.NewListener(tlsListener, certProvider.TLSConfig())
		// Plain http will respond to ACME http-01 challenges and health checks.
		plainServer.Handler = certProvider.acManager.HTTPHandler(plainMux)
	}

	tasks.Queue("http server", func() error {
		log.Printf("Started HTTP server")
		return plainServer.ListenAndServe()
	})

	if *tls_cert != "" || *autoAcquireCert != "" {
		tasks.Queue("https server", func() error {
			log.Printf("Started HTTPS server")
			return httpsServer.Serve(tlsListener)
		})
	}
	tasks.GoRun()

	log.Printf("Listening on %s\n", tlsListener.Addr().String())

	err = tasks.Wait()
	if err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	log.Println("goodbye")
	os.Exit(0)
}
