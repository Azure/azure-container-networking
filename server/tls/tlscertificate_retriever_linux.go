// Copyright 2020 Microsoft. All rights reserved.

package tls

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
)

const (
	CertLabel       = "CERTIFICATE"
	PrivateKeyLabel = "PRIVATE KEY"
)

type linuxTlsCertificateRetriever struct {
	pemBlock []*pem.Block
	settings TlsSettings
}

// GetCertificate Returns the certificate associated with the pem
func (fcert *linuxTlsCertificateRetriever) GetCertificate() (*x509.Certificate, error) {
	for _, block := range fcert.pemBlock {
		if block.Type == CertLabel {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("Failed to parse certificate at location %s with error %+v", fcert.settings.TLSCertificatePath, err)
			}
			if !cert.IsCA {
				return cert, nil
			}
		}
	}
	return nil, fmt.Errorf("No Certificate block found")
}

// GetPrivateKey Returns the private key associated with the pem
func (fcert *linuxTlsCertificateRetriever) GetPrivateKey() (crypto.PrivateKey, error) {
	for _, block := range fcert.pemBlock {
		if block.Type == PrivateKeyLabel {
			pk, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("Could not parse private key %+v", err)
			}
			return pk, nil
		}
	}
	return nil, fmt.Errorf("No private key found in certificate bundle located at %s", fcert.settings.TLSCertificatePath)
}

// ReadFile reads a from disk
func (fcert *linuxTlsCertificateRetriever) readFile() ([]byte, error) {
	content, err := ioutil.ReadFile(fcert.settings.TLSCertificatePath)
	if err != nil {
		return nil, fmt.Errorf("Error reading file from path %s with error: %+v ", fcert.settings.TLSCertificatePath, err)
	}
	return content, nil
}

// Parses a file to PEM format
func (fcert *linuxTlsCertificateRetriever) parsePEMFile(content []byte) error {
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

// NewTlsCertificateRetriever creates a TlsCertificateRetriever
// NewTlsCertificateRetriever depends on the pem being available
// linux users generally store certificates at /etc/ssl/certs/
func NewTlsCertificateRetriever(settings TlsSettings) (TlsCertificateRetriever, error) {
	linuxCertStoreRetriever := &linuxTlsCertificateRetriever{
		settings: settings,
	}
	content, err := linuxCertStoreRetriever.readFile()

	if err != nil {
		return nil, fmt.Errorf("Failed to read file with error %+v", err)
	}

	if err := linuxCertStoreRetriever.parsePEMFile(content); err != nil {
		return nil, fmt.Errorf("Failed to parse PEM file with error %+v", err)
	}

	return linuxCertStoreRetriever, nil
}
