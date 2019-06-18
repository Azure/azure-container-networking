// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
)

// TelemetryConfig - telemetry config read by telemetry service
type TelemetryConfig struct {
	ReportToHostIntervalInSeconds time.Duration `json:"reportToHostIntervalInSeconds"`
}

// FdName - file descriptor name
// Delimiter - delimiter for socket reads/writes
// azureHostReportURL - host net agent url of type buffer
// DefaultInterval - default interval for sending buffer to host
// logName - telemetry log name
// MaxPayloadSize - max buffer size in bytes
const (
	FdName             = "azure-vnet-telemetry"
	Delimiter          = '\n'
	azureHostReportURL = "http://168.63.129.16/machine/plugins?comp=netagent&type=payload"
	minInterval        = 10 * time.Second
	logName            = "azure-vnet-telemetry"
	MaxPayloadSize     = 4096
	MaxNumReports      = 1000
	dnc                = "DNC"
	cns                = "CNS"
	npm                = "NPM"
	cni                = "CNI"
)

var payloadSize uint16 = 0

// TelemetryBuffer object
type TelemetryBuffer struct {
	client             net.Conn
	listener           net.Listener
	connections        []net.Conn
	azureHostReportURL string
	buffer             Buffer
	FdExists           bool
	Connected          bool
	data               chan interface{}
	cancel             chan bool
	mutex              sync.Mutex
}

// Buffer object holds the different types of reports
type Buffer struct {
	DNCReports []DNCReport
	CNIReports []CNIReport
	NPMReports []NPMReport
	CNSReports []CNSReport
}

// NewTelemetryBuffer - create a new TelemetryBuffer
func NewTelemetryBuffer(hostReportURL string) *TelemetryBuffer {
	var tb TelemetryBuffer

	if hostReportURL == "" {
		tb.azureHostReportURL = azureHostReportURL
	}

	tb.data = make(chan interface{}, MaxNumReports)
	tb.cancel = make(chan bool, 1)
	tb.connections = make([]net.Conn, 0)
	tb.buffer.DNCReports = make([]DNCReport, 0, MaxNumReports)
	tb.buffer.CNIReports = make([]CNIReport, 0, MaxNumReports)
	tb.buffer.NPMReports = make([]NPMReport, 0, MaxNumReports)
	tb.buffer.CNSReports = make([]CNSReport, 0, MaxNumReports)

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
							if _, ok := tmp["NpmVersion"]; ok {
								var npmReport NPMReport
								json.Unmarshal([]byte(reportStr), &npmReport)
								tb.data <- npmReport
							} else if _, ok := tmp["CniSucceeded"]; ok {
								var cniReport CNIReport
								json.Unmarshal([]byte(reportStr), &cniReport)
								tb.data <- cniReport
							} else if _, ok := tmp["Allocations"]; ok {
								var dncReport DNCReport
								json.Unmarshal([]byte(reportStr), &dncReport)
								tb.data <- dncReport
							} else if _, ok := tmp["DncPartitionKey"]; ok {
								var cnsReport CNSReport
								json.Unmarshal([]byte(reportStr), &cnsReport)
								tb.data <- cnsReport
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

// BufferAndPushData - BufferAndPushData running an instance if it isn't already being run elsewhere
func (tb *TelemetryBuffer) BufferAndPushData(intervalms time.Duration) {
	defer tb.Close()
	if !tb.FdExists {
		log.Logf("[Telemetry] Buffer telemetry data and send it to host")
		if intervalms < minInterval {
			intervalms = minInterval
		}

		interval := time.NewTicker(intervalms).C
		for {
			select {
			case <-interval:
				// Send buffer to host and clear cache when sent successfully
				// To-do : if we hit max slice size in buffer, write to disk and process the logs on disk on future sends
				tb.mutex.Lock()
				tb.sendToHost()
				tb.mutex.Unlock()
			case report := <-tb.data:
				tb.mutex.Lock()
				tb.buffer.push(report)
				tb.mutex.Unlock()
			case <-tb.cancel:
				log.Logf("[Telemetry] server cancel event")
				goto EXIT
			}
		}
	} else {
		<-tb.cancel
		log.Logf("[Telemetry] Received cancel event")
	}

EXIT:
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
	b = append(b, Delimiter)
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

// sendToHost - send buffer to host
func (tb *TelemetryBuffer) sendToHost() error {
	buf := Buffer{
		DNCReports: make([]DNCReport, 0),
		CNIReports: make([]CNIReport, 0),
		NPMReports: make([]NPMReport, 0),
		CNSReports: make([]CNSReport, 0),
	}

	seed := rand.NewSource(time.Now().UnixNano())
	i, payloadSize, maxPayloadSizeReached := rand.New(seed).Intn(reflect.ValueOf(&buf).Elem().NumField()), 0, false
	isDNCReportsEmpty, isCNIReportsEmpty, isCNSReportsEmpty, isNPMReportsEmpty := false, false, false, false
	for {
		// craft payload in a round-robin manner.
		switch i % 4 {
		case 0:
			reportLen := len(tb.buffer.DNCReports)
			if reportLen == 0 || isDNCReportsEmpty {
				isDNCReportsEmpty = true
				break
			}

			if reportLen == 1 {
				isDNCReportsEmpty = true
			}

			report := tb.buffer.DNCReports[0]
			if bytes, err := json.Marshal(report); err == nil {
				payloadSize += len(bytes)
				if payloadSize > MaxPayloadSize {
					maxPayloadSizeReached = true
					break
				}
			}
			buf.DNCReports = append(buf.DNCReports, report)
			tb.buffer.DNCReports = tb.buffer.DNCReports[1:]
		case 1:
			reportLen := len(tb.buffer.CNIReports)
			if reportLen == 0 || isCNIReportsEmpty {
				isCNIReportsEmpty = true
				break
			}

			if reportLen == 1 {
				isCNIReportsEmpty = true
			}

			report := tb.buffer.CNIReports[0]
			if bytes, err := json.Marshal(report); err == nil {
				payloadSize += len(bytes)
				if payloadSize > MaxPayloadSize {
					maxPayloadSizeReached = true
					break
				}
			}
			buf.CNIReports = append(buf.CNIReports, report)
			tb.buffer.CNIReports = tb.buffer.CNIReports[1:]
		case 2:
			reportLen := len(tb.buffer.CNSReports)
			if reportLen == 0 || isCNSReportsEmpty {
				isCNSReportsEmpty = true
				break
			}

			if reportLen == 1 {
				isCNSReportsEmpty = true
			}

			report := tb.buffer.CNSReports[0]
			if bytes, err := json.Marshal(report); err == nil {
				payloadSize += len(bytes)
				if payloadSize > MaxPayloadSize {
					maxPayloadSizeReached = true
					break
				}
			}
			buf.CNSReports = append(buf.CNSReports, report)
			tb.buffer.CNSReports = tb.buffer.CNSReports[1:]
		case 3:
			reportLen := len(tb.buffer.NPMReports)
			if reportLen == 0 || isNPMReportsEmpty {
				isNPMReportsEmpty = true
				break
			}

			if reportLen == 1 {
				isNPMReportsEmpty = true
			}

			report := tb.buffer.NPMReports[0]
			if bytes, err := json.Marshal(report); err == nil {
				payloadSize += len(bytes)
				if payloadSize > MaxPayloadSize {
					maxPayloadSizeReached = true
					break
				}
			}
			buf.NPMReports = append(buf.NPMReports, report)
			tb.buffer.NPMReports = tb.buffer.NPMReports[1:]
		}

		if isDNCReportsEmpty && isCNIReportsEmpty && isCNSReportsEmpty && isNPMReportsEmpty {
			break
		}

		if maxPayloadSizeReached {
			break
		}

		i++
	}

	httpc := &http.Client{}
	var body bytes.Buffer
	log.Logf("Sending buffer %+v", buf)
	if err := json.NewEncoder(&body).Encode(buf); err != nil {
		log.Logf("[Telemetry] Encode buffer error %v", err)
	}
	resp, err := httpc.Post(tb.azureHostReportURL, ContentType, &body)
	log.Logf("[Telemetry] Got response %v", resp)
	if err != nil {
		return fmt.Errorf("[Telemetry] HTTP Post returned error %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("[Telemetry] HTTP Post returned statuscode %d", resp.StatusCode)
	}

	return nil
}

// push - push the report (x) to corresponding slice
func (buf *Buffer) push(x interface{}) {
	metadata, err := getHostMetadata()
	if err != nil {
		log.Logf("Error getting metadata %v", err)
	} else {
		err = saveHostMetadata(metadata)
		if err != nil {
			log.Logf("saving host metadata failed with :%v", err)
		}
	}

	switch x.(type) {
	case DNCReport:
		if len(buf.DNCReports) >= MaxNumReports {
			return
		}
		dncReport := x.(DNCReport)
		dncReport.Metadata = metadata
		buf.DNCReports = append(buf.DNCReports, dncReport)
	case CNIReport:
		if len(buf.CNIReports) >= MaxNumReports {
			return
		}
		cniReport := x.(CNIReport)
		cniReport.Metadata = metadata
		buf.CNIReports = append(buf.CNIReports, cniReport)
	case NPMReport:
		if len(buf.NPMReports) >= MaxNumReports {
			return
		}
		npmReport := x.(NPMReport)
		npmReport.Metadata = metadata
		buf.NPMReports = append(buf.NPMReports, npmReport)
	case CNSReport:
		if len(buf.CNSReports) >= MaxNumReports {
			return
		}
		cnsReport := x.(CNSReport)
		cnsReport.Metadata = metadata
		buf.CNSReports = append(buf.CNSReports, cnsReport)
	}
}

// reset - reset buffer slices and sets payloadSize to 0
func (buf *Buffer) reset() {
	buf.DNCReports = nil
	buf.DNCReports = make([]DNCReport, 0)
	buf.CNIReports = nil
	buf.CNIReports = make([]CNIReport, 0)
	buf.NPMReports = nil
	buf.NPMReports = make([]NPMReport, 0)
	buf.CNSReports = nil
	buf.CNSReports = make([]CNSReport, 0)
	payloadSize = 0
}

// saveHostMetadata - save metadata got from wireserver to json file
func saveHostMetadata(metadata Metadata) error {
	dataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("[Telemetry] marshal data failed with err %+v", err)
	}

	if err = ioutil.WriteFile(metadataFile, dataBytes, 0644); err != nil {
		log.Logf("[Telemetry] Writing metadata to file failed: %v", err)
	}

	return err
}

// getHostMetadata - retrieve metadata from host
func getHostMetadata() (Metadata, error) {
	content, err := ioutil.ReadFile(metadataFile)
	if err == nil {
		var metadata Metadata
		if err = json.Unmarshal(content, &metadata); err == nil {
			return metadata, nil
		}
	}

	log.Logf("[Telemetry] Request metadata from wireserver")

	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return Metadata{}, err
	}

	req.Header.Set("Metadata", "True")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Metadata{}, err
	}

	defer resp.Body.Close()

	metareport := metadataWrapper{}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("[Telemetry] Request failed with HTTP error %d", resp.StatusCode)
	} else if resp.Body != nil {
		err = json.NewDecoder(resp.Body).Decode(&metareport)
		if err != nil {
			err = fmt.Errorf("[Telemetry] Unable to decode response body due to error: %s", err.Error())
		}
	} else {
		err = fmt.Errorf("[Telemetry] Response body is empty")
	}

	return metareport.Metadata, err
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
	if exists, _ := common.CheckIfFileExists(path); !exists {
		ex, _ := os.Executable()
		exDir := filepath.Dir(ex)
		path = fmt.Sprintf("%v/%v", exDir, TelemetryServiceProcessName)
		if exists, _ = common.CheckIfFileExists(path); !exists {
			log.Logf("Skip starting telemetry service as file didn't exist")
			return
		}
		dir = exDir
	} else {
		dir = CniInstallDir
	}

	return
}
