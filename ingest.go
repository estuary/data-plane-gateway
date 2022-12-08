package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/estuary/flow/go/protocols/capture"
	"github.com/estuary/flow/go/protocols/flow"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
)

type pushRpc struct {
	rpc  capture.Runtime_PushClient
	conn *grpc.ClientConn
}

func newPushRpc(ctx context.Context, serverAddr string) (*pushRpc, error) {
	log.Info("Connecting to: ", serverAddr)
	if conn, err := grpc.Dial(serverAddr, grpc.WithInsecure()); err != nil {
		return nil, fmt.Errorf("fail to dial: %w", err)
	} else if rpc, err := capture.NewRuntimeClient(conn).Push(ctx); err != nil {
		return nil, fmt.Errorf("failed to create Push rpc: %w", err)
	} else {
		return &pushRpc{
			conn: conn,
			rpc:  rpc,
		}, nil
	}
}

func (p *pushRpc) Open(captureName string) (*capture.PushResponse, error) {
	var err = p.rpc.Send(&capture.PushRequest{
		Open: &capture.PushRequest_Open{
			Capture: flow.Capture(captureName),
		},
	})

	if err != nil {
		return nil, err
	}

	var resp, resp_err = p.rpc.Recv()

	if resp_err != nil {
		return nil, resp_err
	}

	return resp, nil
}

func (p *pushRpc) SendDocuments(reader io.Reader, bindingNum int, batchSize int) error {
	var scanner = bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 17*1024*1024)
	scanner.Split(bufio.ScanLines)

	var arena flow.Arena
	var docs []flow.Slice

	count := 0
	for scanner.Scan() {
		count += 1
		var bytes = scanner.Bytes()
		docs = append(docs, arena.Add(bytes))

		if batchSize < 1 || (count%batchSize == 0 && len(docs) > 0) {
			log.Info(fmt.Sprintf("Sending a chunk of %d docs", len(docs)))
			if err := p.rpc.Send(&capture.PushRequest{
				Documents: &capture.Documents{
					Binding:  uint32(bindingNum),
					Arena:    arena,
					DocsJson: docs,
				},
			}); err != nil {
				return fmt.Errorf("send documentszzz: %w", err)
			} else if err := p.Checkpoint(); err != nil {
				return fmt.Errorf("checkpoint: %w", err)
			} else if err := p.Acknowledge(); err != nil {
				return fmt.Errorf("acknowledge: %w", err)
			}

			arena = flow.Arena{}
			docs = []flow.Slice{}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan input: %w", err)
	}

	log.WithField("docs", len(docs)).Info("Done!")

	if len(docs) == 0 {
		return nil
	}

	return p.rpc.Send(&capture.PushRequest{
		Documents: &capture.Documents{
			Binding:  uint32(bindingNum),
			Arena:    arena,
			DocsJson: docs,
		},
	})
}

func (p *pushRpc) Checkpoint() error {
	return p.rpc.Send(&capture.PushRequest{
		Checkpoint: &flow.DriverCheckpoint{},
	})
}

func (p *pushRpc) Acknowledge() error {
	var resp, err = p.rpc.Recv()
	if err != nil {
		return fmt.Errorf("failed in receiving response, %+v", err)
	} else if err = resp.Validate(); err != nil {
		return fmt.Errorf("failed in validating response, %+v", err)
	}
	return nil
}

func (p *pushRpc) Close() {
	p.conn.Close()
}

func logAndExit(err error) {
	log.Fatalf("execution failed: %v", err)
	os.Exit(1)
}

type IngestCaptureBinding struct {
	Name string `json:"name"`
}

var ingestHandler = http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
	var path_components = strings.Split(strings.TrimPrefix(req.URL.Path, "/ingest/"), "/")

	if len(path_components) < 2 {
		http.Error(
			writer,
			"Expected path to be in the format: <capture_name>/<binding_name>",
			http.StatusBadRequest,
		)
		return
	}

	var binding_name = path_components[len(path_components)-1]
	var capture_name = strings.Join(path_components[:len(path_components)-1], "/")

	log := log.WithField("capture_name", capture_name).WithField("binding_name", binding_name)
	log.Info("Running ingest")

	var ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var rpc, err = newPushRpc(ctx, *consumerAddr)
	if err != nil {
		log.Error("execution failed", err.Error())
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rpc.Close()

	var open_resp, open_error = rpc.Open(capture_name)
	if open_error != nil {
		log.Error("open request: ", open_error)
		http.Error(writer, open_error.Error(), http.StatusInternalServerError)
		return
	}

	var bindings = open_resp.Opened.Capture.Bindings
	var binding_num = slices.IndexFunc(
		bindings,
		func(c *flow.CaptureSpec_Binding) bool {
			var parsed IngestCaptureBinding
			err = json.Unmarshal(c.ResourceSpecJson, &parsed)
			if err != nil {
				return false
			}
			return parsed.Name == binding_name
		},
	)

	if binding_num == -1 {
		http.Error(
			writer,
			"No binding found: "+binding_name,
			http.StatusInternalServerError,
		)
		return
	}

	if err := rpc.SendDocuments(req.Body, binding_num, 0); err != nil {
		err := fmt.Errorf("send documents: %w", err)
		log.Error(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	} else if err := rpc.Checkpoint(); err != nil {
		err := fmt.Errorf("checkpoint: %w", err)
		log.Error(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	} else if err := rpc.Acknowledge(); err != nil {
		err := fmt.Errorf("acknowledge: %w", err)
		log.Error(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
})
