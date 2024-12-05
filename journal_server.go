package main

import (
	"context"
	"fmt"
	"io"
	"time"

	pb "go.gazette.dev/core/broker/protocol"
	"google.golang.org/grpc/metadata"
)

type PassThroughAuthorizer struct{}

func (PassThroughAuthorizer) Authorize(ctx context.Context, claims pb.Claims, exp time.Duration) (context.Context, error) {
	var md, _ = metadata.FromIncomingContext(ctx)
	var token = md.Get("authorization")

	if len(token) == 0 {
		return nil, fmt.Errorf("missing required Authorization header")
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", token[0]), nil
}

type JournalProxy struct {
	jc pb.JournalClient
}

func (s *JournalProxy) List(req *pb.ListRequest, stream pb.Journal_ListServer) error {
	var ctx = pb.WithDispatchDefault(stream.Context())

	var proxy, err = s.jc.List(ctx, req)
	if err != nil {
		return err
	}

	for {
		if resp, err := proxy.Recv(); err != nil {
			if err == io.EOF {
				err = nil // Graceful close.
			}
			return err
		} else if err = stream.Send(resp); err != nil {
			return err
		}
	}
}

func (s *JournalProxy) ListFragments(ctx context.Context, req *pb.FragmentsRequest) (*pb.FragmentsResponse, error) {
	return s.jc.ListFragments(pb.WithDispatchDefault(ctx), req)
}

func (s *JournalProxy) Read(req *pb.ReadRequest, stream pb.Journal_ReadServer) error {
	var ctx = stream.Context()

	var proxy, err = s.jc.Read(pb.WithDispatchDefault(ctx), req)
	if err != nil {
		return err
	}

	for {
		if resp, err := proxy.Recv(); err != nil {
			if err == io.EOF {
				err = nil // Graceful close.
			}
			return err
		} else if err = stream.Send(resp); err != nil {
			return err
		}
	}
}

func (s *JournalProxy) Append(stream pb.Journal_AppendServer) error {
	var ctx = stream.Context()

	var req, err = stream.Recv()
	if err != nil {
		return err
	}

	ctx = pb.WithClaims(ctx, pb.Claims{
		Capability: pb.Capability_APPEND,
		Selector: pb.LabelSelector{
			Include: pb.MustLabelSet("name", req.Journal.String()),
		},
	})
	ctx = pb.WithDispatchDefault(ctx)

	proxy, err := s.jc.Append(ctx)
	if err != nil {
		return err
	}

	for {
		if err = proxy.Send(req); err != nil {
			return err
		} else if req, err = stream.Recv(); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	resp, err := proxy.CloseAndRecv()

	if err == nil {
		err = stream.SendAndClose(resp)
	}
	return err
}

func (s *JournalProxy) Apply(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	return s.jc.Apply(pb.WithDispatchDefault(ctx), req)
}

func (s *JournalProxy) Replicate(pb.Journal_ReplicateServer) error {
	return fmt.Errorf("unsupported operation: `Replicate`")
}

var _ pb.JournalServer = &JournalProxy{}
