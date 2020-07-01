package kubecontroller

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

var onlyOneSignalHandler = make(chan struct{})
var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// SetupSignalHandler registers for SIGTERM and SIGINT. A stop channel is returned
// which is closed on one of these signals. If a second signal is caught, the program
// is terminated with exit code 1.
// exitChan is notified when a SIGINT or SIGTERM signal is received.
func SetupSignalHandler(exitChan chan<- bool) (stopCh <-chan struct{}) {
	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		// Notify the provided exitChan
		// this will allow whoever provided the channel time to cleanup before requestController exits
		exitChan <- true
		fmt.Println("Sending to exitChan")
		close(stop)
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return stop
}
