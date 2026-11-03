package ciliumpool

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func TestCredentialExpiryRotationAndUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	cfg := &rest.Config{BearerTokenFile: path}
	for _, expiration := range []int64{1000, 2000} {
		token := "header." + base64.RawURLEncoding.EncodeToString(fmt.Appendf(nil, `{"exp":%d}`, expiration)) + ".signature"
		if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
			t.Fatal(err)
		}
		expiry, err := CredentialExpiry(cfg)
		if err != nil || !expiry.Equal(time.Unix(expiration, 0)) {
			t.Fatalf("rotated expiry = %s, %v", expiry, err)
		}
	}
	for _, token := range []string{"", "opaque", "header.e30.signature"} {
		expiry, err := CredentialExpiry(&rest.Config{BearerToken: token})
		if err != nil || !expiry.IsZero() {
			t.Errorf("unknown expiry for %q = %s, %v", token, expiry, err)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := CredentialExpiry(cfg); err == nil {
		t.Fatal("unreadable credential file reported success")
	}
	if _, err := CredentialExpiry(&rest.Config{TLSClientConfig: rest.TLSClientConfig{CertData: []byte("invalid")}}); err == nil {
		t.Fatal("invalid certificate reported success")
	}
}
