// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// FdName - file descriptor name
// Delimiter - delimiter for socket reads/writes
// HostNetAgentURL - host net agent url of type payload
// DefaultDncReportsSize - default DNC report slice size
// DefaultCniReportsSize - default CNI report slice size
// DefaultNpmReportsSize - default NPM report slice size
// DefaultInterval - default interval for sending payload to host
const (
	FdName          = "azure-telemetry"
	Delimiter       = '\n'
	HostNetAgentURL = "http://169.254.169.254/machine/plugins?comp=netagent&type=payload"
	DefaultInterval = 1 * time.Minute
)

// TelemetryBuffer object
type TelemetryBuffer struct {
	client      net.Conn
	listener    net.Listener
	connections []net.Conn
	payload     Payload
	fdExists    bool
	connected   bool
	data        chan interface{}
	cancel      chan bool
}

// Payload object holds the different types of reports
type Payload struct {
	DNCReports []DNCReport
	CNIReports []CNIReport
	NPMReports []NPMReport
}

// NewTelemetryBuffer - create a new TelemetryBuffer
func NewTelemetryBuffer() (*TelemetryBuffer, error) {
	var tb TelemetryBuffer
	tb.data = make(chan interface{})
	tb.cancel = make(chan bool, 1)
	tb.connections = make([]net.Conn, 1)
	err := tb.Listen(FdName)
	if err != nil {
		tb.fdExists = strings.Contains(err.Error(), "in use") || strings.Contains(err.Error(), "Access is denied")
	} else {
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
								}
							}
						}
					}()
				}
			}
		}()
	}

	err = tb.Dial(FdName)
	if err == nil {
		tb.connected = true
		tb.payload.DNCReports = make([]DNCReport, 0)
		tb.payload.CNIReports = make([]CNIReport, 0)
		tb.payload.NPMReports = make([]NPMReport, 0)
	} else if tb.fdExists {
		tb.cleanup(FdName)
	}

	return &tb, err
}

// Start - start running an instance if it isn't already being run elsewhere
func (tb *TelemetryBuffer) Start(intervalms time.Duration) {
	defer tb.close()
	if !tb.fdExists && tb.connected {
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
				}
			case report := <-tb.data:
				tb.payload.push(report)
			case <-tb.cancel:
				goto EXIT
			}
		}
	} else {
		<-tb.cancel
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
		w.Flush()
	}

	return
}

// Cancel - signal to tear down telemetry buffer
func (tb *TelemetryBuffer) Cancel() {
	tb.cancel <- true
}

// close - close all connections
func (tb *TelemetryBuffer) close() {
	if tb.client != nil {
		tb.client.Close()
	}

	if tb.listener != nil {
		tb.listener.Close()
	}

	for _, conn := range tb.connections {
		if conn != nil {
			conn.Close()
		}
	}
}

// sendToHost - send payload to host
func (tb *TelemetryBuffer) sendToHost() error {
	httpc := &http.Client{}
	var body bytes.Buffer
	json.NewEncoder(&body).Encode(tb.payload)
	resp, err := httpc.Post(HostNetAgentURL, ContentType, &body)
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
	switch x.(type) {
	case DNCReport:
		pl.DNCReports = append(pl.DNCReports, x.(DNCReport))
	case CNIReport:
		pl.CNIReports = append(pl.CNIReports, x.(CNIReport))
	case NPMReport:
		pl.NPMReports = append(pl.NPMReports, x.(NPMReport))
	}
}

// reset - reset payload slices
func (pl *Payload) reset() {
	pl.DNCReports = nil
	pl.DNCReports = make([]DNCReport, 0)
	pl.CNIReports = nil
	pl.CNIReports = make([]CNIReport, 0)
	pl.NPMReports = nil
	pl.NPMReports = make([]NPMReport, 0)
}
