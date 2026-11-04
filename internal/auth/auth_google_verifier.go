package auth

import (
	"context"
	"fmt"

	"nihon-no-hikari-api/pkg/utils"

	"google.golang.org/api/idtoken"
)

// GoogleClaims holds the verified identity claims from a Google ID token.
type GoogleClaims struct {
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

// GoogleVerifier validates a Google ID token and returns its claims.
type GoogleVerifier interface {
	Verify(ctx context.Context, idToken string) (GoogleClaims, error)
}

type googleVerifier struct {
	clientID string
}

// NewGoogleVerifier returns a verifier bound to the given OAuth client ID (token audience).
// An empty client ID yields a verifier that always reports utils.ErrGoogleSignInDisabled.
func NewGoogleVerifier(clientID string) GoogleVerifier {
	return &googleVerifier{clientID: clientID}
}

// Verify implements [GoogleVerifier].
func (v *googleVerifier) Verify(ctx context.Context, token string) (GoogleClaims, error) {
	if v.clientID == "" {
		return GoogleClaims{}, utils.ErrGoogleSignInDisabled
	}

	payload, err := idtoken.Validate(ctx, token, v.clientID)
	if err != nil {
		return GoogleClaims{}, fmt.Errorf("%w: %v", utils.ErrInvalidGoogleToken, err)
	}

	claims := GoogleClaims{Sub: payload.Subject}
	claims.Email, _ = payload.Claims["email"].(string)
	claims.EmailVerified, _ = payload.Claims["email_verified"].(bool)
	claims.Name, _ = payload.Claims["name"].(string)
	claims.Picture, _ = payload.Claims["picture"].(string)

	if claims.Sub == "" || claims.Email == "" {
		return GoogleClaims{}, utils.ErrInvalidGoogleToken
	}
	return claims, nil
}
