package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/estuary/data-plane-gateway/auth"
	"github.com/sirupsen/logrus"
)

// authRedirectHandler is used in the authentication flow for accessing private ports throught the proxy.
// It handles requests to `/auth-redirect` endpoints after users have successfully acquired an auth token
// from the dashboard (control-plane). It expects the request URI to contain `token` and `orig_url`
// parameters. Assuming the request and token are valid, the user will be redirected to `orig_url`
// with a cookie that contains their auth token.
type authRedirectHandler struct {
	dpgDomainSuffix string
}

func newRedirectHandler(dpgHostname string) *authRedirectHandler {
	return &authRedirectHandler{
		dpgDomainSuffix: "." + dpgHostname,
	}
}

func (h *authRedirectHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var params = req.URL.Query()
	// Note that we don't do any validation of the token here. If it's not valid, then we'll
	// catch it when the browser requests the new location, and can handle it then.
	var token = params.Get("token")
	if token == "" {
		renderAuthError(errors.New("url is missing the token parameter"), w, req)
		return
	}
	var origUrl = params.Get("orig_url")
	if origUrl == "" {
		renderAuthError(errors.New("url is missing the orig_url parameter"), w, req)
		return
	}
	var origUrlParsed, err = url.Parse(origUrl)
	if err != nil {
		renderAuthError(fmt.Errorf("invalid orig_url parameter: %w", err), w, req)
		return
	}

	// Check that the hostname of the original url is actually a subdomain of DPG's hostname.
	// This isn't technically neccessary for security because the cookie is scoped to a single
	// origin. But it makes sense to fail fast if we can.
	if !strings.HasSuffix(origUrlParsed.Hostname(), h.dpgDomainSuffix) {
		renderAuthError(fmt.Errorf("invalid orig_url parameter: hostname '%s' is not a subdomain of %s", origUrlParsed.Hostname(), h.dpgDomainSuffix), w, req)
		return
	}

	var cookie = &http.Cookie{
		Name:     auth.AuthCookieName,
		Value:    token,
		Secure:   true,
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, req, origUrl, 307)
}

// renderAuthError writes an html error response page with a 400 status. We don't redirect
// these back to the dashboard in order to prevent an infinite loop of redirects.
// This doesn't handle multiple content types because this is only expected to be used
// with interactive sessions (html).
func renderAuthError(err error, w http.ResponseWriter, r *http.Request) {
	var body []byte
	var contentType string

	var buf bytes.Buffer
	if templateErr := errTemplate.Execute(&buf, err.Error()); templateErr != nil {
		logrus.WithFields(logrus.Fields{
			"origError":     err.Error(),
			"templateError": templateErr.Error(),
		}).Error("error rendering html error template")
	}
	body = buf.Bytes()
	contentType = "text/html"

	w.Header().Add("Content-Type", contentType)
	w.Header().Add("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(400)
	var _, writeErr = w.Write(body)
	if writeErr != nil {
		logrus.WithFields(logrus.Fields{
			"origError":     err.Error(),
			"templateError": writeErr.Error(),
		}).Warn("failed to write error response body")
	}
}
