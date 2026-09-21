package utils

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type JWTPayload struct {
	ID     uuid.UUID `json:"id"`
	UserID int32     `json:"user_id"`
	Email  string    `json:"email"`
	jwt.RegisteredClaims
}

func CreateToken(tokenID uuid.UUID, userID int32, email string, duration time.Duration, secretKey string) (string, *JWTPayload, error) {
	payload := &JWTPayload{
		ID:     tokenID,
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)
	signedToken, err := token.SignedString([]byte(secretKey))
	if err != nil {
		return "", nil, err
	}

	return signedToken, payload, nil
}

func VerifyToken(token string, secretKey string) (*JWTPayload, error) {
	parsedToken, err := jwt.ParseWithClaims(token, &JWTPayload{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secretKey), nil
	})

	if err != nil {
		return nil, err
	}

	payload, ok := parsedToken.Claims.(*JWTPayload)
	if !ok || !parsedToken.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return payload, nil
}
