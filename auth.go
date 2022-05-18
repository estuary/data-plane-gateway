package main

import (
	context "context"
	"fmt"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
	pb "go.gazette.dev/core/broker/protocol"

	"google.golang.org/grpc/metadata"
)

func authorized(ctx context.Context) (*AuthorizedClaims, error) {
	md, ok := metadata.FromIncomingContext(ctx)

	if !ok {
		return nil, fmt.Errorf("Authentication required: No Headers")
	}

	auth := md.Get("authorization")
	if len(auth) == 0 {
		return nil, fmt.Errorf("Authentication required: No Authorization")
	} else if len(auth[0]) == 0 {
		return nil, fmt.Errorf("Authentication required: Empty Authorization")
	} else if !strings.HasPrefix(auth[0], "Bearer ") {
		return nil, fmt.Errorf("Authentication type must be `Bearer`")
	}

	value := strings.TrimPrefix(auth[0], "Bearer ")

	return decodeJwt(value)
}

type AuthorizedClaims struct {
	Prefixes  []string `json:"prefixes"`
	Operation string   `json:"operation"`
	jwt.RegisteredClaims
}

func decodeJwt(tokenString string) (*AuthorizedClaims, error) {
	parseOpts := jwt.WithValidMethods([]string{"HS256"})
	token, err := jwt.ParseWithClaims(tokenString, new(AuthorizedClaims), func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok || token.Method.Alg() != "HS256" {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(*jwtVerificationKey), nil
	}, parseOpts)

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("Token not valid! raw_token=%v", token.Raw)
	}

	authClaims := token.Claims.(*AuthorizedClaims)

	if !authClaims.VerifyExpiresAt(time.Now(), true) {
		return nil, fmt.Errorf("Token has expired! %#v", authClaims)
	}
	if !authClaims.VerifyIssuedAt(time.Now(), true) {
		return nil, fmt.Errorf("Token has invalid issued at! %#v", authClaims)
	}

	return authClaims, nil
}

var authorizingLabels = []string{
	"name",
	"prefix",
	"estuary.dev/collection",
	"estuary.dev/task-name",
}

func enforceSelectorPrefix(claims *AuthorizedClaims, selector pb.LabelSelector) error {

	var authorizedLabels = 0

	for _, authorizingLabel := range authorizingLabels {
		for _, label := range selector.Include.Labels {
			if label.Name != authorizingLabel {
				continue
			}

			err := enforcePrefix(claims, label.Value)
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

func enforcePrefix(claims *AuthorizedClaims, name string) error {
	for _, prefix := range claims.Prefixes {
		if strings.HasPrefix(name, prefix) {
			return nil
		}
	}

	return fmt.Errorf("%v was not found in claims=%v\n", name, claims.Prefixes)
}
