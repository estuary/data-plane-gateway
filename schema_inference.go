package main

import (
	context "context"
	"fmt"
	"io"
	"net/http"

	"github.com/estuary/data-plane-gateway/auth"
	"github.com/urfave/negroni"
)

func NewSchemaInferenceServer(ctx context.Context) http.Handler {
	inferenceHandler := negroni.Classic()
	inferenceHandler.Use(negroni.HandlerFunc(cors))
	inferenceHandler.UseHandler(schemaInferenceHandler)

	return inferenceHandler
}

// Will be used with both http and https
// Inspired partially by https://gist.github.com/yowu/f7dc34bd4736a65ff28d
var schemaInferenceHandler = http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
	// Do auth
	// Pull JWT from authz header
	// See auth.go:authorized()
	// decodeJWT(that bearer token) -> AuthorizedClaims
	claims, err := auth.AuthorizedReq(req, []byte(*jwtVerificationKey))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusUnauthorized)
		return
	}

	collection_name := req.URL.Query().Get("collection")
	authorization_error := auth.EnforcePrefix(claims, collection_name)

	// enforcePrefix(claims, collection_name)
	// collection_name comes from actual inference request
	if authorization_error != nil {
		http.Error(writer, authorization_error.Error(), http.StatusForbidden)
		return
	}

	// TODO: rename the argument to `?collection=...` in the schema inference service, then get rid of this:
	args := req.URL.Query()
	args.Set("collection_name", args.Get("collection"))
	args.Del("collection")

	// Call inference
	inference_response, inference_error := http.Get(fmt.Sprintf("http://%s/infer_schema?%s", *inferenceAddr, args.Encode()))

	if inference_error != nil {
		// An error is returned if there were too many redirects or if there was an HTTP protocol error.
		// A non-2xx response doesn't cause an error.
		http.Error(writer, inference_error.Error(), http.StatusInternalServerError)
		return
	}

	defer inference_response.Body.Close()
	// Return result

	copyHeader(writer.Header(), inference_response.Header)
	writer.WriteHeader(inference_response.StatusCode)
	io.Copy(writer, inference_response.Body)
})
