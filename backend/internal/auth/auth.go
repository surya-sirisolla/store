// Package auth issues and verifies the owner-console session token. There is
// a single operator (no user accounts to manage) so login is just "the
// correct shared password in, a signed token out."
package auth

import (
	"crypto/subtle"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const sessionTTL = 24 * time.Hour

// CheckPassword compares the supplied password against the configured admin
// password in constant time, to avoid leaking length/prefix via timing.
func CheckPassword(supplied, configured string) bool {
	if configured == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(supplied), []byte(configured)) == 1
}

// IssueToken returns a signed session token valid for sessionTTL.
func IssueToken(secret string) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   "owner",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(sessionTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// VerifyToken checks the signature and expiry of a session token.
func VerifyToken(token, secret string) error {
	parsed, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		return errors.New("invalid or expired session")
	}
	return nil
}
