// Copyright 2020 Microsoft. All rights reserved.

package tls

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/billgraziano/dpapi"
	"io/ioutil"
	"strings"
)

type windowsTlsCertificateRetriever struct {
	pemBlock []*pem.Block
	settings TlsSettings
}

const (
	CertLabel       = "CERTIFICATE"
	PrivateKeyLabel = "PRIVATE KEY"
)

// GetCertificate Returns the certificate associated with the pem
func (wtls *windowsTlsCertificateRetriever) GetCertificate() (*x509.Certificate, error) {
	for _, block := range wtls.pemBlock {
		if block.Type == CertLabel {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("Failed to parse certificate at location %s with error %+v", wtls.settings.TLSCertificatePath, err)
			}
			if !cert.IsCA {
				return cert, nil
			}
		}
	}
	return nil, fmt.Errorf("No Certificate block found")
}

// GetPrivateKey Returns the private key associated with the pem
func (wtls *windowsTlsCertificateRetriever) GetPrivateKey() (crypto.PrivateKey, error) {
	for _, block := range wtls.pemBlock {
		if block.Type == PrivateKeyLabel {
			pk, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("Could not parse private key %+v", err)
			}
			return pk, nil
		}
	}
	return nil, fmt.Errorf("No private key found in certificate bundle located at %s", wtls.settings.TLSCertificatePath)
}

// ReadFile reads a from disk
func (wtls *windowsTlsCertificateRetriever) readFile() ([]byte, error) {
	content, err := ioutil.ReadFile(wtls.settings.TLSCertificatePath)
	if err != nil {
		return nil, fmt.Errorf("Error reading file from path %s with error: %+v ", wtls.settings.TLSCertificatePath, err)
	}
	return content, nil
}

// ParsePEMFile Parses a file to PEM format
func (fcert *windowsTlsCertificateRetriever) parsePEMFile(content []byte) error {
	pemBlocks := make([]*pem.Block, 0)

	var pemBlock *pem.Block
	nextPemBlock := content

	for {
		pemBlock, nextPemBlock = pem.Decode(nextPemBlock)

		if pemBlock == nil {
			break
		}
		pemBlocks = append(pemBlocks, pemBlock)
	}

	if len(pemBlocks) < 2 {
		return fmt.Errorf("Invalid PEM format located at %s", fcert.settings.TLSCertificatePath)
	}

	fcert.pemBlock = pemBlocks
	return nil
}

// Decrypt is a no-op for linux implementation
func (wtls *windowsTlsCertificateRetriever) decrypt(content []byte) (string, error) {
	decrypted, err := dpapi.Decrypt(string(content))
	if err != nil {
		return "", fmt.Errorf("Error decrypting file from path %s with error: %+v ", wtls.settings.TLSCertificatePath, err)
	}

	decrypted = formatDecryptedPemString(decrypted)
	return decrypted, nil
}

// formatDecryptedPemString ensures pem format
// removes spaces that should be line breaks
// ensures headers are properly formatted
// removes null terminated strings that dpapi.decrypt introduces
func formatDecryptedPemString(s string) string {
	s = strings.ReplaceAll(s, " ", "\r\n")
	s = strings.ReplaceAll(s, "\000", "")
	s = strings.ReplaceAll(s, "-----BEGIN\r\nPRIVATE\r\nKEY-----", "-----BEGIN PRIVATE KEY-----")
	s = strings.ReplaceAll(s, "-----END\r\nPRIVATE\r\nKEY-----", "-----END PRIVATE KEY-----")
	s = strings.ReplaceAll(s, "-----BEGIN\r\nCERTIFICATE-----", "-----BEGIN CERTIFICATE-----")
	s = strings.ReplaceAll(s, "-----END\r\nCERTIFICATE-----", "-----END CERTIFICATE-----")
	return s
}

// NewWindowsTlsCertificateRetriever creates a TlsCertificateRetriever
// NewFileTlsCertificateRetriever depends on the pem being available on the windows file
// and encrypted with the DPAPI libraries
func NewTlsCertificateRetriever(settings TlsSettings) (TlsCertificateRetriever, error) {
	windowsCertStoreRetriever := &windowsTlsCertificateRetriever{
		settings: settings,
	}

	content, err := windowsCertStoreRetriever.readFile()

	if err != nil {
		return nil, fmt.Errorf("Failed to read file with error %+v", err)
	}

	decrypted, err := windowsCertStoreRetriever.decrypt(content)

	if err != nil {
		return nil, fmt.Errorf("Failed to decrypt file with error %+v", err)
	}

	if err := windowsCertStoreRetriever.parsePEMFile([]byte(decrypted)); err != nil {
		return nil, fmt.Errorf("Failed to parse PEM file with error %+v", err)
	}

	return windowsCertStoreRetriever, nil
}
