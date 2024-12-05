package main

import (
	context "context"

	pb "go.gazette.dev/core/broker/protocol"
	pc "go.gazette.dev/core/consumer/protocol"
)

type ShardProxy struct {
	sc pc.ShardClient
}

func (s *ShardProxy) List(ctx context.Context, req *pc.ListRequest) (*pc.ListResponse, error) {
	return s.sc.List(pb.WithDispatchDefault(ctx), req)
}

func (s *ShardProxy) Stat(ctx context.Context, req *pc.StatRequest) (*pc.StatResponse, error) {
	return s.sc.Stat(pb.WithDispatchDefault(ctx), req)
}

func (s *ShardProxy) Apply(ctx context.Context, req *pc.ApplyRequest) (*pc.ApplyResponse, error) {
	return s.sc.Apply(pb.WithDispatchDefault(ctx), req)
}
func (s *ShardProxy) GetHints(ctx context.Context, req *pc.GetHintsRequest) (*pc.GetHintsResponse, error) {
	return s.sc.GetHints(pb.WithDispatchDefault(ctx), req)
}
func (s *ShardProxy) Unassign(ctx context.Context, req *pc.UnassignRequest) (*pc.UnassignResponse, error) {
	return s.sc.Unassign(pb.WithDispatchDefault(ctx), req)
}

var _ pc.ShardServer = &ShardProxy{}
