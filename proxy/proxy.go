package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/estuary/flow/go/labels"
	pf "github.com/estuary/flow/go/protocols/flow"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	pb "go.gazette.dev/core/broker/protocol"
	pc "go.gazette.dev/core/consumer/protocol"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var NoPrimaryShards = errors.New("no primary shards")
var NoMatchingShard = errors.New("no shards matching hostname")
var PortNotPublic = errors.New("port is not public and protocol is not http")
var ProtoNotHttp = errors.New("invalid protocol for port")

// TappedListener is a net.Listener for all of the connections that are _not_ handled by the ProxyServer.
type TappedListener struct {
	cancelFunc  context.CancelFunc
	proxyServer *ProxyServer
	recv        <-chan acceptResult
}

// Accept waits for and returns the next connection to the listener.
// Connections returned by this listener will always be of type `*tls.Conn`.
func (l *TappedListener) Accept() (net.Conn, error) {
	var result = <-l.recv
	if result.err != nil {
		return nil, result.err
	}
	if result.conn != nil {
		return result.conn, nil
	}
	// If the result is zero-valued, then it means that the channel has closed
	// and we should return an EOF error to signal that this listener is done.
	return nil, io.EOF
}

type acceptResult struct {
	conn *tls.Conn
	err  error
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (l *TappedListener) Close() error {
	var err = l.proxyServer.tlsListener.Close()
	l.cancelFunc()
	return err
}

// Addr returns the listener's network address.
func (l *TappedListener) Addr() net.Addr {
	return l.proxyServer.tlsListener.Addr()
}

type ProxyServer struct {
	ctx          context.Context
	tlsListener  net.Listener
	overflow     chan<- acceptResult
	proxyHandler *ProxyHandler
	baseConfig   *tls.Config
}

// NewTlsProxyServer returns a ProxyServer, which listens for TLS connections and will handle each connection either by
// proxying to a running task's container or by passing the connection on to the associated `TappedListener`. This
// decision is based on the SNI (Server Name Indicator) in the TLS Client Hello message. Connections that are made to
// subdomains of `hostname` will be proxied to a container of the shard that's indicated by the subdomain labels.
// Connections that are made to `hostname` exactly will be given to the returned `TappedListener` to be handled by
// another server.
func NewTlsProxyServer(hostname string, port uint16, tlsCerts []tls.Certificate, shardClient pc.ShardClient, jwtVerificationKey []byte) (*ProxyServer, *TappedListener, error) {
	var tcpListener, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, nil, err
	}
	var ctx, cancelFunc = context.WithCancel(context.Background())
	var proxyHandler = newHandler(hostname, shardClient, jwtVerificationKey)
	var tlsConfig = getTlsConfig(tlsCerts, proxyHandler)

	var tlsListener = tls.NewListener(tcpListener, tlsConfig)

	var acceptCh = make(chan acceptResult)
	var server = &ProxyServer{
		ctx:          ctx,
		tlsListener:  tlsListener,
		overflow:     acceptCh,
		proxyHandler: proxyHandler,
		baseConfig:   tlsConfig,
	}
	var tappedListener = &TappedListener{
		cancelFunc:  cancelFunc,
		proxyServer: server,
		recv:        acceptCh,
	}
	return server, tappedListener, nil
}

// Run starts the server accepting and handling new connections, and blocks until
// the server stops with an error (which will never be nil).
func (ps *ProxyServer) Run() error {
	var err error
	defer func() {
		if err == nil {
			panic("ProxyServer.Run has nil terminal error")
		}
		log.Info("proxy server shutting down, sending final error to overflow listener")
		select {
		case ps.overflow <- acceptResult{err: err}:
		case <-ps.ctx.Done():
		}
		close(ps.overflow)
		log.Info("proxy server shutdown complete")
	}()
	for {
		if err = ps.ctx.Err(); err != nil {
			return err
		}
		var conn net.Conn
		conn, err = ps.tlsListener.Accept()
		if err != nil {
			log.WithField("error", err).Error("failed to accept tls connection")
			return err
		}
		// Start a new goroutine to hanlde this connection, so we don't block the accept loop
		go func() {
			// Await the completion of the TLS handshake. This is needed in order to
			// ensure that ConnectionState is populated with the SNI from the client hello.
			if hsErr := conn.(*tls.Conn).Handshake(); hsErr != nil {
				// This may
				log.WithFields(log.Fields{
					"error":      hsErr,
					"clientAddr": conn.RemoteAddr(),
				}).Warn("tls handshake error")
				return
			}
			var state = conn.(*tls.Conn).ConnectionState()

			if ps.proxyHandler.IsProxySubdomain(state.ServerName) {
				log.WithFields(log.Fields{
					"clientAddr": conn.RemoteAddr(),
					"sni":        state.ServerName,
				}).Debug("handling connection as a proxy")
				ps.proxyHandler.handleProxyConnection(context.Background(), conn.(*tls.Conn))
			} else {
				log.WithFields(log.Fields{
					"clientAddr": conn.RemoteAddr(),
					"sni":        state.ServerName,
				}).Debug("sending connection to overflow listener")
				select {
				case ps.overflow <- acceptResult{conn: conn.(*tls.Conn)}:
				case <-ps.ctx.Done():
				}

			}
		}()
	}
}

func (ps *ProxyServer) IsProxyHost(httpHostHeader string) bool {
	// remove the port from the host header value, if present
	var host = strings.SplitN(httpHostHeader, ":", 1)[0]
	return ps.proxyHandler.IsProxySubdomain(host)
}

func getTlsConfig(certs []tls.Certificate, proxyHandler *ProxyHandler) *tls.Config {

	return &tls.Config{
		// We use the same certs for all connections. We assume that this is a wildcard certificate for `*.$hostname`
		Certificates: certs,
		// NextProtos is used only for connections that are not to be proxied.
		// We need to explicitly configure support for HTTP2 here, or else it won't be offered.
		NextProtos: []string{"h2", "http/1.1"},

		// This function looks up the server's ALPN protocols
		// based on the server name given in the TLS client hello message.
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			log.WithFields(log.Fields{
				"sni":          hello.ServerName,
				"clientAddr":   hello.Conn.RemoteAddr(),
				"clientProtos": hello.SupportedProtos,
			}).Debug("got tls client hello")
			if proxyHandler.IsProxySubdomain(hello.ServerName) {
				var protos, proxyErr = proxyHandler.GetAlpnProtocols(hello)
				if proxyErr != nil {
					log.WithFields(log.Fields{
						"sni":        hello.ServerName,
						"clientAddr": hello.Conn.RemoteAddr(),
						"error":      proxyErr,
					}).Warn("error resolving sni to shard")
					ProxyConnectionRejectedCounter.Inc()
					return nil, proxyErr
				}
				return &tls.Config{
					Certificates: certs,
					// Go's tls package would normally enable session tickets and automatically rotates the keys. The problem is that
					// key rotation is tied to each individual tls config. If we want to enable session resumption that works across mutliple
					// `tls.Config` instances, we'd have to implement key rotation ourselves, because that's the way `tls.Config` is designed.
					// This might be worth doing at some point, but doesn't seem motivated right now. So we instead just disable session tickets
					// altogether for proxied connections.
					SessionTicketsDisabled: true,
					// This is the main thing we needed to override.
					NextProtos: protos,
				}, nil
			} else {
				// nil config here means to use the base config.
				// We'll accept any value for SNI for connections that aren't intended to be
				// proxied to containers. It would probably also be OK to reject any connections
				// with an SNI that doesn't match the configured hostname, but I'm also not really
				// seeing a strong reason to do so.
				return nil, nil
			}
		},
	}

}

// ProxyHandler proxies TCP traffic to connector containers, though a gRPC service in the reactor.
type ProxyHandler struct {
	hostname          string
	proxyDomainSuffix string
	mu                *sync.Mutex

	shardResolutionCache *lru.Cache[string, *resolvedShard]
	shardClient          pc.ShardClient
	jwtVerificationKey   []byte
}

func newHandler(gatewayHostname string, shardClient pc.ShardClient, jwtVerificationKey []byte) *ProxyHandler {
	var cache, err = lru.New[string, *resolvedShard](SHARD_RESOLUTION_CACHE_MAX_SIZE)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize shardResolutionCache: %v", err))
	}
	return &ProxyHandler{
		hostname:             gatewayHostname,
		proxyDomainSuffix:    "." + gatewayHostname,
		mu:                   &sync.Mutex{},
		shardResolutionCache: cache,
		shardClient:          shardClient,
		jwtVerificationKey:   jwtVerificationKey,
	}
}

func (h *ProxyHandler) IsProxySubdomain(sni string) bool {
	return strings.HasSuffix(sni, h.proxyDomainSuffix) && len(sni) > len(h.proxyDomainSuffix)
}

func (h *ProxyHandler) GetAlpnProtocols(hello *tls.ClientHelloInfo) ([]string, error) {
	if hello.ServerName == "" {
		return nil, fmt.Errorf("TLS client hello is missing SNI")
	}
	var resolved, err = h.getResolvedShard(hello.Context(), hello.ServerName, hello.Conn.RemoteAddr().String())
	if err != nil {
		return nil, err
	}
	if configuredProto := resolved.getAlpnProto(); configuredProto != "" {
		// The protocol can be comma-separated // in order to allow an h2c server running in the connector
		// to specify `h2,http/1.1`. The importance of this is questionable, so it might be something we
		// remove if we find it's not necessary.
		return strings.Split(configuredProto, ","), nil
	}
	// The port configuration from the shard does not specify an alpn protocol.
	// That's fine. From DPG's perspective, we don't care what the protocol is, or if alpn is used at all.
	// So in that case we just use whatever alpn protocol(s) were provided in the client hello.
	// In the case that the client hello specifies either 0 or 1 candidate protocol, we have no reason
	// to believe the negotiated protocol would be incorrect, regardless of what it is.
	// But if the client hello contains _multiple_ candidate protocols, there is significant
	// possibility of a mismatch between the negotiated protocol and the one that is expected
	// by the actual container. BUT, given that users won't have direct visibility to DPG logs,
	// we'll allow the connection to succeed anyway and let the container log any possible protocol errors,
	// since those will at least be visible to users.
	// We are likely to hit this warning if users fail to specify a protocol for an HTTP listener, since most
	// clients these days will offer at least `h2,http/1.1`.
	if len(hello.SupportedProtos) > 1 {
		log.WithFields(log.Fields{
			"sni":          hello.ServerName,
			"clientAddr":   hello.Conn.RemoteAddr().String(),
			"clientProtos": strings.Join(hello.SupportedProtos, ","),
		}).Warn("client ALPN supports multiple protocols, but the configuration for the port does not specify a protocol")
	}
	return hello.SupportedProtos, nil
}

type reactor struct {
	conn  *grpc.ClientConn
	count uint32
}

// This solution kind of sucks, but it's expedient: An important part of exposing
// ports is that you can set the alpn protocol. This is used during the TLS
// handshake to help clients negotiate between the various protocols that all
// operate using TLS. So we need to know which alpn protocols to offer for a given
// hostname as part of the GetConfigForClient callback. Knowing this requires
// resolving the SNI hostname to a specific shard, whose labeling whill be used
// to validate the connection request. We _also_ need to resolve that shard in
// order to know which endpoint to connect to for dialing the grpc.ClientConn. The
// problem is that there's no way to pass state between that GetConfigForClient
// callback and the rest of the process. So we have GetConfigForClient do the
// shard resolution and cache the result. GetConfigForClient will call the
// GetAlpnProtocols function in order to see which alpn protocols should be
// offered for the connection. GetAlpnProtocols will validate the hostname
// and fetch the shard (and selecting from among multiple matched shards, if
// necessary), cacheing the result. HandleProxyConnection will then lookup in the
// cache to continue the connection process. But the shard listing results include
// transient information, such as status, which cannot be cached for very long.
// The TTL on the cache then acts on a limit to the total amount of time required
// to complete the TLS handshake. Probs not a biggie, but worth knowing. The other
// thing is that it's an LRU cache so that we can be reasonably confident that
// the entries for currently handshaking connections won't get evicted in between
// these two phases under heavy load. This means that concurrent connection
// requests for _n_ distinct hosts could start to fail if n > cacheLimit. I see
// this as the failure mode just being load shedding, although it does make this
// number ...complicated.
const SHARD_RESOLUTION_CACHE_MAX_SIZE = 1024
const SHARD_RESOLUTION_CACHE_MAX_AGE = 30 * time.Second

type resolvedShard struct {
	spec       pc.ShardSpec
	labeling   labels.ShardLabeling
	route      pb.Route
	shardHost  string
	targetPort uint16
	fetchedAt  time.Time
}

func (rs *resolvedShard) expiration() time.Time {
	return rs.fetchedAt.Add(SHARD_RESOLUTION_CACHE_MAX_AGE)
}

func (rs *resolvedShard) getAlpnProto() string {
	if cfg := rs.maybePortConfig(); cfg != nil {
		return cfg.Protocol
	} else {
		return ""
	}
}

func (rs *resolvedShard) maybePortConfig() *labels.PortConfig {
	return rs.labeling.Ports[rs.targetPort]
}

// getResolvedShard returns either a non-nil resolvedShard, or an error.
func (h *ProxyHandler) getResolvedShard(ctx context.Context, sni string, clientAddr string) (*resolvedShard, error) {
	var err error
	var resolved, ok = h.shardResolutionCache.Get(sni)
	if !ok || resolved.expiration().Before(time.Now()) {
		resolved, err = h.doResolveShard(ctx, sni, clientAddr)
		if err != nil {
			// We do _not_ cache failed resolutions
			return nil, err
		}
		h.shardResolutionCache.Add(sni, resolved)
	}
	return resolved, nil
}

func (h *ProxyHandler) doResolveShard(ctx context.Context, sni string, clientAddr string) (*resolvedShard, error) {
	var query, err = h.parseServerName(sni)
	if err != nil {
		return nil, err
	}

	var queryLabels = []pb.Label{
		{Name: labels.ExposePort, Value: strconv.Itoa(int(query.port))},
		{Name: labels.Hostname, Value: query.hostname},
	}
	if query.keyBegin != "" && query.rClockBegin != "" {
		// It just so happens that the labels will still be sorted after appending these
		queryLabels = append(queryLabels,
			pb.Label{Name: labels.KeyBegin, Value: query.keyBegin},
			pb.Label{Name: labels.RClockBegin, Value: query.rClockBegin},
		)
	}
	listResp, err := h.shardClient.List(ctx, &pc.ListRequest{
		Selector: pb.LabelSelector{
			Include: pb.LabelSet{
				Labels: queryLabels,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("listing shards: %w", err)
	}
	if listResp.Status != pc.Status_OK {
		return nil, fmt.Errorf("error status when listing shards: %s", listResp.Status.String())
	}

	// If there's no shards, then immediately return
	if len(listResp.Shards) == 0 {
		return nil, NoMatchingShard
	}

	var shardsWithPrimary []int
	for i, shard := range listResp.Shards {
		if shard.Route.Primary >= 0 {
			shardsWithPrimary = append(shardsWithPrimary, i)
		}
	}
	if len(shardsWithPrimary) == 0 {
		return nil, NoPrimaryShards
	}

	var shardIndex = shardsWithPrimary[0]
	if len(shardsWithPrimary) > 1 {
		// If there's more than one matching shard having a primary assignment, then select one of them at random.
		shardIndex = shardsWithPrimary[rand.Intn(len(shardsWithPrimary))]
	}
	var shard = listResp.Shards[shardIndex]
	log.WithFields(log.Fields{
		"sni":        sni,
		"shardID":    shard.Spec.Id,
		"clientAddr": clientAddr,
	}).Debug("resolved proxy host to shard")

	var route = shard.Route
	if route.Primary < 0 {
		return nil, fmt.Errorf("no primary member available for shard: '%s'", shard.Spec.Id)
	}
	if len(route.Endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available for shard: '%s'", shard.Spec.Id)
	}
	labeling, err := labels.ParseShardLabels(shard.Spec.LabelSet)
	if err != nil {
		return nil, fmt.Errorf("parsing shard labels: %w", err)
	}

	return &resolvedShard{
		spec:       shard.Spec,
		route:      shard.Route,
		labeling:   labeling,
		shardHost:  query.hostname,
		targetPort: query.port,
		fetchedAt:  time.Now(),
	}, nil
}

func (h *ProxyHandler) handleProxyConnection(ctx context.Context, conn *tls.Conn) {
	defer conn.Close()
	var state = conn.ConnectionState()
	var sni = state.ServerName
	var clientAddr = conn.RemoteAddr().String()
	var resolved, err = h.getResolvedShard(ctx, sni, clientAddr)
	if err != nil {
		err = fmt.Errorf("resolving shard for connection attempt: %w", err)
	} else if !isHttp(state.NegotiatedProtocol) {
		// Check to see if the connection is allowed. If the protocol is http, then
		// the port visibility can either be public or private, since the http proxy
		// will enforce authZ based on the Authorization request header. But for any
		// other protocol, we'll require that the port is public
		if portConfig := resolved.maybePortConfig(); portConfig != nil {
			if !portConfig.Public {
				err = PortNotPublic
			}
		} else {
			err = PortNotPublic
		}
	}

	if err != nil {
		log.WithFields(log.Fields{
			"error":      err,
			"sni":        sni,
			"clientAddr": clientAddr,
			"proto":      state.NegotiatedProtocol,
		}).Warn("rejecting connection")
		ProxyConnectionRejectedCounter.Inc()
		return
	}

	var portStr = strconv.Itoa(int(resolved.targetPort))
	shardID := resolved.spec.Id.String()
	ProxyConnectionsAcceptedCounter.WithLabelValues(shardID, portStr).Inc()
	if err := h.proxyConnection(ctx, conn, sni, clientAddr, resolved); err != nil {
		ProxyConnectionsClosedCounter.WithLabelValues(shardID, portStr, "error").Inc()
		log.WithFields(log.Fields{
			"error":      err,
			"sni":        sni,
			"clientAddr": clientAddr,
		}).Warn("failed to proxy connection")
	} else {
		ProxyConnectionsClosedCounter.WithLabelValues(shardID, portStr, "ok").Inc()
		log.WithFields(log.Fields{
			"sni":        sni,
			"clientAddr": clientAddr,
		}).Info("finished proxy connection")
	}
}

func (h *ProxyHandler) proxyConnection(ctx context.Context, conn *tls.Conn, sni string, clientAddr string, resolved *resolvedShard) error {
	shardID := resolved.spec.Id.String()
	var endpoint = resolved.route.Endpoints[resolved.route.Primary]
	reactorAddr := endpoint.GRPCAddr()
	log.WithFields(log.Fields{
		"sni":         sni,
		"reactorAddr": reactorAddr,
	}).Info("starting proxy connection")
	// Ideally we'd reuse an existing connection that's cached per reactor.
	// I don't think that's necessary at this stage, though.
	proxyConn, err := grpc.DialContext(ctx, reactorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connecting to reactor: %w", err)
	}
	var proxyClient = pf.NewNetworkProxyClient(proxyConn)

	proxyStreaming, err := proxyClient.Proxy(ctx)
	if err != nil {
		return fmt.Errorf("starting proxy RPC: %w", err)
	}
	defer proxyStreaming.CloseSend()

	if err = proxyStreaming.Send(&pf.TaskNetworkProxyRequest{
		Open: &pf.TaskNetworkProxyRequest_Open{
			ShardId:    resolved.spec.Id,
			TargetPort: uint32(resolved.targetPort),
			ClientAddr: conn.RemoteAddr().String(),
		},
	}); err != nil {
		return fmt.Errorf("sending Open message: %w", err)
	}

	openResp, err := proxyStreaming.Recv()
	if err != nil {
		return fmt.Errorf("receiving opened response: %w", err)
	}
	if err = validateOpenResponse(openResp.OpenResponse); err != nil {
		return err
	}

	var proxyConnnection = &ProxyConnection{
		hostname:   sni,
		taskName:   resolved.labeling.TaskName,
		shardID:    shardID,
		targetPort: resolved.targetPort,
		client:     proxyStreaming,
	}
	var negotiatedProto = conn.ConnectionState().NegotiatedProtocol
	log.WithFields(log.Fields{
		"sni":        sni,
		"clientAddr": clientAddr,
		"proto":      negotiatedProto,
	}).Debug("starting to proxy connection data")

	// We're finally ready to copy the data between the connection and our grpc streaming rpc.
	if isHttp(negotiatedProto) {
		return h.proxyHttp(ctx, conn, proxyConnnection, resolved.maybePortConfig())
	} else {
		return proxyTcp(ctx, conn, proxyConnnection)
	}
}

func proxyTcp(ctx context.Context, clientConn *tls.Conn, proxyConn *ProxyConnection) error {

	grp, ctx := errgroup.WithContext(ctx)
	grp.Go(func() error {
		if outgoingBytes, e := io.Copy(clientConn, proxyConn); !errors.Is(e, io.EOF) {
			log.WithFields(log.Fields{
				"hostname":      proxyConn.hostname,
				"error":         e,
				"outgoingBytes": outgoingBytes,
			}).Warn("copyProxyResponseData completed with error")
			return e
		} else {
			log.WithFields(log.Fields{
				"hostname":      proxyConn.hostname,
				"outgoingBytes": outgoingBytes,
			}).Debug("copyProxyResponseData completed successfully")
			return nil
		}
	})

	grp.Go(func() error {
		defer proxyConn.Close()
		if incomingBytes, e := io.Copy(proxyConn, clientConn); !errors.Is(e, io.EOF) {
			log.WithFields(log.Fields{
				"hostname":      proxyConn.hostname,
				"error":         e,
				"incomingBytes": incomingBytes,
			}).Warn("copyProxyRequestData completed with error")
			return e
		} else {
			log.WithFields(log.Fields{
				"hostname":      proxyConn.hostname,
				"incomingBytes": incomingBytes,
			}).Warn("copyProxyRequestData completed successfully")
			return nil
		}
	})

	var err = grp.Wait()
	log.WithField("error", err).Debug("finished proxy connection")
	return err
}

func validateOpenResponse(resp *pf.TaskNetworkProxyResponse_OpenResponse) error {
	if resp == nil {
		return fmt.Errorf("missing open response")
	}
	if resp.Status != pf.TaskNetworkProxyResponse_OK {
		return fmt.Errorf("open response status (%s) not OK", resp.Status)
	}
	return nil
}

type shardQuery struct {
	hostname string
	port     uint16
	// We don't bother to parse out the keyBegin and rClockBegin fields
	// because we'd only need to convert them to strings again to use as
	// label selectors.
	keyBegin    string
	rClockBegin string
}

func (h *ProxyHandler) parseServerName(sni string) (*shardQuery, error) {
	var shardAndPort, domainSuffix, ok = strings.Cut(sni, ".")
	if !ok {
		return nil, fmt.Errorf("sni does not have enough components")
	}
	if domainSuffix != h.hostname {
		return nil, fmt.Errorf("invalid sni does not match domain suffix")
	}
	if len(shardAndPort) == 0 {
		return nil, fmt.Errorf("invalid sni contains empty label")
	}

	var query = &shardQuery{}
	var parts = strings.Split(shardAndPort, "-")
	var portStr string
	if len(parts) == 2 {
		query.hostname = parts[0]
		portStr = parts[1]
	} else if len(parts) == 4 {
		query.hostname = parts[0]
		query.keyBegin = parts[1]
		query.rClockBegin = parts[2]
		portStr = parts[3]
	} else {
		return nil, fmt.Errorf("invalid subdomain")
	}

	var targetPort, err = strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("parsing subdomain port number: %w", err)
	}
	query.port = uint16(targetPort)

	return query, nil
}

var ProxyConnectionsAcceptedCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "net_proxy_conns_accept_total",
	Help: "counter of proxy connections that have been accepted",
}, []string{"shard", "port"})
var ProxyConnectionsClosedCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "net_proxy_conns_closed_total",
	Help: "counter of proxy connections that have completed and closed",
}, []string{"shard", "port", "status"})

var ProxyConnectionRejectedCounter = promauto.NewCounter(prometheus.CounterOpts{
	Name: "net_proxy_conns_reject_total",
	Help: "counter of proxy connections that have been rejected due to error or invalid sni",
})

var ProxyConnBytesInboundCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "net_proxy_conn_inbound_bytes_total",
	Help: "total bytes proxied from client to container",
}, []string{"shard", "port"})
var ProxyConnBytesOutboundCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "net_proxy_conn_outbound_bytes_total",
	Help: "total bytes proxied from container to client",
}, []string{"shard", "port"})
