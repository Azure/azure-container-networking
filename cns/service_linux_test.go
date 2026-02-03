package cns

// postProcessPEMCert does not do anything on linux
func postProcessPEMCert(pem []byte) ([]byte, error) {
	return pem, nil
}
