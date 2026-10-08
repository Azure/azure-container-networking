// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"strconv"
	"testing"
	"time"
)

func testFDName(t *testing.T) string {
	t.Helper()
	return FdName + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}
