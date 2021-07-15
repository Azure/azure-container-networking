// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
)

// TelemetryConfig - telemetry config read by telemetry service
type TelemetryConfig struct {
	ReportToHostIntervalInSeconds time.Duration `json:"reportToHostIntervalInSeconds"`
	DisableAll                    bool
	DisableTrace                  bool
	DisableMetric                 bool
	DisableMetadataThread         bool
	DebugMode                     bool
	DisableTelemetryToNetAgent    bool
	RefreshTimeoutInSecs          int
	BatchIntervalInSecs           int
	BatchSizeInBytes              int
	GetEnvRetryCount              int
	GetEnvRetryWaitTimeInSecs     int
}

// FdName - file descriptor name
// Delimiter - delimiter for socket reads/writes
// MaxPayloadSize - max buffer size in bytes
const (
	FdName         = "azure-vnet-telemetry"
	Delimiter      = '\n'
	MaxPayloadSize = 4096
	MaxNumReports  = 1000
)

// TelemetryBuffer object
type TelemetryBuffer struct {
	client             net.Conn
	listener           net.Listener
	connections        []net.Conn
	azureHostReportURL string
	FdExists           bool
	Connected          bool
	data               chan interface{}
	cancel             chan bool
	mutex              sync.Mutex
}

// Buffer object holds the different types of reports
type Buffer struct {
	CNIReports []CNIReport
}

// NewTelemetryBuffer - create a new TelemetryBuffer
func NewTelemetryBuffer() *TelemetryBuffer {
	var tb TelemetryBuffer

	tb.data = make(chan interface{}, MaxNumReports)
	tb.cancel = make(chan bool, 1)
	tb.connections = make([]net.Conn, 0)

	return &tb
}

func remove(s []net.Conn, i int) []net.Conn {
	if len(s) > 0 && i < len(s) {
		s[i] = s[len(s)-1]
		return s[:len(s)-1]
	}

	log.Logf("tb connections remove failed index %v len %v", i, len(s))
	return s
}

// Starts Telemetry server listening on unix domain socket
func (tb *TelemetryBuffer) StartServer() error {
	err := tb.Listen(FdName)
	if err != nil {
		tb.FdExists = strings.Contains(err.Error(), "in use") || strings.Contains(err.Error(), "Access is denied")
		log.Logf("Listen returns: %v", err.Error())
		return err
	}

	log.Logf("Telemetry service started")
	// Spawn server goroutine to handle incoming connections
	go func() {
		for {
			// Spawn worker goroutines to communicate with client
			conn, err := tb.listener.Accept()
			if err == nil {
				tb.mutex.Lock()
				tb.connections = append(tb.connections, conn)
				tb.mutex.Unlock()
				go func() {
					for {
						reportStr, err := read(conn)
						if err == nil {
							var tmp map[string]interface{}
							json.Unmarshal(reportStr, &tmp)
							if _, ok := tmp["CniSucceeded"]; ok {
								var cniReport CNIReport
								json.Unmarshal([]byte(reportStr), &cniReport)
								tb.data <- cniReport
							} else if _, ok := tmp["Metric"]; ok {
								var aiMetric AIMetric
								json.Unmarshal([]byte(reportStr), &aiMetric)
								tb.data <- aiMetric
							}
						} else {
							var index int
							var value net.Conn
							var found bool

							tb.mutex.Lock()
							defer tb.mutex.Unlock()

							for index, value = range tb.connections {
								if value == conn {
									conn.Close()
									found = true
									break
								}
							}

							if found {
								tb.connections = remove(tb.connections, index)
							}

							return
						}
					}
				}()
			} else {
				log.Logf("Telemetry Server accept error %v", err)
				return
			}
		}
	}()

	return nil
}

func (tb *TelemetryBuffer) Connect() error {
	err := tb.Dial(FdName)
	if err == nil {
		tb.Connected = true
	} else if tb.FdExists {
		tb.Cleanup(FdName)
	}

	return err
}

// PushData - PushData running an instance if it isn't already being run elsewhere
func (tb *TelemetryBuffer) PushData() {
	for {
		select {
		case report := <-tb.data:
			tb.mutex.Lock()
			push(report)
			tb.mutex.Unlock()
		case <-tb.cancel:
			log.Logf("[Telemetry] server cancel event")
			goto EXIT
		}
	}

EXIT:
	tb.Close()
}

// read - read from the file descriptor
func read(conn net.Conn) (b []byte, err error) {
	b, err = bufio.NewReader(conn).ReadBytes(Delimiter)
	if err == nil {
		b = b[:len(b)-1]
	}

	return
}

// Write - write to the file descriptor
func (tb *TelemetryBuffer) Write(b []byte) (c int, err error) {
	buf := make([]byte, len(b))
	copy(b, buf)
	b = append(buf, Delimiter)
	w := bufio.NewWriter(tb.client)
	c, err = w.Write(b)
	if err == nil {
		err = w.Flush()
	}

	return
}

// Cancel - signal to tear down telemetry buffer
func (tb *TelemetryBuffer) Cancel() {
	tb.cancel <- true
}

// Close - close all connections
func (tb *TelemetryBuffer) Close() {
	if tb.client != nil {
		tb.client.Close()
		tb.client = nil
	}

	if tb.listener != nil {
		log.Logf("server close")
		tb.listener.Close()
	}

	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	for _, conn := range tb.connections {
		if conn != nil {
			conn.Close()
		}
	}

	tb.connections = nil
	tb.connections = make([]net.Conn, 0)
}

// push - push the report (x) to corresponding slice
func push(x interface{}) {
	metadata, err := common.GetHostMetadata(metadataFile)
	if err != nil {
		log.Logf("Error getting metadata %v", err)
	} else {
		kvs, err := store.NewJsonFileStore(metadataFile)
		if err != nil {
			log.Printf("Error acuiring lock for writing metadata file: %v", err)
		}

		kvs.Lock(true)
		err = common.SaveHostMetadata(metadata, metadataFile)
		if err != nil {
			log.Logf("saving host metadata failed with :%v", err)
		}
		kvs.Unlock(true)
	}

	switch y := x.(type) {
	case CNIReport:
		y.Metadata = metadata
		SendAITelemetry(y)

	case AIMetric:
		SendAIMetric(y)
	}
}

// WaitForTelemetrySocket - Block still pipe/sock created or until max attempts retried
func WaitForTelemetrySocket(maxAttempt int, waitTimeInMillisecs time.Duration) {
	for attempt := 0; attempt < maxAttempt; attempt++ {
		if SockExists() {
			break
		}

		time.Sleep(waitTimeInMillisecs * time.Millisecond)
	}
}

// StartTelemetryService - Kills if any telemetry service runs and start new telemetry service
func StartTelemetryService(path string, args []string) error {
	platform.KillProcessByName(TelemetryServiceProcessName)

	log.Logf("[Telemetry] Starting telemetry service process :%v args:%v", path, args)

	if err := common.StartProcess(path, args); err != nil {
		log.Logf("[Telemetry] Failed to start telemetry service process :%v", err)
		return err
	}

	log.Logf("[Telemetry] Telemetry service started")

	return nil
}

// ReadConfigFile - Read telemetry config file and populate to structure
func ReadConfigFile(filePath string) (TelemetryConfig, error) {
	config := TelemetryConfig{}

	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Logf("[Telemetry] Failed to read telemetry config: %v", err)
		return config, err
	}

	if err = json.Unmarshal(b, &config); err != nil {
		log.Logf("[Telemetry] unmarshal failed with %v", err)
	}

	return config, err
}

// ConnectToTelemetryService - Attempt to spawn telemetry process if it's not already running.
func (tb *TelemetryBuffer) ConnectToTelemetryService(telemetryNumRetries, telemetryWaitTimeInMilliseconds int) {
	path, dir := getTelemetryServiceDirectory()
	args := []string{"-d", dir}
	for attempt := 0; attempt < 2; attempt++ {
		if err := tb.Connect(); err != nil {
			log.Logf("Connection to telemetry socket failed: %v", err)
			tb.Cleanup(FdName)
			StartTelemetryService(path, args)
			WaitForTelemetrySocket(telemetryNumRetries, time.Duration(telemetryWaitTimeInMilliseconds))
		} else {
			tb.Connected = true
			log.Logf("Connected to telemetry service")
			return
		}
	}
}

// TryToConnectToTelemetryService - Attempt to connect telemetry process without spawning it if it's not already running.
func (tb *TelemetryBuffer) TryToConnectToTelemetryService() {
	if err := tb.Connect(); err != nil {
		log.Logf("Connection to telemetry socket failed: %v", err)
		return
	}

	tb.Connected = true
	log.Logf("Connected to telemetry service")
}

func getTelemetryServiceDirectory() (path string, dir string) {
	path = fmt.Sprintf("%v/%v", CniInstallDir, TelemetryServiceProcessName)
	if exists, _ := platform.CheckIfFileExists(path); !exists {
		ex, _ := os.Executable()
		exDir := filepath.Dir(ex)
		path = fmt.Sprintf("%v/%v", exDir, TelemetryServiceProcessName)
		if exists, _ = platform.CheckIfFileExists(path); !exists {
			log.Logf("Skip starting telemetry service as file didn't exist")
			return
		}
		dir = exDir
	} else {
		dir = CniInstallDir
	}

	return
}
