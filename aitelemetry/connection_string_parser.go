package aitelemetry

import (
	"fmt"
	"strings"
)

type connectionVars struct {
	InstrumentationKey string
	IngestionUrl       string
}

func parseConnectionString(connectionString string) (*connectionVars, error) {
	connectionVars := &connectionVars{}

	if connectionString == "" {
		return nil, fmt.Errorf("Connection string cannot be empty")
	}

	pairs := strings.Split(connectionString, ";")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("Invalid connection string format: %s", pair)
		}
		key, value := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])

		if key == "" {
			return nil, fmt.Errorf("Key in connection string cannot be empty")
		}

		switch strings.ToLower(key) {
		case "instrumentationkey":
			connectionVars.InstrumentationKey = value
		case "ingestionendpoint":
			connectionVars.IngestionUrl = value + "v2.1/track"
		}
	}

	if connectionVars.InstrumentationKey == "" || connectionVars.IngestionUrl == "" {
		return nil, fmt.Errorf("Missing required fields in connection string")
	}

	return connectionVars, nil
}
