package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/keyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const serverAddr = "127.0.0.1:9005"

var logger *zap.Logger

func mustArgs() (kvURL string, kvCert string) {
	flag.StringVar(&kvURL, "keyvault-url", "", "keyvault url")
	flag.StringVar(&kvCert, "keyvault-cert-name", "", "keyvault certificate name")
	flag.Parse()
	if kvURL == "" || kvCert == "" {
		flag.Usage()
		os.Exit(1)
	}
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()), os.Stdout, zap.DebugLevel)
	logger = zap.New(core)
	return
}

// you must be logged in via the az cli and have proper permissions to a keyvault to run this example
func main() {
	kvURL, kvCert := mustArgs()
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		logger.Fatal("could not create credentials", zap.Error(err))
	}

	kvs, err := keyvault.NewShim(kvURL, cred)
	if err != nil {
		logger.Fatal("could not create keyvault client", zap.Error(err))
	}

	tlsCert, err := kvs.GetLatestTLSCertificate(context.TODO(), kvCert)
	if err != nil {
		logger.Fatal("could not get tls cert from keyvault", zap.Error(err))
	}

	clientTLSConfig, err := createClientTLSConfig(tlsCert)
	if err != nil {
		logger.Fatal("could not create client tls config", zap.Error(err))
	}

	server := http.Server{
		Addr: serverAddr,
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte("hello"))
		}),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			ClientCAs:    clientTLSConfig.RootCAs,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		},
	}

	go func() {
		if err := server.ListenAndServeTLS("", ""); err != nil {
			logger.Fatal("could not serve tls", zap.Error(err))
		}
	}()

	// wait for a short time to allow server to start
	time.Sleep(time.Second)

	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
	}

	addr := fmt.Sprintf("https://%s", serverAddr)
	resp, err := client.Get(addr)
	if err != nil {
		logger.Fatal("could not get response", zap.String("host", addr), zap.Error(err))
	}

	printTLSConnState(resp.TLS)

	bs, _ := io.ReadAll(resp.Body)
	logger.Info("response from tls server", zap.String("body bytes", string(bs)))
}

func createClientTLSConfig(tlsCert tls.Certificate) (*tls.Config, error) {
	certs := x509.NewCertPool()

	if len(tlsCert.Certificate) == 1 { // self signed
		cer, err := x509.ParseCertificate(tlsCert.Certificate[0])
		if err != nil {
			return nil, err
		}
		certs.AddCert(cer)
		return &tls.Config{RootCAs: certs, ServerName: tlsCert.Leaf.Subject.CommonName}, nil
	}

	for i, bytes := range tlsCert.Certificate {
		if i == 0 {
			continue // skip leaf
		}
		cer, err := x509.ParseCertificate(bytes)
		if err != nil {
			return nil, err
		}
		certs.AddCert(cer)
	}

	return &tls.Config{Certificates: []tls.Certificate{tlsCert}, RootCAs: certs, ServerName: tlsCert.Leaf.Subject.CommonName}, nil
}

func printTLSConnState(connState *tls.ConnectionState) {
	logger.Info("response tls connection state", zap.Object("conn state", loggableConnState(*connState)))

	for i, cert := range connState.PeerCertificates {
		logger.Info(fmt.Sprintf("peer certificate %d:", i), zap.Stringer("subject", cert.Subject), zap.Stringer("issuer", cert.Issuer))
	}

	for i, chain := range connState.VerifiedChains {
		for j, cert := range chain {
			logger.Info(fmt.Sprintf("chain %d, cert %d:", i, j), zap.Stringer("subject", cert.Subject), zap.Stringer("issuer", cert.Issuer))
		}
	}
}

type loggableConnState tls.ConnectionState

func (l loggableConnState) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("server name", l.ServerName)
	encoder.AddBool("handshake complete", l.HandshakeComplete)
	encoder.AddInt("peer certificates", len(l.PeerCertificates))
	encoder.AddInt("verified certificates", len(l.VerifiedChains))
	return nil
}
