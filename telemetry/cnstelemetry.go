// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"reflect"
	"regexp"
	"time"

	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/google/uuid"
)

const (
	// CNSTelemetryFile - telemetry file path.
	CNSTelemetryFile                = platform.CNSRuntimePath + "AzureCNSTelemetry.json"
	errorcodePrefix                 = 5
	heartbeatIntervalInMinutes      = 30
	retryWaitTimeInSeconds          = 60
	telemetryNumRetries             = 5
	telemetryWaitTimeInMilliseconds = 200
)

var codeRegex = regexp.MustCompile(`Code:(\w*)`)

// SendCnsTelemetry - handles cns telemetry reports
func SendCnsTelemetry(reports chan interface{}, service *restserver.HTTPRestService, telemetryStopProcessing chan bool) {

CONNECT:
	tb := NewTelemetryBuffer("")
	tb.ConnectToTelemetryService(telemetryNumRetries, telemetryWaitTimeInMilliseconds)
	if tb.Connected {
		heartbeat := time.NewTicker(time.Minute * heartbeatIntervalInMinutes).C
		reportMgr := ReportManager{
			ContentType: ContentType,
			Report:      &CNSReport{},
		}

		reportMgr.GetReportState(CNSTelemetryFile)
		reportMgr.GetKernelVersion()

		for {
			// Try to set partition key from DNC
			if reportMgr.Report.(*CNSReport).DncPartitionKey == "" {
				reflect.ValueOf(reportMgr.Report).Elem().FieldByName("DncPartitionKey").SetString(service.GetPartitionKey())
			}

			select {
			case <-heartbeat:
				reflect.ValueOf(reportMgr.Report).Elem().FieldByName("EventMessage").SetString("Heartbeat")
			case msg := <-reports:
				codeStr := codeRegex.FindString(msg.(string))
				if len(codeStr) > errorcodePrefix {
					reflect.ValueOf(reportMgr.Report).Elem().FieldByName("Errorcode").SetString(codeStr[errorcodePrefix:])
				}

				reflect.ValueOf(reportMgr.Report).Elem().FieldByName("EventMessage").SetString(msg.(string))
			case <-telemetryStopProcessing:
				tb.Close()
				return
			}

			reflect.ValueOf(reportMgr.Report).Elem().FieldByName("Timestamp").SetString(time.Now().UTC().String())
			if id, err := uuid.NewUUID(); err == nil {
				reflect.ValueOf(reportMgr.Report).Elem().FieldByName("UUID").SetString(id.String())
			}

			if !reportMgr.GetReportState(CNSTelemetryFile) {
				reportMgr.SetReportState(CNSTelemetryFile)
			}

			report, err := reportMgr.ReportToBytes()
			if err == nil {
				// If write fails, try to re-establish connections as server/client
				if _, err = tb.Write(report); err != nil {
					log.Logf("[CNS-Telemetry] Telemetry write failed: %v", err)
					tb.Close()
					goto CONNECT
				}
			}
		}
	}
}
