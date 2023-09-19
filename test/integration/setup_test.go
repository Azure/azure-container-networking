//go:build integration

package k8s

import (
	"context"
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"testing"

	k8sutils "github.com/Azure/azure-container-networking/test/internal/k8sutils"
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

	clientset, err := k8sutils.MustGetClientset()
	if err != nil {
		return
	}

	ctx := context.Background()
	if installopt := os.Getenv(envInstallCNS); installopt != "" {
		// create dirty cns ds
		if installCNS, err := strconv.ParseBool(installopt); err == nil && installCNS == true {
			if cnscleanup, err = InstallCNSDaemonset(ctx, clientset, logDir); err != nil {
				log.Print(err)
				exitCode = 2
				return
			}
		}
	} else {
		log.Printf("Env %v not set to true, skipping", envInstallCNS)
	}

	exitCode = m.Run()
}
