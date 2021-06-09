// Copyright 2020 Microsoft. All rights reserved.

package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"testing"
)

const (
	rsaBits    = 2048
	commonName = "test.azure.com"
)

func TestPemConsumptionLinux(t *testing.T) {
	pemContent := createPemCertificate(t)
	currentDirectory, _ := os.Getwd()
	pemLocation := fmt.Sprintf("%s/%s.Pem", currentDirectory, commonName)

	ioutil.WriteFile(pemLocation, pemContent, 0644)
	defer os.Remove(pemLocation)

	config := TlsSettings{
		TLSCertificatePath: pemLocation,
		TLSSubjectName:     commonName,
	}

	fileCertRetriever, err := NewTlsCertificateRetriever(config)
	if err != nil {
		t.Fatalf("Failed to open file certificate retriever %+v", err)
	}
	certificate, err := fileCertRetriever.GetCertificate()
	if err != nil {
		t.Fatalf("Failed to get certificate %+v", err)
	}
	if certificate.Subject.CommonName != commonName {
		t.Fatalf("Recieved a unexpected subject name %+v", err)
	}
	_, err = fileCertRetriever.GetPrivateKey()
	if err != nil {
		t.Fatalf("Failed to get private key %+v", err)
	}
}

func createPemCertificate(t *testing.T) []byte {
	priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
			CommonName:   commonName,
		},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("Could not marshal private key %+v", err)
	}

	if err != nil {
		t.Fatalf("Could not encode certificate to Pem %+v", err)
	}

	pemCert := pem.EncodeToMemory(&pem.Block{Type: CertLabel, Bytes: derBytes})
	pemKey := pem.EncodeToMemory(&pem.Block{Type: PrivateKeyLabel, Bytes: privateKeyBytes})

	pemBundle := fmt.Sprintf("%s%s", pemCert, pemKey)

	return []byte(pemBundle)
}
