package main

import (
	context "context"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

func dialAddress(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var dialAddr string
	opts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithBlock()}
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

/// This is a bit reversed from normal operations. We're forwarding messages
/// from the local grpc server to a remote server.  Sends messages received by
/// the server to the client and sends responses sent by the client to the
/// server.
func proxyStream(ctx context.Context, source grpc.ServerStream, destination grpc.ClientStream, req interface{}, resp interface{}) error {
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
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
			err = destination.SendMsg(req)
			if err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
		}
	})
	eg.Go(func() error {
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
			err = source.SendMsg(resp)
			if err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
		}
	})

	return eg.Wait()
}
