package keyvault

import (
	"context"
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLatestTLSCert(t *testing.T) {
	tests := []struct {
		name        string
		certPath    string
		contentType string
	}{
		{
			name:        "pem encoding",
			certPath:    "testdata/dummy.pem",
			contentType: pemContentType,
		},
		{
			name:        "pfx encoding",
			certPath:    "testdata/dummy.pfx",
			contentType: pkcs12ContentType,
		},
	}

	for _, ts := range tests {
		ts := ts
		t.Run(ts.name, func(t *testing.T) {
			kvc := Shim{sf: newFakeSecretFetcher(ts.certPath, ts.contentType)}

			cert, err := kvc.GetLatestTLSCertificate(context.TODO(), "dummy")
			require.NoError(t, err)
			assert.NotNil(t, cert.Leaf)
		})
	}
}

type fakeSecretFetcher struct {
	certPath    string
	contentType string
}

func newFakeSecretFetcher(certPath, contentType string) *fakeSecretFetcher {
	return &fakeSecretFetcher{certPath: certPath, contentType: contentType}
}

func (f *fakeSecretFetcher) GetSecret(_ context.Context, _, _ string, _ *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
	bs, err := os.ReadFile(f.certPath)
	if err != nil {
		return azsecrets.GetSecretResponse{}, errors.Wrap(err, "could not read file")
	}
	v := string(bs)
	resp := azsecrets.GetSecretResponse{
		SecretBundle: azsecrets.SecretBundle{
			ContentType: &f.contentType,
			Value:       &v,
		},
	}
	return resp, nil
}
