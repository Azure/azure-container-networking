package metrics

import (
	"fmt"
	"strconv"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/npm/util"
)

// Printf logs in the AI telemetry
func Printf(errorCode int, packageName, functionName, format string, args ...interface{}) {
	if th == nil {
		return
	}

	msg := fmt.Sprintf(format, args...)
	customDimensions := map[string]string {
		util.PackageName: packageName,
		util.FunctionName: functionName,
	}
	report := aitelemetry.Report{
		Message:          msg,
		Context:          strconv.Itoa(errorCode),
		CustomDimensions: customDimensions,
	}
	th.TrackLog(report)
}