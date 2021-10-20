package controllers

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
)

func TestMain(m *testing.M) {
	metrics.InitializeAll()

	exitCode := m.Run()
	os.Exit(exitCode)
}
