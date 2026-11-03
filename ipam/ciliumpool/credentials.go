package ciliumpool

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"k8s.io/client-go/rest"
)

var errCredentialExpiry = errors.New("ciliumpool: invalid credential expiry")

// CredentialExpiry observes configured credentials, not their authorization.
// Exec/auth-provider credentials have no stable expiry surface here.
func CredentialExpiry(cfg *rest.Config) (time.Time, error) {
	if cfg.ExecProvider != nil || cfg.AuthProvider != nil {
		return time.Time{}, nil
	}
	var expires time.Time
	certData := cfg.CertData
	if len(certData) == 0 && cfg.CertFile != "" {
		var err error
		certData, err = os.ReadFile(cfg.CertFile)
		if err != nil {
			return time.Time{}, fmt.Errorf("reading client certificate expiry: %w", err)
		}
	}
	if len(certData) != 0 {
		block, _ := pem.Decode(certData)
		if block == nil {
			return time.Time{}, fmt.Errorf("%w: client certificate is not PEM encoded", errCredentialExpiry)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return time.Time{}, fmt.Errorf("parsing client certificate expiry: %w", err)
		}
		expires = cert.NotAfter
	}
	token := cfg.BearerToken
	if cfg.BearerTokenFile != "" {
		raw, err := os.ReadFile(cfg.BearerTokenFile)
		if err != nil {
			return time.Time{}, fmt.Errorf("reading token expiry: %w", err)
		}
		token = strings.TrimSpace(string(raw))
	}
	if token != "" {
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			// Opaque tokens do not provide an observable expiry.
			return time.Time{}, nil
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("%w: decoding token expiry", errCredentialExpiry)
		}
		var claims struct {
			Expiry int64 `json:"exp"`
		}
		if err = json.Unmarshal(payload, &claims); err != nil {
			return time.Time{}, fmt.Errorf("%w: parsing token expiry", errCredentialExpiry)
		}
		if claims.Expiry <= 0 {
			return time.Time{}, nil
		}
		tokenExpiry := time.Unix(claims.Expiry, 0)
		if expires.IsZero() || tokenExpiry.Before(expires) {
			expires = tokenExpiry
		}
	}
	return expires, nil
}
