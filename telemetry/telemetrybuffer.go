// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
)

// FdName - file descriptor name
// Delimiter - delimiter for socket reads/writes
// azureHostReportURL - host net agent url of type payload
// DefaultInterval - default interval for sending payload to host
// logName - telemetry log name
// MaxPayloadSize - max payload size in bytes
const (
	FdName                    = "azure-vnet-telemetry"
	Delimiter                 = '\n'
	azureHostReportURL        = "http://168.63.129.16/machine/plugins?comp=netagent&type=payload"
	DefaultInterval           = 10 * time.Second
	logName                   = "azure-vnet-telemetry"
	MaxPayloadSize     uint16 = 65535
	dnc                       = "DNC"
	cns                       = "CNS"
	npm                       = "NPM"
	cni                       = "CNI"
)

var telemetryLogger = log.NewLogger(logName, log.LevelInfo, log.TargetStderr)
var payloadSize uint16 = 0

// TelemetryBuffer object
type TelemetryBuffer struct {
	client             net.Conn
	listener           net.Listener
	connections        []net.Conn
	azureHostReportURL string
	payload            Payload
	FdExists           bool
	Connected          bool
	data               chan interface{}
	cancel             chan bool
}

// Payload object holds the different types of reports
type Payload struct {
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

	tb.data = make(chan interface{})
	tb.cancel = make(chan bool, 1)
	tb.connections = make([]net.Conn, 0)
	tb.payload.DNCReports = make([]DNCReport, 0)
	tb.payload.CNIReports = make([]CNIReport, 0)
	tb.payload.NPMReports = make([]NPMReport, 0)
	tb.payload.CNSReports = make([]CNSReport, 0)

	err := telemetryLogger.SetTarget(log.TargetLogfile)
	if err != nil {
		fmt.Printf("Failed to configure logging: %v\n", err)
	}

	return &tb
}

func remove(s []net.Conn, i int) []net.Conn {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

// Starts Telemetry server listening on unix domain socket
func (tb *TelemetryBuffer) StartServer() error {
	err := tb.Listen(FdName)
	if err != nil {
		tb.FdExists = strings.Contains(err.Error(), "in use") || strings.Contains(err.Error(), "Access is denied")
		telemetryLogger.Printf("Listen returns: %v", err.Error())
		return err
	}

	telemetryLogger.Printf("Telemetry service started")
	// Spawn server goroutine to handle incoming connections
	go func() {
		for {
			// Spawn worker goroutines to communicate with client
			conn, err := tb.listener.Accept()
			if err == nil {
				tb.connections = append(tb.connections, conn)
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
							telemetryLogger.Printf("Server closing client connection")
							for index, value := range tb.connections {
								if value == conn {
									conn.Close()
									tb.connections = remove(tb.connections, index)
									return
								}
							}
						}
					}
				}()
			} else {
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
		telemetryLogger.Printf("[Telemetry] Buffer telemetry data and send it to host")
		if intervalms < DefaultInterval {
			intervalms = DefaultInterval
		}

		interval := time.NewTicker(intervalms).C
		for {
			select {
			case <-interval:
				// Send payload to host and clear cache when sent successfully
				// To-do : if we hit max slice size in payload, write to disk and process the logs on disk on future sends
				if err := tb.sendToHost(); err == nil {
					tb.payload.reset()
				} else {
					telemetryLogger.Printf("[Telemetry] sending to host failed with error %+v", err)
				}
			case report := <-tb.data:
				tb.payload.push(report)
			case <-tb.cancel:
				telemetryLogger.Printf("server cancel event")
				goto EXIT
			}
		}
	} else {
		<-tb.cancel
		telemetryLogger.Printf("Received cancel event")
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
		telemetryLogger.Printf("client close")
		tb.client.Close()
		tb.client = nil
	}

	if tb.listener != nil {
		telemetryLogger.Printf("server close")
		tb.listener.Close()
		tb.listener = nil
	}

	for _, conn := range tb.connections {
		if conn != nil {
			telemetryLogger.Printf("connection close")
			conn.Close()
		}
	}

	tb.connections = nil
	tb.connections = make([]net.Conn, 0)
}

// sendToHost - send payload to host
func (tb *TelemetryBuffer) sendToHost() error {
	httpc := &http.Client{}
	var body bytes.Buffer
	telemetryLogger.Printf("Sending payload %+v", tb.payload)
	json.NewEncoder(&body).Encode(tb.payload)
	resp, err := httpc.Post(tb.azureHostReportURL, ContentType, &body)
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
func (pl *Payload) push(x interface{}) {
	metadata, err := getHostMetadata()
	if err != nil {
		telemetryLogger.Printf("Error getting metadata %v", err)
	} else {
		err = saveHostMetadata(metadata)
		if err != nil {
			telemetryLogger.Printf("saving host metadata failed with :%v", err)
		}
	}

	if notExceeded, reportType := pl.payloadCapNotExceeded(x); notExceeded {
		switch reportType {
		case dnc:
			dncReport := x.(DNCReport)
			dncReport.Metadata = metadata
			pl.DNCReports = append(pl.DNCReports, dncReport)
		case cni:
			cniReport := x.(CNIReport)
			cniReport.Metadata = metadata
			pl.CNIReports = append(pl.CNIReports, cniReport)
		case npm:
			npmReport := x.(NPMReport)
			npmReport.Metadata = metadata
			pl.NPMReports = append(pl.NPMReports, npmReport)
		case cns:
			cnsReport := x.(CNSReport)
			cnsReport.Metadata = metadata
			pl.CNSReports = append(pl.CNSReports, cnsReport)
		}
	}
}

// reset - reset payload slices and sets payloadSize to 0
func (pl *Payload) reset() {
	pl.DNCReports = nil
	pl.DNCReports = make([]DNCReport, 0)
	pl.CNIReports = nil
	pl.CNIReports = make([]CNIReport, 0)
	pl.NPMReports = nil
	pl.NPMReports = make([]NPMReport, 0)
	pl.CNSReports = nil
	pl.CNSReports = make([]CNSReport, 0)
	payloadSize = 0
}

// payloadCapNotExceeded - Returns whether payload cap will be exceeded as a result of adding the new report; and the report type
//                         If the cap is not exceeded, we update the payload size here.
func (pl *Payload) payloadCapNotExceeded(x interface{}) (notExceeded bool, reportType string) {
	var body bytes.Buffer
	switch x.(type) {
	case DNCReport:
		reportType = dnc
		json.NewEncoder(&body).Encode(x.(DNCReport))
	case CNIReport:
		reportType = cni
		json.NewEncoder(&body).Encode(x.(CNIReport))
	case NPMReport:
		reportType = npm
		json.NewEncoder(&body).Encode(x.(NPMReport))
	case CNSReport:
		reportType = cns
		json.NewEncoder(&body).Encode(x.(CNSReport))
	}

	updatedPayloadSize := uint16(body.Len()) + payloadSize
	if notExceeded = updatedPayloadSize < MaxPayloadSize && payloadSize < updatedPayloadSize; notExceeded {
		payloadSize = updatedPayloadSize
	}

	return
}

// saveHostMetadata - save metadata got from wireserver to json file
func saveHostMetadata(metadata Metadata) error {
	dataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("[Telemetry] marshal data failed with err %+v", err)
	}

	if err = ioutil.WriteFile(metadataFile, dataBytes, 0644); err != nil {
		telemetryLogger.Printf("[Telemetry] Writing metadata to file failed: %v", err)
	}

	return err
}

// getHostMetadata - retrieve metadata from host
func getHostMetadata() (Metadata, error) {
	content, err := ioutil.ReadFile(metadataFile)
	if err == nil {
		var metadata Metadata
		if err = json.Unmarshal(content, &metadata); err == nil {
			telemetryLogger.Printf("[Telemetry] Returning hostmetadata from state")
			return metadata, nil
		}
	}

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

// StartTelemetryService - Kills if any telemetry service runs and start new telemetry service
func StartTelemetryService() error {
	platform.KillProcessByName(telemetryServiceProcessName)

	telemetryLogger.Printf("[Telemetry] Starting telemetry service process")
	path := fmt.Sprintf("%v/%v", cniInstallDir, telemetryServiceProcessName)
	if err := common.StartProcess(path); err != nil {
		telemetryLogger.Printf("[Telemetry] Failed to start telemetry service process :%v", err)
		return err
	}

	telemetryLogger.Printf("[Telemetry] Telemetry service started")

	for attempt := 0; attempt < 5; attempt++ {
		if checkIfSockExists() {
			break
		}

		time.Sleep(200 * time.Millisecond)
	}

	return nil
}
