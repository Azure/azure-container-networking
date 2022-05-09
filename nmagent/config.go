package nmagent

import "github.com/Azure/azure-container-networking/nmagent/internal"

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
