package enablebanking

import (
	"crypto/rsa"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwtIssuer   = "enablebanking.com"
	jwtAudience = "api.enablebanking.com"
	jwtLifetime = time.Hour
	jwtRefresh  = 5 * time.Minute
)

// signer issues RS256 application tokens and reuses one until shortly before
// it expires.
type signer struct {
	appID string
	key   *rsa.PrivateKey
	now   func() time.Time

	mu      sync.Mutex
	token   string
	expires time.Time
}

func (s *signer) Token() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if s.token != "" && now.Before(s.expires.Add(-jwtRefresh)) {
		return s.token, nil
	}
	expires := now.Add(jwtLifetime)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Issuer:    jwtIssuer,
		Audience:  jwt.ClaimStrings{jwtAudience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expires),
	})
	tok.Header["typ"] = "JWT"
	tok.Header["kid"] = s.appID
	signed, err := tok.SignedString(s.key)
	if err != nil {
		return "", errSigning
	}
	s.token, s.expires = signed, expires
	return signed, nil
}
