// Package auth issues and verifies the owner-console session token. There is
// a single operator (no user accounts to manage) so login is just "the
// correct shared password in, a signed token out."
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 24 * time.Hour

// HashPassword returns a bcrypt hash of the password, suitable for storing in
// the database. (bcrypt only uses the first 72 bytes of the input.)
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckHash reports whether pw matches the stored bcrypt hash. bcrypt's compare
// is constant-time with respect to the hash, so it doesn't leak timing.
func CheckHash(pw, hash string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
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
