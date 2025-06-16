package main

import (
	utiljson "encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/component-base/logs"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
)

// Version is populated by make during build.
var version string

var (
	// path to a yaml or json files
	configPath = flag.String("input", "/etc/config/", `Name of the directory with configs to merge`)
	// merged config written to this directory
	outputPath = flag.String("output", "/etc/merged-config/", `Name of the directory to output the merged config`)
	// errors
	errAlignment = errors.New("ip not aligned to CIDR block")
)

const (
	// config files in this path must start with this to be read
	configFilePrefix = "ip-masq"
	// error formats
	cidrParseErrFmt = "CIDR %q could not be parsed: %w"
	cidrAlignErrFmt = "CIDR %q is not aligned to a CIDR block, ip: %q network: %q: %w"
)

type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	ReadDir(dirname string) ([]fs.DirEntry, error)
	DeleteFile(name string) error
}

type OSFileSystem struct{}

func (OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name) // nolint
}

func (OSFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm) // nolint
}

func (OSFileSystem) ReadDir(dirname string) ([]fs.DirEntry, error) {
	return os.ReadDir(dirname) // nolint
}

func (OSFileSystem) DeleteFile(name string) error {
	return os.Remove(name) // nolint
}

// name of nat chain for iptables masquerade rules
var resyncInterval = flag.Int("resync-interval", 60, "How often to refresh the config (in seconds)")

// MasqConfig object
type MasqConfig struct {
	NonMasqueradeCIDRs []string `json:"nonMasqueradeCIDRs"`
	MasqLinkLocal      bool     `json:"masqLinkLocal"`
	MasqLinkLocalIPv6  bool     `json:"masqLinkLocalIPv6"`
}

// EmptyMasqConfig returns a MasqConfig with empty values
func EmptyMasqConfig() *MasqConfig {
	return &MasqConfig{
		NonMasqueradeCIDRs: make([]string, 0),
		MasqLinkLocal:      false,
		MasqLinkLocalIPv6:  false,
	}
}

// MasqDaemon object
type MasqDaemon struct {
	config *MasqConfig
}

// NewMasqDaemon returns a MasqDaemon with default values
func NewMasqDaemon(c *MasqConfig) *MasqDaemon {
	return &MasqDaemon{
		config: c,
	}
}

func main() {
	klog.InitFlags(nil)

	flag.Parse()

	c := EmptyMasqConfig()

	logs.InitLogs()
	defer logs.FlushLogs()

	klog.Infof("Version: %s", version)

	verflag.PrintAndExitIfRequested()

	m := NewMasqDaemon(c)
	err := m.Run()
	if err != nil {
		klog.Fatalf("the daemon encountered an error: %v", err)
	}
}

func (m *MasqDaemon) Run() error {
	// Periodically resync
	for {
		// resync config
		err := m.osMergeConfig()
		if err != nil {
			return fmt.Errorf("error merging configuration: %w", err)
		}

		time.Sleep(time.Duration(*resyncInterval) * time.Second)
	}
}

func (m *MasqDaemon) osMergeConfig() error {
	var fs FileSystem = OSFileSystem{}
	return m.mergeConfig(fs)
}

// Syncs the config to the file at ConfigPath, or uses defaults if the file could not be found
// Error if the file is found but cannot be parsed.
func (m *MasqDaemon) mergeConfig(fileSys FileSystem) error {
	var err error
	c := EmptyMasqConfig()
	defer func() {
		if err == nil {
			json, marshalErr := utiljson.Marshal(c)
			if marshalErr == nil {
				klog.V(2).Infof("using config: %s", string(json))
			} else {
				klog.V(2).Info("could not marshal final config")
			}
		}
	}()

	files, err := fileSys.ReadDir(*configPath)
	if err != nil {
		return fmt.Errorf("failed to read config directory, error: %w", err)
	}

	var configAdded bool
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), configFilePrefix) {
			continue
		}
		var yaml []byte
		var json []byte

		klog.V(2).Infof("syncing config file %q at %q", file.Name(), *configPath)
		yaml, err = fileSys.ReadFile(filepath.Join(*configPath, file.Name()))
		if err != nil {
			return fmt.Errorf("failed to read config file %q, error: %w", file.Name(), err)
		}

		json, err = utilyaml.ToJSON(yaml)
		if err != nil {
			return fmt.Errorf("failed to convert config file %q to JSON, error: %w", file.Name(), err)
		}

		var newConfig MasqConfig
		err = utiljson.Unmarshal(json, &newConfig)
		if err != nil {
			return fmt.Errorf("failed to unmarshal config file %q, error: %w", file.Name(), err)
		}

		err = newConfig.validate()
		if err != nil {
			return fmt.Errorf("config file %q is invalid: %w", file.Name(), err)
		}
		c.merge(&newConfig)

		configAdded = true
	}

	mergedPath := filepath.Join(*outputPath, "ip-masq-agent")

	if !configAdded {
		// no valid config files found to merge-- remove any existing merged config file so ip masq agent uses defaults
		// the default config map is different from an empty config map
		klog.V(2).Infof("no valid config files found at %q, removing existing config map", *configPath)
		err = fileSys.DeleteFile(mergedPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to remove existing config file: %w", err)
		}
		return nil
	}

	// apply new config
	m.config = c

	out, err := yaml.Marshal(m.config)
	if err != nil {
		return fmt.Errorf("failed to marshal merged config to YAML: %w", err)
	}

	err = fileSys.WriteFile(mergedPath, out, 0o644)
	if err != nil {
		return fmt.Errorf("failed to write merged config: %w", err)
	}

	return nil
}

func (c *MasqConfig) validate() error {
	// check CIDRs are valid
	for _, cidr := range c.NonMasqueradeCIDRs {
		err := validateCIDR(cidr)
		if err != nil {
			return err
		}
	}
	return nil
}

// merge combines the existing MasqConfig with newConfig. The bools are OR'd together.
func (c *MasqConfig) merge(newConfig *MasqConfig) {
	if newConfig == nil {
		return
	}

	if len(newConfig.NonMasqueradeCIDRs) > 0 {
		c.NonMasqueradeCIDRs = mergeCIDRs(c.NonMasqueradeCIDRs, newConfig.NonMasqueradeCIDRs)
	}

	c.MasqLinkLocal = c.MasqLinkLocal || newConfig.MasqLinkLocal
	c.MasqLinkLocalIPv6 = c.MasqLinkLocalIPv6 || newConfig.MasqLinkLocalIPv6
}

// mergeCIDRS merges two slices of CIDRs into one, ignoring duplicates
func mergeCIDRs(cidrs1, cidrs2 []string) []string {
	cidrsSet := map[string]struct{}{}

	for _, cidr := range cidrs1 {
		cidrsSet[cidr] = struct{}{}
	}

	for _, cidr := range cidrs2 {
		cidrsSet[cidr] = struct{}{}
	}

	cidrsList := []string{}
	for cidr := range cidrsSet {
		cidrsList = append(cidrsList, cidr)
	}

	return cidrsList
}

func validateCIDR(cidr string) error {
	// parse test
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf(cidrParseErrFmt, cidr, err)
	}
	// alignment test
	if !ip.Equal(ipnet.IP) {
		return fmt.Errorf(cidrAlignErrFmt, cidr, ip, ipnet.String(), errAlignment)
	}
	return nil
}
