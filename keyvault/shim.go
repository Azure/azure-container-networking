package keyvault

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
	"github.com/pkg/errors"
	"golang.org/x/crypto/pkcs12"
)

const (
	pemContentType    = "application/x-pem-file"
	pkcs12ContentType = "application/x-pkcs12"
)

type secretFetcher interface {
	GetSecret(ctx context.Context, secretName, version string, opts *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
}

// Shim provides convenience methods for working with KeyVault.
type Shim struct {
	sf secretFetcher
}

// NewShim constructs a Shim for a KeyVault instance located at the provided url. The azcore.TokenCredential will
// only be used during method calls, it is not verified at initialization.
func NewShim(vaultURL string, cred azcore.TokenCredential) (*Shim, error) {
	c, err := azsecrets.NewClient(vaultURL, cred, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not create new azsecrets.Client")
	}

	return &Shim{sf: c}, nil
}

// GetLatestTLSCertificate fetches the latest version of a keyvault certificate and transforms it into a usable tls.Certificate.
func (s *Shim) GetLatestTLSCertificate(ctx context.Context, certName string) (tls.Certificate, error) {
	resp, err := s.sf.GetSecret(ctx, certName, "", nil)
	if err != nil {
		return tls.Certificate{}, errors.Wrap(err, "could not get secret")
	}

	pemBlocks, err := getPEMBlocks(*resp.ContentType, *resp.Value)
	if err != nil {
		return tls.Certificate{}, errors.Wrap(err, "could not get pem blocks")
	}

	var (
		key       crypto.PrivateKey
		leaf      *x509.Certificate
		leafBytes []byte
		cas       [][]byte
	)

	for _, v := range pemBlocks {
		switch {
		case strings.Contains(v.Type, "PRIVATE KEY"):
			key, err = parsePrivateKey(v.Bytes)
			if err != nil {
				return tls.Certificate{}, errors.Wrap(err, "could not parse private key")
			}
		case strings.Contains(v.Type, "CERTIFICATE"):
			c, err := x509.ParseCertificate(v.Bytes)
			if err != nil {
				return tls.Certificate{}, errors.Wrap(err, "could not parse certificate")
			}
			if !c.IsCA {
				leaf = c
				leafBytes = v.Bytes
				continue
			}
			cas = append(cas, v.Bytes)
		}
	}

	if leaf == nil {
		return tls.Certificate{}, errors.New("could not find leaf certificate")
	}

	return tls.Certificate{
		PrivateKey:  key,
		Certificate: append([][]byte{leafBytes}, cas...),
		Leaf:        leaf,
	}, nil
}

func getPEMBlocks(contentType, payload string) ([]*pem.Block, error) {
	switch contentType {
	case pkcs12ContentType:
		return handlePFXBytes(payload)
	case pemContentType:
		return handlePEMBytes(payload)
	}
	return nil, errors.Errorf("unsupported content type %s", contentType)
}

func handlePFXBytes(v string) ([]*pem.Block, error) {
	pfxBytes, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, errors.Wrap(err, "could not base64 decode keyvault.SecretBundle.Value")
	}

	bl, err := pkcs12.ToPEM(pfxBytes, "")
	if err != nil {
		return nil, errors.Wrap(err, "could not convert pfx to pem")
	}

	return bl, nil
}

func handlePEMBytes(v string) ([]*pem.Block, error) {
	pemData := []byte(v)
	var pemBlocks []*pem.Block
	for {
		b, rest := pem.Decode(pemData)
		if b == nil {
			break
		}
		pemBlocks = append(pemBlocks, b)
		pemData = rest
	}

	if len(pemBlocks) == 0 {
		return nil, errors.New("no pem blocks in input bytes")
	}

	return pemBlocks, nil
}

// from crypto/tls/tls.go
func parsePrivateKey(der []byte) (crypto.PrivateKey, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		switch key := key.(type) {
		case *rsa.PrivateKey, *ecdsa.PrivateKey, ed25519.PrivateKey:
			return key, nil
		default:
			return nil, errors.New("tls: found unknown private key type in PKCS#8 wrapping")
		}
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}

	return nil, errors.New("tls: failed to parse private key")
}
