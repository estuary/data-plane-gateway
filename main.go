package main

import (
	context "context"
	"flag"
	"log"
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
	port               = flag.String("port", "", "Service port for HTTP and gRPC requests. A random port is used if not set. Port may also take the form 'unix:///path/to/socket' to use a Unix Domain Socket")
	zone               = flag.String("zone", "local", "Availability zone within which this process is running")
)

func main() {
	flag.Parse()
	envy.Parse("GATEWAY")

	pb.RegisterGRPCDispatcher(*zone)

	srv, err := server.New("", *port)
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
