package auth

import (
	context "context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/sirupsen/logrus"
	pb "go.gazette.dev/core/broker/protocol"

	"google.golang.org/grpc/metadata"
)

var (
	MissingAuthHeader   = errors.New("missing or empty Authorization header")
	InvalidAuthHeader   = errors.New("invalid Authorization header")
	UnsupportedAuthType = errors.New("invalid or unsupported Authorization header (expected 'Bearer')")
	Unauthorized        = errors.New("you are not authorized to access this resource")
)

func Authorized(ctx context.Context, jwtVerificationKey []byte) (*AuthorizedClaims, error) {
	md, ok := metadata.FromIncomingContext(ctx)

	if !ok {
		return nil, fmt.Errorf("Unauthenticated: No Headers")
	}

	auth := md.Get("authorization")
	if len(auth) == 0 {
		return nil, MissingAuthHeader
	} else if len(auth[0]) == 0 {
		return nil, MissingAuthHeader
	} else if !strings.HasPrefix(auth[0], "Bearer ") {
		return nil, UnsupportedAuthType
	}

	value := strings.TrimPrefix(auth[0], "Bearer ")

	return decodeJwt(value, jwtVerificationKey)
}

func AuthorizedReq(req *http.Request, jwtVerificationKey []byte) (*AuthorizedClaims, error) {
	auth := req.Header.Get("authorization")
	if len(auth) == 0 {
		return nil, MissingAuthHeader
	} else if !strings.HasPrefix(auth, "Bearer ") {
		return nil, InvalidAuthHeader
	}

	value := strings.TrimPrefix(auth, "Bearer ")

	var claims, err = decodeJwt(value, jwtVerificationKey)
	// The error returned from decodeJwt may contain helpful details, but we don't want to provide all those details
	// to the client. Instead we log the detailed error here and return a simpler error. This also makes it easier to
	// match errors as part of error handling.
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"host":  req.Host,
			"URI":   req.RequestURI,
			"error": err,
		}).Debug("invalid Authorization header")
		return nil, InvalidAuthHeader
	}
	return claims, nil
}

type AuthorizedClaims struct {
	Prefixes  []string `json:"prefixes"`
	Operation string   `json:"operation"`
	jwt.RegisteredClaims
}

func decodeJwt(tokenString string, jwtVerificationKey []byte) (*AuthorizedClaims, error) {
	parseOpts := jwt.WithValidMethods([]string{"HS256"})
	token, err := jwt.ParseWithClaims(tokenString, new(AuthorizedClaims), func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok || token.Method.Alg() != "HS256" {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		return jwtVerificationKey, nil
	}, parseOpts)

	if err != nil {
		return nil, fmt.Errorf("parsing jwt: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("JWT validation failed")
	}

	authClaims := token.Claims.(*AuthorizedClaims)

	if !authClaims.VerifyExpiresAt(time.Now(), true) {
		return nil, fmt.Errorf("JWT expired at %v", authClaims.ExpiresAt)
	}
	if !authClaims.VerifyIssuedAt(time.Now(), true) {
		return nil, fmt.Errorf("JWT iat is invalid: %v", authClaims.IssuedAt)
	}

	return authClaims, nil
}

var authorizingLabels = []string{
	"name",
	"prefix",
	"estuary.dev/collection",
	"estuary.dev/task-name",
}

func EnforceSelectorPrefix(claims *AuthorizedClaims, selector pb.LabelSelector) error {

	var authorizedLabels = 0

	for _, authorizingLabel := range authorizingLabels {
		for _, label := range selector.Include.Labels {
			if label.Name != authorizingLabel {
				continue
			}

			err := EnforcePrefix(claims, label.Value)
			if err != nil {
				return fmt.Errorf("unauthorized `%v` label: %w", authorizingLabel, err)
			}

			authorizedLabels++
		}
	}

	if authorizedLabels == 0 {
		return fmt.Errorf("No authorizing labels provided")
	}

	return nil
}

func EnforcePrefix(claims *AuthorizedClaims, name string) error {
	for _, prefix := range claims.Prefixes {
		if strings.HasPrefix(name, prefix) {
			return nil
		}
	}

	return Unauthorized
}
