package aitelemetry

import (
	"strings"

	"github.com/pkg/errors"
)

type connectionVars struct {
	instrumentationKey string
	ingestionURL       string
}

func (c *connectionVars) String() string {
	return "InstrumentationKey=" + c.instrumentationKey + ";IngestionEndpoint=" + c.ingestionURL
}

func parseConnectionString(connectionString string) (*connectionVars, error) {
	connectionVars := &connectionVars{}

	if connectionString == "" {
		return nil, errors.New("connection string cannot be empty")
	}

	pairs := strings.Split(connectionString, ";")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return nil, errors.Errorf("invalid connection string format: %s", pair)
		}
		key, value := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])

		if key == "" {
			return nil, errors.Errorf("key in connection string cannot be empty")
		}

		switch strings.ToLower(key) {
		case "instrumentationkey":
			connectionVars.instrumentationKey = value
		case "ingestionendpoint":
			if value != "" {
				connectionVars.ingestionURL = value + "v2.1/track"
			}
		}
	}

	if connectionVars.instrumentationKey == "" || connectionVars.ingestionURL == "" {
		return nil, errors.Errorf("missing required fields in connection string: %s", connectionVars)
	}

	return connectionVars, nil
}
