package main

import (
	"bytes"
	context "context"
	"fmt"
	"slices"

	"github.com/estuary/data-plane-gateway/auth"
	"github.com/estuary/flow/go/labels"
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
	claims, err := auth.AuthenticateGrpcReq(ctx, s.jwtVerificationKey)
	if err != nil {
		return nil, err
	}
	ctx = pb.WithDispatchDefault(ctx)

	// Is the user listing (only) ops collections?
	var requested = req.Selector.Include.ValuesOf(labels.Collection)
	var isOpsListing = len(requested) != 0
	for _, r := range requested {
		isOpsListing = isOpsListing && slices.Contains(allOpsCollections, r)
	}

	// Special-case listings of ops collections.
	// We list all journals, and then filter to those that the user may access.
	if isOpsListing {
		var resp, err = s.journalClient.List(ctx, req)
		if err != nil {
			return nil, err
		}

		// Filter journals to those the user has access to.
		var filtered []pb.ListResponse_Journal
		for _, j := range resp.Journals {
			if isAllowedOpsJournal(claims, j.Spec.Name) {
				filtered = append(filtered, j)
			}
		}
		resp.Journals = filtered
		return resp, nil
	}

	err = auth.EnforceSelectorPrefix(claims, req.Selector)
	if err != nil {
		return nil, fmt.Errorf("Unauthorized: %w", err)
	}

	return s.journalClient.List(ctx, req)
}

// ListFragments implements protocol.JournalServer
func (s *JournalAuthServer) ListFragments(ctx context.Context, req *pb.FragmentsRequest) (*pb.FragmentsResponse, error) {
	claims, err := auth.AuthenticateGrpcReq(ctx, s.jwtVerificationKey)
	if err != nil {
		return nil, err
	}

	err = auth.EnforcePrefix(claims, req.Journal.String())
	if err != nil {
		if !isAllowedOpsJournal(claims, req.Journal) {
			return nil, fmt.Errorf("Unauthorized: %w", err)
		}
	}

	return s.journalClient.ListFragments(ctx, req)
}

// Read implements protocol.JournalServer
func (s *JournalAuthServer) Read(req *pb.ReadRequest, readServer pb.Journal_ReadServer) error {
	ctx := readServer.Context()

	claims, err := auth.AuthenticateGrpcReq(ctx, s.jwtVerificationKey)
	if err != nil {
		return err
	}

	err = auth.EnforcePrefix(claims, req.Journal.String())
	if err != nil {
		if !isAllowedOpsJournal(claims, req.Journal) {
			return fmt.Errorf("Unauthorized: %w", err)
		}
	}

	readClient, err := s.journalClient.Read(ctx, req)
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

// TODO(johnny): This authorization check is an encapsulated hack that allows
// ops logs and stats to be read-able by end users.
// It's a placeholder for a missing partition-level authorization feature.
func isAllowedOpsJournal(claims *auth.AuthorizedClaims, journal pb.Journal) bool {
	var b = make([]byte, 256)

	for _, oc := range allOpsCollections {
		for _, kind := range []string{"capture", "derivation", "materialization"} {
			for _, prefix := range claims.Prefixes {
				b = append(b[:0], oc...)
				b = append(b, "/kind="...)
				b = append(b, kind...)
				b = append(b, "/name="...)
				b = labels.EncodePartitionValue(b, prefix)

				if bytes.HasPrefix([]byte(journal), b) {
					return true
				}
			}
		}
	}
	return false
}

var allOpsCollections = []string{
	"ops.us-central1.v1/logs",
	"ops.us-central1.v1/stats",
}

var _ pb.JournalServer = &JournalAuthServer{}
