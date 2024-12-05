package main

import (
	context "context"
	"crypto/tls"
	"os"
	"os/signal"
	"syscall"

	"github.com/estuary/flow/go/network"
	pf "github.com/estuary/flow/go/protocols/flow"
	"github.com/gogo/gateway"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
	"go.gazette.dev/core/auth"
	pb "go.gazette.dev/core/broker/protocol"
	pc "go.gazette.dev/core/consumer/protocol"
	mbp "go.gazette.dev/core/mainboilerplate"
	"go.gazette.dev/core/server"
	"go.gazette.dev/core/task"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
)

// Config is the top-level configuration object of data-plane-gateway.
var Config = new(struct {
	Broker struct {
		mbp.AddressConfig
	} `group:"Broker" namespace:"broker" env-namespace:"BROKER"`

	Consumer struct {
		mbp.AddressConfig
	} `group:"Consumer" namespace:"consumer" env-namespace:"CONSUMER"`

	Gateway struct {
		mbp.ServiceConfig
	} `group:"Gateway" namespace:"gateway" env-namespace:"GATEWAY"`

	Flow struct {
		ControlAPI    pb.Endpoint `long:"control-api" env:"CONTROL_API" description:"Address of the control-plane API"`
		Dashboard     pb.Endpoint `long:"dashboard" env:"DASHBOARD" description:"Address of the Estuary dashboard"`
		DataPlaneFQDN string      `long:"data-plane-fqdn" env:"DATA_PLANE_FQDN" description:"Fully-qualified domain name of the data-plane to which this reactor belongs"`
	} `group:"flow" namespace:"flow" env-namespace:"FLOW"`

	Log         mbp.LogConfig         `group:"Logging" namespace:"log" env-namespace:"LOG"`
	Diagnostics mbp.DiagnosticsConfig `group:"Debug" namespace:"debug" env-namespace:"DEBUG"`
})

const iniFilename = "data-plane-gateway.ini"

type cmdServe struct{}

func (cmdServe) Execute(args []string) error {
	defer mbp.InitDiagnosticsAndRecover(Config.Diagnostics)()
	mbp.InitLog(Config.Log)

	log.WithFields(log.Fields{
		"config":    Config,
		"version":   mbp.Version,
		"buildDate": mbp.BuildDate,
	}).Info("data-plane-gateway configuration")
	pb.RegisterGRPCDispatcher(Config.Gateway.Zone)

	var shardKeys, err = auth.NewKeyedAuth(Config.Consumer.AuthKeys)
	mbp.Must(err, "failed to parse consumer auth keys")

	var serverTLS *tls.Config
	var tap = network.NewTap()

	if Config.Gateway.ServerCertFile != "" {
		serverTLS, err = server.BuildTLSConfig(
			Config.Gateway.ServerCertFile, Config.Gateway.ServerCertKeyFile, "")
		mbp.Must(err, "building server TLS config")
	}

	// Bind our server listener, grabbing a random available port if Port is zero.
	srv, err := server.New(
		"",
		Config.Gateway.Host,
		Config.Gateway.Port,
		serverTLS,
		nil,
		Config.Gateway.MaxGRPCRecvSize,
		tap.Wrap,
	)
	mbp.Must(err, "building Server instance")

	var (
		ctx        = context.Background()
		brokerConn = Config.Broker.MustDial(ctx)
		shardConn  = Config.Consumer.MustDial(ctx)
		signalCh   = make(chan os.Signal, 1)
		tasks      = task.NewGroup(ctx)

		journalClient = pb.NewAuthJournalClient(
			pb.NewJournalClient(brokerConn),
			PassThroughAuthorizer{},
		)
		shardClient = pc.NewAuthShardClient(
			pc.NewShardClient(shardConn),
			PassThroughAuthorizer{},
		)
	)
	srv.QueueTasks(tasks)

	// Register proxying gRPC server.
	pb.RegisterJournalServer(srv.GRPCServer, &JournalProxy{journalClient})
	pc.RegisterShardServer(srv.GRPCServer, &ShardProxy{shardClient})

	// Register gRPC web gateway.
	var mux *runtime.ServeMux = runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &gateway.JSONPb{EmitDefaults: true}),
		runtime.WithProtoErrorHandler(runtime.DefaultHTTPProtoErrorHandler),
	)
	pb.RegisterJournalHandler(tasks.Context(), mux, brokerConn)
	pc.RegisterShardHandler(tasks.Context(), mux, shardConn)
	srv.HTTPMux.Handle("/v1/", Config.Gateway.CORSWrapper(mux))

	// Initialize connector networking frontend.
	networkProxy, err := network.NewFrontend(
		tap,
		Config.Gateway.Host,
		Config.Flow.ControlAPI.URL(),
		Config.Flow.Dashboard.URL(),
		pf.NewAuthNetworkProxyClient(pf.NewNetworkProxyClient(shardConn), shardKeys),
		pc.NewAuthShardClient(pc.NewShardClient(shardConn), shardKeys),
		shardKeys,
	)
	mbp.Must(err, "failed to build network proxy")

	tasks.Queue("network-proxy-frontend", func() error {
		return networkProxy.Serve(tasks.Context())
	})

	// Install signal handler & start gateway tasks.
	signal.Notify(signalCh, syscall.SIGTERM, syscall.SIGINT)
	tasks.Queue("handle-signal", func() error {
		<-signalCh
		log.Info("caught signal; exiting")
		return nil
	})

	// Block until all tasks complete. Assert none returned an error.
	tasks.GoRun()
	mbp.Must(tasks.Wait(), "data-plane-gateway task failed")
	log.Info("goodbye")

	return nil
}

func main() {
	var parser = flags.NewParser(Config, flags.Default)

	_, _ = parser.AddCommand("serve", "Serve as Gazette broker", `
Serve a Gazette broker with the provided configuration, until signaled to
exit (via SIGTERM). Upon receiving a signal, the broker will seek to discharge
its responsible journals and will exit only when it can safely do so.
`, &cmdServe{})

	mbp.AddPrintConfigCmd(parser, iniFilename)
	mbp.MustParseConfig(parser, iniFilename)
}

/*

	proxyServer, tappedListener, err := proxy.NewTlsProxyServer(*hostname, uint16(tlsPortNum), certificates, shardServer.shardClient, *cpAuthUrl, []byte(*jwtVerificationKey))

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
*/
