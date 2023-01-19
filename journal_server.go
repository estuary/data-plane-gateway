package main

import (
	context "context"
	"fmt"

	"github.com/estuary/data-plane-gateway/auth"
	log "github.com/sirupsen/logrus"
	pb "go.gazette.dev/core/broker/protocol"
)

type JournalAuthServer struct {
	clientCtx          context.Context
	journalClient      pb.JournalClient
	jwtVerificationKey []byte
}

func NewJournalAuthServer(ctx context.Context, jwtVerificationKey []byte) *JournalAuthServer {
	journalClient, err := newJournalClient(ctx, *brokerAddr)
	if err != nil {
		log.Fatalf("Failed to connect to broker: %v", err)
	}

	authServer := &JournalAuthServer{
		clientCtx:          ctx,
		journalClient:      journalClient,
		jwtVerificationKey: jwtVerificationKey,
	}

	return authServer
}

func newJournalClient(ctx context.Context, addr string) (pb.JournalClient, error) {
	log.Printf("connecting journal client to: %s", addr)
	conn, err := dialAddress(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("error connecting to server: %w", err)
	}

	return pb.NewJournalClient(conn), nil
}

// List implements protocol.JournalServer
func (s *JournalAuthServer) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	claims, err := auth.Authorized(ctx, s.jwtVerificationKey)
	if err != nil {
		return nil, err
	}

	err = auth.EnforceSelectorPrefix(claims, req.Selector)
	if err != nil {
		return nil, fmt.Errorf("Unauthorized: %w", err)
	}

	ctx = pb.WithDispatchDefault(ctx)
	return s.journalClient.List(ctx, req)
}

// ListFragments implements protocol.JournalServer
func (s *JournalAuthServer) ListFragments(ctx context.Context, req *pb.FragmentsRequest) (*pb.FragmentsResponse, error) {
	claims, err := auth.Authorized(ctx, s.jwtVerificationKey)
	if err != nil {
		return nil, err
	}

	err = auth.EnforcePrefix(claims, req.Journal.String())
	if err != nil {
		return nil, fmt.Errorf("Unauthorized: %w", err)
	}

	return s.journalClient.ListFragments(ctx, req)
}

// Read implements protocol.JournalServer
func (s *JournalAuthServer) Read(readReq *pb.ReadRequest, readServer pb.Journal_ReadServer) error {
	ctx := readServer.Context()

	claims, err := auth.Authorized(ctx, s.jwtVerificationKey)
	if err != nil {
		return err
	}

	err = auth.EnforcePrefix(claims, readReq.Journal.String())
	if err != nil {
		return fmt.Errorf("Unauthorized: %w", err)
	}

	readClient, err := s.journalClient.Read(ctx, readReq)
	if err != nil {
		return err
	}

	return proxyStream(ctx, "/protocol.Journal/Read", readServer, readClient, new(pb.ReadRequest), new(pb.ReadResponse))
}

// We're currently only implementing the read-only RPCs for protocol.JournalServer.
func (s *JournalAuthServer) Append(pb.Journal_AppendServer) error {
	return fmt.Errorf("Unsupported operation: `Append`")
}
func (s *JournalAuthServer) Apply(context.Context, *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	return nil, fmt.Errorf("Unsupported operation: `Apply`")
}
func (s *JournalAuthServer) Replicate(pb.Journal_ReplicateServer) error {
	return fmt.Errorf("Unsupported operation: `Replicate`")
}

var _ pb.JournalServer = &JournalAuthServer{}
