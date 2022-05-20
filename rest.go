package main

import (
	"context"
	"log"
	"net/http"
	"regexp"

	"github.com/gogo/gateway"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/urfave/negroni"
	_ "go.gazette.dev/core/broker/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	bgw "github.com/estuary/data-plane-gateway/gen/broker/protocol"
	cgw "github.com/estuary/data-plane-gateway/gen/consumer/protocol"
)

func NewRestServer(ctx context.Context, gatewayAddr string) http.Handler {
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

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	err = bgw.RegisterJournalHandlerFromEndpoint(ctx, mux, gatewayAddr, opts)
	if err != nil {
		log.Fatalf("Failed to initialize journal rest gateway: %v", err)
	}

	err = cgw.RegisterShardHandlerFromEndpoint(ctx, mux, gatewayAddr, opts)
	if err != nil {
		log.Fatalf("Failed to initialize shard rest gateway: %v", err)
	}

	n := negroni.Classic()
	n.Use(negroni.HandlerFunc(cors))
	n.UseHandler(mux)

	return n
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
