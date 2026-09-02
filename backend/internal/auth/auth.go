// Package auth issues and verifies console session tokens. The console has a
// single admin login (one deployment serves one business), so a signed JWT
// carries nothing but that admin's username — there are no roles or tenants to
// authorize against.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 24 * time.Hour

// Claims is the payload of a session token: the standard registered claims
// (expiry, issued-at, subject) plus the admin's username.
type Claims struct {
	jwt.RegisteredClaims
	Username string `json:"username"`
}

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

// IssueToken returns a signed session token identifying the admin, valid for
// sessionTTL.
func IssueToken(secret, username string) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(sessionTTL)),
		},
		Username: username,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseToken checks the signature and expiry of a session token and returns its
// claims.
func ParseToken(token, secret string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		return nil, errors.New("invalid or expired session")
	}
	return claims, nil
}
