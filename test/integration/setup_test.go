//go:build integration

package k8s

import (
	"context"
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
)

const (
	exitFail = 1

	// relative log directory
	logDir = "logs/"
)

func TestMain(m *testing.M) {
	var (
		err        error
		exitCode   int
		cnicleanup func() error
		cnscleanup func() error
	)

	defer func() {
		if r := recover(); r != nil {
			log.Println(string(debug.Stack()))
			exitCode = exitFail
		}

		if err != nil {
			log.Print(err)
			exitCode = exitFail
		} else {
			if cnicleanup != nil {
				cnicleanup()
			}
			if cnscleanup != nil {
				cnscleanup()
			}
		}

		os.Exit(exitCode)
	}()

	clientset := kubernetes.MustGetClientset()

	ctx := context.Background()
	installopt := os.Getenv(kubernetes.EnvInstallCNS)
	// create dirty cns ds
	if installCNS, err := strconv.ParseBool(installopt); err == nil && installCNS {
		if cnscleanup, err = kubernetes.InstallCNSDaemonset(ctx, clientset, logDir); err != nil {
			log.Print(err)
			exitCode = 2
			return
		}
	} else {
		log.Printf("Env %v not set to true, skipping", kubernetes.EnvInstallCNS)
	}

	exitCode = m.Run()
}
