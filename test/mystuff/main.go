package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"golang.org/x/sys/unix"
)

func main() {
	file, err := getIPTablesLockFile(time.Second*10, time.Millisecond*100)
	if err != nil {
		panic(err)
	}
	grabLock(file)
	fmt.Println("grabbed the lock")
	t := 30
	for i := 0; i < t; i++ {
		fmt.Println("sleeping for", i, "seconds")
		time.Sleep(time.Second)
	}
	releaseLockFile(file)
	fmt.Println("released the lock")
}

const iptablesLockFile = "/run/xtables.lock"

var ErrLocktimeout = fmt.Errorf("timed out while trying to grab iptables lock")

func getIPTablesLockFile(timeout, probeInterval time.Duration) (*os.File, error) {
	// file, err := os.Create(iptablesLockFile)
	file, err := os.OpenFile(iptablesLockFile, os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open iptables lock file: %w", err)
	}

	startTime := time.Now()
	for {
		if err := grabLock(file); err == nil {
			break
		}
		if time.Since(startTime) > timeout {
			file.Close()
			return nil, ErrLocktimeout
		}
		time.Sleep(probeInterval)
	}
	return file, nil
}

func grabLock(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) // or just unix.LOCK_EX??
}

func releaseLockFile(file *os.File) {
	err := file.Close()
	// err := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	if err != nil {
		log.Errorf("failed to unlock file: %w", err)
		panic(fmt.Sprintf("failed to unlock file: %s", err.Error()))
	}
}

// import (
// 	"net/http"
// 	"time"

// 	"github.com/Azure/azure-container-networking/npm/http/api"
// 	"github.com/Azure/azure-container-networking/npm/metrics"
// 	"github.com/gorilla/mux"
// )

// func main() {
// 	metrics.InitializeAll()
// 	router := mux.NewRouter()
// 	router.Handle(api.NodeMetricsPath, metrics.GetHandler(true))
// 	router.Handle(api.ClusterMetricsPath, metrics.GetHandler(false))
// 	// router.Handle(api.DataplaneHealthMetricsPath, metrics.GetHandler(metrics.DataplaneHealthMetrics))
// 	srv := &http.Server{
// 		Handler: router,
// 		Addr:    "0.0.0.0:8080",
// 	}

// 	timer := metrics.StartNewTimer()
// 	time.Sleep(time.Millisecond * 50)
// 	metrics.RecordControllerPodExecTime(timer, metrics.CreateOp)

// 	timer = metrics.StartNewTimer()
// 	time.Sleep(time.Millisecond * 200)
// 	metrics.RecordControllerPodExecTime(timer, metrics.CreateOp)

// 	timer = metrics.StartNewTimer()
// 	time.Sleep(time.Millisecond * 400)
// 	metrics.RecordControllerPodExecTime(timer, metrics.UpdateOp)

// 	srv.ListenAndServe()
// 	time.Sleep(time.Minute * 10)
// }
