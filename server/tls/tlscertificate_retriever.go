// Copyright 2020 Microsoft. All rights reserved.

package tls

import "time"

// TlsSettings - Details related to the TLS certificate.
type TlsSettings struct {
	TLSSubjectName                     string
	TLSCertificatePath                 string
	TLSPort                            string
	KeyVaultURL                        string
	KeyVaultCertificateName            string
	MSIResourceID                      string
	KeyVaultCertificateRefreshInterval time.Duration
}

func GetTlsCertificateRetriever(settings TlsSettings) (TlsCertificateRetriever, error) {
	// if Windows build flag is set, the below will return a windows implementation
	// if Linux build flag is set, the below will return a Linux implementation
	// tls certificate parsed from disk.
	// note if file ends with OS type, ie ends with Linux or Windows
	// go treats that as a build tag : https://golang.org/cmd/go/#hdr-Build_constraints
	return NewTlsCertificateRetriever(settings)
}
