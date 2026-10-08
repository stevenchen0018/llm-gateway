// Package jwtauth issues and validates the JWTs that guard the admin
// management API (distinct from the gateway's API-Key auth on /v1/*).
package jwtauth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid or expired admin token")

type claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Issue signs a token whose subject is the console user's ID. Authorization
// data (role, department) is deliberately not embedded: it is re-read from
// the database on each request so changes apply immediately.
func Issue(secret string, userID int64, username string, ttl time.Duration) (string, error) {
	now := time.Now()
	c := claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
}

func Parse(secret, tokenString string) (userID int64, err error) {
	var c claims
	token, err := jwt.ParseWithClaims(tokenString, &c, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return 0, ErrInvalidToken
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrInvalidToken // e.g. tokens issued before user accounts existed
	}
	return id, nil
}
