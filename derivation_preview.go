package main

import (
	context "context"
	"fmt"
	"io"
	"net/http"
	"encoding/json"
	"bytes"

	"github.com/estuary/data-plane-gateway/auth"
	"github.com/urfave/negroni"
)

func NewDerivationPreviewServer(ctx context.Context) http.Handler {
	previewHandler := negroni.Classic()
	previewHandler.Use(negroni.HandlerFunc(cors))
	previewHandler.UseHandler(derivationPreviewHandler)

	return previewHandler
}

type PreviewRequest struct {
	DraftId      string `json:"draft_id"`
	Collection   string `json:"collection"`
	NumDocuments int    `json:"num_documents"`
}

// Will be used with both http and https
// Inspired partially by https://gist.github.com/yowu/f7dc34bd4736a65ff28d
var derivationPreviewHandler = http.HandlerFunc(func(writer http.ResponseWriter, proxy_req *http.Request) {
	// Do auth
	// Pull JWT from authz header
	// See auth.go:authorized()
	// decodeJWT(that bearer token) -> AuthorizedClaims
	claims, err := auth.AuthenticateHttpReq(proxy_req, []byte(*jwtVerificationKey))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusUnauthorized)
		return
	}

	var req PreviewRequest

	if reqBytes, err := io.ReadAll(proxy_req.Body); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	} else if err := json.Unmarshal(reqBytes, &req); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	authorization_error := auth.EnforcePrefix(claims, req.Collection)

	// enforcePrefix(claims, collection_name)
	// collection_name comes from actual preview request
	if authorization_error != nil {
		http.Error(writer, authorization_error.Error(), http.StatusForbidden)
		return
	}

	// Call preview
	reqBytes, err := json.Marshal(req)
	var reqReader = bytes.NewReader(reqBytes)

	httpRequest, err := http.NewRequest("POST", fmt.Sprintf("http://%s/preview", *previewAddr), reqReader)
	if err != nil {
		http.Error(writer, fmt.Errorf("creating request to be sent to derivation preview: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	httpRequest.Header.Add("content-type", "application/json")
	httpRequest.Header.Add("authorization", proxy_req.Header.Get("authorization"))

	var httpClient = http.Client{}
	preview_response, preview_error := httpClient.Do(httpRequest)

	if preview_error != nil {
		// An error is returned if there were too many redirects or if there was an HTTP protocol error.
		// A non-2xx response doesn't cause an error.
		http.Error(writer, preview_error.Error(), http.StatusInternalServerError)
		return
	}

	defer preview_response.Body.Close()
	// Return result

	copyHeader(writer.Header(), preview_response.Header)
	writer.WriteHeader(preview_response.StatusCode)
	io.Copy(writer, preview_response.Body)
})
