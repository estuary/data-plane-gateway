package main

import (
	context "context"
	"fmt"
	"log"

	pc "go.gazette.dev/core/consumer/protocol"
)

type ShardAuthServer struct {
	clientCtx   context.Context
	shardClient pc.ShardClient
}

func NewShardAuthServer() *ShardAuthServer {
	ctx := context.Background()

	shardClient, err := newShardClient(ctx, *consumerAddr)
	if err != nil {
		log.Fatalf("Failed to connect to consumer: %v", err)
	}

	authServer := &ShardAuthServer{
		clientCtx:   ctx,
		shardClient: shardClient,
	}

	return authServer
}

func newShardClient(ctx context.Context, addr string) (pc.ShardClient, error) {
	conn, err := dialAddress(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("error connecting to server: %w", err)
	}

	return pc.NewShardClient(conn), nil
}

// List implements protocol.ShardServer
func (s *ShardAuthServer) List(ctx context.Context, req *pc.ListRequest) (*pc.ListResponse, error) {
	return s.shardClient.List(ctx, req)

}

// Stat implements protocol.ShardServer
func (s *ShardAuthServer) Stat(ctx context.Context, req *pc.StatRequest) (*pc.StatResponse, error) {
	return s.shardClient.Stat(ctx, req)

}

// We're currently only implementing the read-only RPCs for protocol.ShardServer.
func (s *ShardAuthServer) Apply(context.Context, *pc.ApplyRequest) (*pc.ApplyResponse, error) {
	panic("unimplemented")
}
func (s *ShardAuthServer) GetHints(context.Context, *pc.GetHintsRequest) (*pc.GetHintsResponse, error) {
	panic("unimplemented")
}
func (s *ShardAuthServer) Unassign(context.Context, *pc.UnassignRequest) (*pc.UnassignResponse, error) {
	panic("unimplemented")
}

var _ pc.ShardServer = &ShardAuthServer{}
