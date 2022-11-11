package main

import (
	context "context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

func logUnaryRPC(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	var start = time.Now().UTC()
	// "method" is the gRPC term, which actually means "path" in HTTP terms.
	log.WithField("method", method).Trace("starting unary RPC")
	var err = invoker(ctx, method, req, reply, cc, opts...)
	log.WithFields(log.Fields{
		"method":     method,
		"timeMillis": time.Now().UTC().Sub(start).Milliseconds,
		"error":      err,
	}).Debug("finished gRPC client request")
	return err
}

func dialAddress(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var logger = grpc.UnaryClientInterceptor(grpc.UnaryClientInterceptor(logUnaryRPC))
	var dialAddr string
	opts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithBlock(), grpc.WithChainUnaryInterceptor(logger)}
	if strings.HasPrefix(addr, "unix://") {
		parsedUrl, err := url.Parse(addr)
		if err != nil {
			return nil, err
		}
		dialAddr = parsedUrl.Path
		opts = append(opts, grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
	} else {
		dialAddr = addr
	}

	conn, err := grpc.DialContext(dialCtx, dialAddr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial `%v`: %w", dialAddr, err)
	}
	log.Printf("[dialAddress] dial successful. addr = %s", addr)

	go func() {
		<-ctx.Done()
		if cerr := conn.Close(); cerr != nil {
			grpclog.Infof("Failed to close conn to %s: %v", addr, cerr)
		}
	}()

	return conn, err
}

// / This is a bit reversed from normal operations. We're forwarding messages
// / from the local grpc server to a remote server.  Sends messages received by
// / the server to the client and sends responses sent by the client to the
// / server.
func proxyStream(ctx context.Context, streamDesc string, source grpc.ServerStream, destination grpc.ClientStream, req interface{}, resp interface{}) error {
	eg, ctx := errgroup.WithContext(ctx)
	log.WithField("stream", streamDesc).Trace("starting streaming proxy")
	var startTime = time.Now().UTC()

	eg.Go(func() (_err error) {
		var msgCount = 0
		defer func() {
			log.WithFields(log.Fields{
				"stream":   streamDesc,
				"error":    _err,
				"msgCount": msgCount,
			}).Trace("finished forwarding messages from client to server")
		}()
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
				// Keep going
			}
			err := source.RecvMsg(req)
			if err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
			msgCount++
			err = destination.SendMsg(req)
			if err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
		}
	})
	eg.Go(func() (_err error) {
		var msgCount = 0
		defer func() {
			log.WithFields(log.Fields{
				"stream":   streamDesc,
				"error":    _err,
				"msgCount": msgCount,
			}).Trace("finished forwarding messages from server to client")
		}()
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
				// Keep going
			}
			err := destination.RecvMsg(resp)
			if err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
			msgCount++
			err = source.SendMsg(resp)
			if err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
		}
	})

	var err = eg.Wait()
	log.WithFields(log.Fields{
		"stream":     streamDesc,
		"error":      err,
		"timeMillis": time.Now().UTC().Sub(startTime).Milliseconds(),
	}).Debug("finished proxying streaming RPC")
	return err
}
