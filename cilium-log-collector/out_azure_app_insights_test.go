package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/stretchr/testify/require"
)

// mockAppInsightsTracker captures tracked telemetry for testing
type MockAppInsightsTracker struct {
	TrackedItems []appinsights.Telemetry
}

func (m *MockAppInsightsTracker) Track(telemetry appinsights.Telemetry) {
	m.TrackedItems = append(m.TrackedItems, telemetry)
}

func NewMockAppInsightsTracker() *MockAppInsightsTracker {
	return &MockAppInsightsTracker{TrackedItems: make([]appinsights.Telemetry, 0)}
}

func TestProcessSingleRecord_BasicLogging(t *testing.T) {
	tracker := NewMockAppInsightsTracker()
	processor := &RecordProcessor{
		tracker: tracker,
		tag:     "test.tag",
		debug:   false,
		logKey:  "log",
	}

	record := ProcessRecord{
		Timestamp: time.Now(),
		Fields: map[interface{}]interface{}{
			"log":   "test",
			"level": "info",
			"app":   "test-app",
		},
	}

	processor.ProcessSingleRecord(record, 0)

	require.Len(t, tracker.TrackedItems, 1)

	firstTrace := tracker.TrackedItems[0].(*appinsights.TraceTelemetry)
	require.Equal(t, "test", firstTrace.Message)
	require.Equal(t, "info", firstTrace.Properties["level"])
	require.Equal(t, "test.tag", firstTrace.Properties["fluentbit_tag"])
	require.Equal(t, "0", firstTrace.Properties["record_count"])
}

func TestProcessSingleRecord_CustomLogKey(t *testing.T) {
	tracker := NewMockAppInsightsTracker()
	processor := &RecordProcessor{
		tracker: tracker,
		tag:     "custom.tag",
		debug:   false,
		logKey:  "message",
	}

	record := ProcessRecord{
		Timestamp: time.Now(),
		Fields: map[interface{}]interface{}{
			"message": "c",
			"level":   "warn",
		},
	}

	processor.ProcessSingleRecord(record, 5)

	require.Len(t, tracker.TrackedItems, 1)

	firstTrace := tracker.TrackedItems[0].(*appinsights.TraceTelemetry)
	require.Equal(t, "c", firstTrace.Message)
	require.Equal(t, "warn", firstTrace.Properties["level"])
	require.Equal(t, "5", firstTrace.Properties["record_count"])
}

func TestProcessSingleRecord_MultipleRecords(t *testing.T) {
	tracker := NewMockAppInsightsTracker()
	processor := &RecordProcessor{
		tracker: tracker,
		tag:     "multi.tag",
		debug:   false,
		logKey:  "log",
	}

	records := []ProcessRecord{
		{
			Timestamp: time.Now(),
			Fields: map[interface{}]interface{}{
				"log":   "11",
				"level": "info",
			},
		},
		{
			Timestamp: time.Now(),
			Fields: map[interface{}]interface{}{
				"log":   "22",
				"level": "error",
			},
		},
	}

	for i, record := range records {
		processor.ProcessSingleRecord(record, i)
	}

	require.Len(t, tracker.TrackedItems, 2)

	firstTrace := tracker.TrackedItems[0].(*appinsights.TraceTelemetry)
	require.Equal(t, "11", firstTrace.Message)
	require.Equal(t, "0", firstTrace.Properties["record_count"])

	secondTrace := tracker.TrackedItems[1].(*appinsights.TraceTelemetry)
	require.Equal(t, "22", secondTrace.Message)
	require.Equal(t, "1", secondTrace.Properties["record_count"])
}

type Blah struct {
	Blah string
}

func TestProcessSingleRecord_NestedMapConversion(t *testing.T) {
	tracker := NewMockAppInsightsTracker()
	processor := &RecordProcessor{
		tracker: tracker,
		tag:     "nested.tag",
		debug:   false,
		logKey:  "log",
	}

	record := ProcessRecord{
		Timestamp: time.Now(),
		Fields: map[interface{}]interface{}{
			"log": "Test message",
			"metadata": map[interface{}]interface{}{
				"nested_key": "nested_value",
				"count":      11,
				"metadata2": map[interface{}]interface{}{
					"inner_key":   "inner_value",
					"inner_count": 123,
					"metadata3": map[interface{}]interface{}{
						"enabled": true,
						"data":    []byte{57, 57, 54, 50},
						"data2":   []byte{107, 117, 98, 101, 45, 115, 121, 115, 116, 101, 109},
						"hi":      Blah{Blah: "aaah"},
					},
				},
			},
		},
	}

	processor.ProcessSingleRecord(record, 0)

	require.Len(t, tracker.TrackedItems, 1)

	firstTrace := tracker.TrackedItems[0].(*appinsights.TraceTelemetry)
	require.Equal(t, "Test message", firstTrace.Message)

	// check that nested metadata was converted to JSON string and parse it
	metadataJSON := firstTrace.Properties["metadata"]
	require.NotEmpty(t, metadataJSON)

	// parse the JSON to verify structure
	var parsedMetadata map[string]interface{}
	err := json.Unmarshal([]byte(metadataJSON), &parsedMetadata)
	require.NoError(t, err)

	// verify nesting
	require.Equal(t, "nested_value", parsedMetadata["nested_key"])
	require.Equal(t, "11", parsedMetadata["count"])

	metadata2, ok := parsedMetadata["metadata2"].(map[string]interface{})
	require.True(t, ok, "metadata2 should be a map")
	require.Equal(t, "inner_value", metadata2["inner_key"])
	require.Equal(t, "123", metadata2["inner_count"])

	metadata3, ok := metadata2["metadata3"].(map[string]interface{})
	require.True(t, ok, "metadata3 should be a map")
	require.Equal(t, "true", metadata3["enabled"])

	// verify byte arrays are converted to strings
	require.Equal(t, "9962", metadata3["data"])         // [57 57 54 50] -> "9962"
	require.Equal(t, "kube-system", metadata3["data2"]) // [107 117 98 101 45 115 121 115 116 101 109] -> "kube-system"

	// %v representation of the struct
	require.Equal(t, "{aaah}", metadata3["hi"])
}

func TestProcessSingleRecord_EmptyLogMessage(t *testing.T) {
	tracker := NewMockAppInsightsTracker()
	processor := &RecordProcessor{
		tracker: tracker,
		tag:     "empty.tag",
		debug:   false,
		logKey:  "log",
	}

	record := ProcessRecord{
		Timestamp: time.Now(),
		Fields: map[interface{}]interface{}{
			"level": "info",
			"app":   "test-app",
		},
	}

	processor.ProcessSingleRecord(record, 0)

	require.Len(t, tracker.TrackedItems, 1)

	firstTrace := tracker.TrackedItems[0].(*appinsights.TraceTelemetry)
	require.Equal(t, "", firstTrace.Message)
	require.Equal(t, "info", firstTrace.Properties["level"])
}

func TestConvertToString_VariousTypes(t *testing.T) {
	// test conversion
	require.Equal(t, "test", convertToString("test"))

	require.Equal(t, "bytes", convertToString([]byte("bytes")))

	require.Equal(t, "11", convertToString(11))

	testMap := map[interface{}]interface{}{
		"key1": "value1",
		"key2": 123,
	}
	result := convertToString(testMap)
	require.Contains(t, result, `"key1":"value1"`)
	require.Contains(t, result, `"key2":"123"`)
}

func TestMockAppInsightsTracker_TrackMultiple(t *testing.T) {
	tracker := NewMockAppInsightsTracker()

	trace1 := appinsights.NewTraceTelemetry("Message 1", appinsights.Information)
	trace2 := appinsights.NewTraceTelemetry("Message 2", appinsights.Warning)

	tracker.Track(trace1)
	tracker.Track(trace2)

	require.Len(t, tracker.TrackedItems, 2)

	firstTrace := tracker.TrackedItems[0].(*appinsights.TraceTelemetry)
	require.Equal(t, "Message 1", firstTrace.Message)

	secondTrace := tracker.TrackedItems[1].(*appinsights.TraceTelemetry)
	require.Equal(t, "Message 2", secondTrace.Message)
}

func TestRecordProcessor_DebugMode(t *testing.T) {
	tracker := NewMockAppInsightsTracker()
	processor := &RecordProcessor{
		tracker: tracker,
		tag:     "debug.tag",
		debug:   true,
		logKey:  "log",
	}

	record := ProcessRecord{
		Timestamp: time.Now(),
		Fields: map[interface{}]interface{}{
			"log":   "dbg",
			"level": "debug",
		},
	}

	// this test mainly ensures debug mode doesn't break processing
	processor.ProcessSingleRecord(record, 0)

	require.Len(t, tracker.TrackedItems, 1)

	firstTrace := tracker.TrackedItems[0].(*appinsights.TraceTelemetry)
	require.Equal(t, "dbg", firstTrace.Message)
}
