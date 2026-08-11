package jwt

import (
	"errors"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid or expired access token")

type Issuer struct {
	secret []byte
	ttl    time.Duration
}

func NewIssuer(secret string, ttl time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: ttl}
}

type claims struct {
	jwtlib.RegisteredClaims
}

func (i *Issuer) IssueAccessToken(userID uuid.UUID) (string, error) {
	now := time.Now()
	c := claims{
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(now.Add(i.ttl)),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, c)
	return token.SignedString(i.secret)
}

func (i *Issuer) ParseAccessToken(tokenString string) (uuid.UUID, error) {
	var c claims
	token, err := jwtlib.ParseWithClaims(tokenString, &c, func(t *jwtlib.Token) (interface{}, error) {
		return i.secret, nil
	})
	if err != nil || !token.Valid {
		return uuid.UUID{}, ErrInvalidToken
	}

	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.UUID{}, ErrInvalidToken
	}
	return userID, nil
}
