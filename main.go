package main

import (
	"flag"
	"log"
	"os"
	"time"
)

var (
	brokerAddr   = flag.String("broker-address", "localhost:8080", "Target broker address")
	consumerAddr = flag.String("consumer-address", "localhost:9000", "Target consumer address")
	corsOrigin   = flag.String("cors-origin", "*", "CORS Origin")
	gatewayAddr  = flag.String("gateway-address", "localhost:8081", "Gateway address")
)

func main() {
	flag.Parse()
	var err error

	srv := NewRestServer()
	go srv.TrapShutdownSignal(15 * time.Second)

	// Start HTTP server (and proxy calls to gRPC server endpoint)
	err = srv.Serve(*gatewayAddr)
	if err != nil {
		log.Fatalf("Failed to serve rest gateway: %v", err)
	}
	os.Exit(0)
}
