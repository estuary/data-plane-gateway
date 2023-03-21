package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/estuary/data-plane-gateway/auth"
	"github.com/estuary/flow/go/labels"
	log "github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"golang.org/x/net/http2"
)

func (h *ProxyHandler) proxyHttp(ctx context.Context, clientConn *tls.Conn, proxyConn *ProxyConnection, portConfig *labels.PortConfig) error {
	defer proxyConn.Close()
	// We generally assume that the upstream connector container wishes to speak http/1.1, unless they explicitly request to use _only_ h2.
	var useHttp2Upstream = portConfig != nil && portConfig.Protocol == "h2"
	var isPublicPort = portConfig != nil && portConfig.Public

	var targetScheme = "http"
	if useHttp2Upstream {
		targetScheme = "https"
	}
	var proxy = httputil.ReverseProxy{
		Transport:     proxyConn.singleConnectionTransport(useHttp2Upstream),
		FlushInterval: 0,
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			log.WithFields(log.Fields{
				"hostname":   proxyConn.hostname,
				"remoteAddr": clientConn.RemoteAddr().String(),
				"shardID":    proxyConn.shardID,
				"error":      err,
				"URI":        req.RequestURI,
			}).Error("proxy error")
			handleHttpError(err, w, req)
		},

		Director: func(req *http.Request) {
			req.URL.Host = proxyConn.hostname
			req.URL.Scheme = targetScheme
			if _, ok := req.Header["User-Agent"]; !ok {
				// explicitly disable User-Agent so it's not set to default value
				req.Header.Set("User-Agent", "")

			}
			// if the port is private, then scrub the authentication token from the requests.
			if !isPublicPort {
				req.Header.Del("Authorization")
				// There's no `DeleteCookie` function, so we parse them, delete them all, and
				// add them back in while filtering out the flow_auth cookie.
				var cookies = req.Cookies()
				req.Header.Del("Cookie")
				for _, cookie := range cookies {
					if cookie.Name != auth.AuthCookieName {
						req.AddCookie(cookie)
					}
				}
			}
		},
	}

	var handlerFunc = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// If the port is private, then require that each request has a JWT that permits it to
		// access the task. We don't check the Authorization header if the port is public, since the header value
		// might be meant to be interpreted by the connector itself.
		if !isPublicPort {
			var claims, authErr = auth.AuthenticateHttpReq(req, h.jwtVerificationKey)
			if authErr == nil {
				authErr = auth.EnforcePrefix(claims, proxyConn.taskName)
			}
			var acceptHeader = req.Header.Get("accept")

			// The port is private and the request is unauthorized.
			// We might redirect the user to the dashboard so that they can authorize the request and get redirected back here.
			// Or, this request might be the _result_ of a successful redirect back from that endpoint, in which case we'll
			// redirect _again_, with a `Set-Cookie` header to make sure that the next request is authorized. But this type
			// of redirection only makes sense if this request originated from an interactive browser session. For example,
			// it wouldn't make sense to respond with a redirect to dashboard if this we a JSON API request. In that case, we'd
			// prefer to simply return a JSON response with the error message. Checking if the accept header contains "html"
			// just seemed like a cheap and easy way to determine if a redirect is likely to be appreciated, since actually parsing
			// the accept header is pretty complicated.
			if authErr != nil && strings.Contains(acceptHeader, "html") {
				// Note that we only match this path if the request doesn't already contain a valid auth token. Technically, a
				// connector could itself expose an `/auth-redirect` endpoint, and that would work as long as the request can be authorized.
				if req.URL.Path == "/auth-redirect" {
					h.redirectHandler.ServeHTTP(w, req)
					return
				} else {
					// This is just a regular request that's unauthorized, so we'll handle this by redirecting to the dashboard.
					var origUrl = "https://" + req.Host + req.URL.Path
					var redirectTarget = h.controlPlaneAuthUrl.JoinPath("/data-plane-auth-req")
					var query = &url.Values{}
					query.Add("orig_url", origUrl)
					query.Add("prefix", proxyConn.taskName)
					redirectTarget.RawQuery = query.Encode()

					var targetUrl = redirectTarget.String()
					log.WithFields(log.Fields{
						"error":          authErr,
						"host":           req.Host,
						"clientAddr":     req.RemoteAddr,
						"reqUrl":         origUrl,
						"redirectTarget": targetUrl,
					}).Info("HTTP proxy request to private port is unauthorized")

					http.Redirect(w, req, targetUrl, 307)
					return
				}
			} else if authErr != nil {
				handleHttpError(authErr, w, req)
			}
		}
		proxy.ServeHTTP(w, req)
	})

	// These timeouts seemed like reasonable starting points, and haven't been very
	// carefully considered. But better arbitrary timeouts than no timeouts at all!
	if clientConn.ConnectionState().NegotiatedProtocol == "h2" {
		var h2Server = http2.Server{
			IdleTimeout: 10 * time.Second,
		}
		// The clientConn will be closed automatically by ServeConn, but we'll need to close the proxyConn ourselves
		h2Server.ServeConn(clientConn, &http2.ServeConnOpts{
			Context: ctx,
			Handler: handlerFunc,
			BaseConfig: &http.Server{
				IdleTimeout:  10 * time.Second,
				ReadTimeout:  20 * time.Second,
				WriteTimeout: 20 * time.Second,
			},
		})
		return nil
	} else {
		// We'll be speaking http/1.1, which requires a 3rd party library because there's no ServeConn function in Go's http package.
		var server = fasthttp.Server{
			Handler:      fasthttpadaptor.NewFastHTTPHandler(handlerFunc),
			Name:         fmt.Sprintf("%s (%s)", proxyConn.hostname, clientConn.RemoteAddr().String()),
			IdleTimeout:  10 * time.Second,
			ReadTimeout:  20 * time.Second,
			WriteTimeout: 20 * time.Second,
		}

		return server.ServeConn(clientConn)
	}
}

var errTemplate = template.Must(template.New("proxy-error").Parse(`<!DOCTYPE html>
<html>
	<head>
	    <title>Error</title>
		<style>
			html {
				height: 100%;
				display: table;
				margin: auto;
			}
			body {
				height: 100%;
				display: table-cell;
				vertical-align: middle;
				background-color: white;
			}
		</style>
	</head>
	<body>
		<span style='font-size: 40px; color: black; font-family:Arial,Helvetica,sans-serif;'>{{.}}</span>
	</body>
</html>`))

func handleHttpError(err error, w http.ResponseWriter, r *http.Request) {
	var body []byte
	var contentType string
	var status = httpStatus(err)

	// If the error was caused by an issue with the upstream connection, then we must
	// ask the client to close the connection and create a new one. This is important
	// because the http proxy handler isn't currently able to re-establish connections
	// in response to them breaking. So if the client continues to use this connection,
	// then they will continue to get 5xx errors due to the broken upstream connection.
	// Note that, while the `Connection` header is not compatible with http2, the Go
	// http2 package seems to handle this by removing the header and sending a GOAWAY.
	// In fact, this is the _only_ way I can figure out how to send a GOAWAY from a handler.
	if status >= 500 {
		w.Header().Add("Connection", "close")
	}

	var headers = r.Header["Accept"]
	var accept string
	if len(headers) > 0 {
		accept = headers[0]
	}
	if strings.Contains(accept, "json") {
		body, _ = json.Marshal(map[string]interface{}{
			"error": err.Error(),
		})
		contentType = "application/json"
	} else if strings.Contains(accept, "html") {
		var buf bytes.Buffer
		if templateErr := errTemplate.Execute(&buf, err.Error()); templateErr != nil {
			log.WithFields(log.Fields{
				"origError":     err.Error(),
				"templateError": templateErr.Error(),
			}).Error("error rendering html error template")
		}
		body = buf.Bytes()
		contentType = "text/html"
	} else {
		// just render as plain text
		body = []byte(fmt.Sprintf("Error: %s", err))
		contentType = "text/plain"
	}

	w.Header().Add("Content-Type", contentType)
	w.Header().Add("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	var _, writeErr = w.Write(body)
	if writeErr != nil {
		log.WithFields(log.Fields{
			"origError":     err.Error(),
			"templateError": writeErr.Error(),
		}).Warn("failed to write error response body")
	}
}

func httpStatus(err error) int {
	if err == NoMatchingShard {
		return 404
	} else if err == auth.InvalidAuthToken || err == auth.UnsupportedAuthType {
		return 400
	} else if err == auth.MissingAuthToken {
		return 401
	} else if err == auth.Unauthorized {
		// In this case, the user provided a valid auth token, which just didn't authorize them to access the shard. We return a 403
		// instead of a 404 because we can have _a little_ more trust in an authenticated user, and thus provide them with more
		// specific and helpful information.
		return 403
	} else {
		return 503
	}
}

func isHttp(negotiatedProto string) bool {
	return negotiatedProto == "h2" || negotiatedProto == "http/1.1"
}
