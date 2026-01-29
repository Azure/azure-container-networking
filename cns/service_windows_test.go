package cns

import (
	"github.com/billgraziano/dpapi"
)

// postProcessPEMCert encrypts the pem to base64 with dpapi on windows
func postProcessPEMCert(pem []byte) ([]byte, error) {
	var fileContent []byte
	// On Windows, the TLS certificate retriever expects DPAPI-encrypted content
	encrypted, encryptErr := dpapi.Encrypt(string(pem))
	if encryptErr != nil {
		return nil, encryptErr
	}
	fileContent = []byte(encrypted)

	return fileContent, nil
}
