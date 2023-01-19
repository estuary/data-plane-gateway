package main

import (
	context "context"
	"fmt"

	"github.com/estuary/data-plane-gateway/auth"
	log "github.com/sirupsen/logrus"
	pc "go.gazette.dev/core/consumer/protocol"
)

type ShardAuthServer struct {
	clientCtx          context.Context
	shardClient        pc.ShardClient
	jwtVerificationKey []byte
}

func NewShardAuthServer(ctx context.Context, jwtVerificationKey []byte) *ShardAuthServer {
	shardClient, err := newShardClient(ctx, *consumerAddr)
	if err != nil {
		log.Fatalf("Failed to connect to consumer: %v", err)
	}

	authServer := &ShardAuthServer{
		clientCtx:          ctx,
		shardClient:        shardClient,
		jwtVerificationKey: jwtVerificationKey,
	}

	return authServer
}

func newShardClient(ctx context.Context, addr string) (pc.ShardClient, error) {
	var entry = log.WithField("address", addr)
	entry.Debug("starting to connect shard client")
	conn, err := dialAddress(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("error connecting to server: %w", err)
	}

	entry.Info("successfully connected shard client")
	return pc.NewShardClient(conn), nil
}

// List implements protocol.ShardServer
func (s *ShardAuthServer) List(ctx context.Context, req *pc.ListRequest) (*pc.ListResponse, error) {
	claims, err := auth.Authorized(ctx, s.jwtVerificationKey)
	if err != nil {
		return nil, err
	}

	err = auth.EnforceSelectorPrefix(claims, req.Selector)
	if err != nil {
		return nil, fmt.Errorf("Unauthorized: %w", err)
	}

	return s.shardClient.List(ctx, req)

}

// Stat implements protocol.ShardServer
func (s *ShardAuthServer) Stat(ctx context.Context, req *pc.StatRequest) (*pc.StatResponse, error) {
	claims, err := auth.Authorized(ctx, s.jwtVerificationKey)
	if err != nil {
		return nil, err
	}

	err = auth.EnforcePrefix(claims, req.Shard.String())
	if err != nil {
		return nil, fmt.Errorf("Unauthorized: %w", err)
	}

	return s.shardClient.Stat(ctx, req)

}

// We're currently only implementing the read-only RPCs for protocol.ShardServer.
func (s *ShardAuthServer) Apply(context.Context, *pc.ApplyRequest) (*pc.ApplyResponse, error) {
	return nil, fmt.Errorf("Unsupported operation: `Apply`")
}
func (s *ShardAuthServer) GetHints(context.Context, *pc.GetHintsRequest) (*pc.GetHintsResponse, error) {
	return nil, fmt.Errorf("Unsupported operation: `GetHints`")
}
func (s *ShardAuthServer) Unassign(context.Context, *pc.UnassignRequest) (*pc.UnassignResponse, error) {
	return nil, fmt.Errorf("Unsupported operation: `Unassign`")
}

var _ pc.ShardServer = &ShardAuthServer{}
