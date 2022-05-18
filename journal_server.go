package main

import (
	context "context"
	"fmt"
	"log"

	pb "go.gazette.dev/core/broker/protocol"
)

type JournalAuthServer struct {
	clientCtx     context.Context
	journalClient pb.JournalClient
}

func NewJournalAuthServer() *JournalAuthServer {
	ctx := context.Background()

	journalClient, err := newJournalClient(ctx, *brokerAddr)
	if err != nil {
		log.Fatalf("Failed to connect to broker: %v", err)
	}

	authServer := &JournalAuthServer{
		clientCtx:     ctx,
		journalClient: journalClient,
	}

	return authServer
}

func newJournalClient(ctx context.Context, addr string) (pb.JournalClient, error) {
	conn, err := dialAddress(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("error connecting to server: %w", err)
	}

	return pb.NewJournalClient(conn), nil
}

// List implements protocol.JournalServer
func (s *JournalAuthServer) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	return s.journalClient.List(ctx, req)
}

// ListFragments implements protocol.JournalServer
func (s *JournalAuthServer) ListFragments(ctx context.Context, req *pb.FragmentsRequest) (*pb.FragmentsResponse, error) {
	return s.journalClient.ListFragments(ctx, req)
}

// Read implements protocol.JournalServer
func (s *JournalAuthServer) Read(readReq *pb.ReadRequest, readServer pb.Journal_ReadServer) error {
	ctx := readServer.Context()

	readClient, err := s.journalClient.Read(ctx, readReq)
	if err != nil {
		return err
	}

	return proxyStream(ctx, readServer, readClient, new(pb.ReadRequest), new(pb.ReadResponse))
}

// We're currently only implementing the read-only RPCs for protocol.JournalServer.
func (s *JournalAuthServer) Append(pb.Journal_AppendServer) error { panic("unimplemented") }
func (s *JournalAuthServer) Apply(context.Context, *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	panic("unimplemented")
}
func (s *JournalAuthServer) Replicate(pb.Journal_ReplicateServer) error { panic("unimplemented") }

var _ pb.JournalServer = &JournalAuthServer{}
