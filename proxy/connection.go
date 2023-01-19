package proxy

import (
	"context"
	"net"
	"net/http"
	"time"

	pf "github.com/estuary/flow/go/protocols/flow"
	log "github.com/sirupsen/logrus"
)

type ProxyConnection struct {
	hostname   string
	taskName   string
	shardID    string
	targetPort uint16
	client     pf.NetworkProxy_ProxyClient
	// readBuf is the remaining Data from the most recent response message.
	// We don't do any explicit buffering, per se, at this layer.
	// This is just here in case the buffer that's given to `Read`
	// is too small to hold all the data from the last response.
	readBuf []byte
}

func (pc *ProxyConnection) singleConnectionTransport(useHttp2 bool) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
			log.WithFields(log.Fields{
				"hostname": pc.hostname,
				"shardID":  pc.shardID,
			}).Info("returning proxy connection from dialer")
			return pc, nil
		},
		MaxIdleConns:        1,
		MaxIdleConnsPerHost: 1,
		MaxConnsPerHost:     1,
		IdleConnTimeout:     0,
		//ResponseHeaderTimeout: 0,
		MaxResponseHeaderBytes: 0,
		ForceAttemptHTTP2:      useHttp2,
	}
}

// TODO: Do we need to handle deadlines?
func (pc *ProxyConnection) SetDeadline(dl time.Time) error {
	return nil
}
func (pc *ProxyConnection) SetReadDeadline(dl time.Time) error {
	return nil
}
func (pc *ProxyConnection) SetWriteDeadline(dl time.Time) error {
	return nil
}

func (pc *ProxyConnection) LocalAddr() net.Addr {
	return nil
}

func (pc *ProxyConnection) RemoteAddr() net.Addr {
	return nil
}

func (pc *ProxyConnection) Close() error {
	var err = pc.client.CloseSend()
	log.WithFields(log.Fields{
		"hostname": pc.hostname,
		"error":    err,
	}).Debug("closed upstream connection")
	return err
}

func (pc *ProxyConnection) Read(buf []byte) (int, error) {
	if len(pc.readBuf) == 0 {
		// We need to read another response
		var resp, err = pc.client.Recv()
		if err != nil {
			return 0, err
		}
		pc.readBuf = resp.Data
	}
	var i = copy(buf, pc.readBuf)
	if log.IsLevelEnabled(log.TraceLevel) {
		log.WithFields(log.Fields{
			"hostname":   pc.hostname,
			"readBufLen": len(pc.readBuf),
			"bufLen":     len(buf),
			"i":          i,
		}).Trace("read data from proxy conn")
	}
	pc.readBuf = pc.readBuf[i:]
	return i, nil
}

func (pc *ProxyConnection) Write(buf []byte) (int, error) {
	var err = pc.client.Send(&pf.TaskNetworkProxyRequest{
		Data: buf,
	})
	if err != nil {
		return 0, err
	}
	return len(buf), nil
}
