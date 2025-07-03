package appinsights

import (
	"errors"
	"strings"
)

type ConnectionParams struct {
	InstrumentationKey string
	IngestionEndpoint  string
}

func parseConnectionString(cString string) (*ConnectionParams, error) {
	connectionParams := &ConnectionParams{}

	pairs := strings.Split(cString, ";")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return nil, errors.New("invalid connection parameter format")
		}

		key, value := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])

		switch strings.ToLower(key) {
		case "instrumentationkey":
			connectionParams.InstrumentationKey = value
		case "ingestionendpoint":
			if value != "" {
				connectionParams.IngestionEndpoint = value + "v2/track"
			}
		}
	}

	if connectionParams.InstrumentationKey == "" || connectionParams.IngestionEndpoint == "" {
		return nil, errors.New("missing required connection parameters")
	}
	return connectionParams, nil
}
