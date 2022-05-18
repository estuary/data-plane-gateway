package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/soheilhy/cmux"
	pb "go.gazette.dev/core/broker/protocol"
	pc "go.gazette.dev/core/consumer/protocol"
	"google.golang.org/grpc"
)

var (
	brokerAddr         = flag.String("broker-address", "localhost:8080", "Target broker address")
	consumerAddr       = flag.String("consumer-address", "localhost:9000", "Target consumer address")
	corsOrigin         = flag.String("cors-origin", "*", "CORS Origin")
	gatewayAddr        = flag.String("gateway-address", "localhost:28318", "Gateway address")
	jwtVerificationKey = flag.String("verification-key", "supersecret", "Key used to verify JWTs signed by the Flow Control Plane")
)

func main() {
	flag.Parse()

	journalServer := NewJournalAuthServer()
	shardServer := NewShardAuthServer()

	grpcServer := grpc.NewServer()
	pb.RegisterJournalServer(grpcServer, journalServer)
	pc.RegisterShardServer(grpcServer, shardServer)

	mux := NewGracefulMux()
	go mux.ServeGrpc(*grpcServer)
	go mux.ServeHttp(NewRestServer())
	go mux.TrapShutdownSignal(5 * time.Second)

	log.Printf("Listening on %s\n", *gatewayAddr)
	err := mux.Serve()
	if err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	os.Exit(0)
}

type GracefulMux struct {
	mux      cmux.CMux
	shutdown chan bool
}

func NewGracefulMux() *GracefulMux {
	listener, err := listen(*gatewayAddr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	return &GracefulMux{
		mux:      cmux.New(listener),
		shutdown: make(chan bool, 0),
	}
}

func (s *GracefulMux) ServeGrpc(server grpc.Server) {
	// TODO: figure out why the content-type matcher doesn't work.
	// listener := s.mux.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	listener := s.mux.Match(cmux.HTTP2())

	server.Serve(listener)
}

func (s *GracefulMux) ServeHttp(handler http.Handler) {
	listener := s.mux.Match(cmux.HTTP1Fast())
	server := &http.Server{Handler: handler}

	server.Serve(listener)
}

func (s *GracefulMux) Serve() error {
	err := s.mux.Serve()
	if err != nil {
		return err
	}

	// Wait for shutdown to complete
	<-s.shutdown

	return nil
}

func (s *GracefulMux) TrapShutdownSignal(timeout time.Duration) error {
	var signaled = make(chan os.Signal, 1)
	signal.Notify(signaled, syscall.SIGTERM, syscall.SIGINT)
	<-signaled

	s.mux.Close()

	// Signal that we've finished shutting down
	close(s.shutdown)

	return nil
}
