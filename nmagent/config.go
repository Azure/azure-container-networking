package nmagent

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/nmagent/internal"
	"github.com/pkg/errors"
)

// Config is a configuration for an NMAgent Client.
type Config struct {
	/////////////////////
	// Required Config //
	/////////////////////
	Host string // the host the client will connect to
	Port uint16 // the port the client will connect to

	/////////////////////
	// Optional Config //
	/////////////////////
	UseTLS bool // forces all connections to use TLS
}

// Validate reports whether this configuration is a valid configuration for a
// client.
func (c Config) Validate() error {
	err := internal.ValidationError{}

	if c.Host == "" {
		err.MissingFields = append(err.MissingFields, "Host")
	}

	if c.Port == 0 {
		err.MissingFields = append(err.MissingFields, "Port")
	}

	if err.IsEmpty() {
		return nil
	}
	return err
}

// NewConfig returns a nmagent client config using the provided wireserverIP string
func NewConfig(wireserverIP string) (Config, error) {
	host := "168.63.129.16" // wireserver's IP

	if wireserverIP != "" {
		host = wireserverIP
	}

	if strings.Contains(host, "http") {
		parts, err := url.Parse(host)
		if err != nil {
			return Config{}, errors.Wrap(err, "parsing WireserverIP as URL")
		}
		host = parts.Host
	}

	if strings.Contains(host, ":") {
		host, prt, err := net.SplitHostPort(host) //nolint:govet // it's fine to shadow ergit r here
		if err != nil {
			return Config{}, errors.Wrap(err, "splitting wireserver IP into host port")
		}

		port, err := strconv.ParseUint(prt, 10, 16) //nolint:gomnd // obvious from ParseUint docs
		if err != nil {
			return Config{}, errors.Wrap(err, "parsing wireserver port value as uint16")
		}

		return Config{
			Host: host,
			Port: uint16(port),
		}, nil
	}

	return Config{
		Host: host,

		// nolint:gomnd // there's no benefit to constantizing a well-known port
		Port: 80,
	}, nil
}
