package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	CiliumPoolConfigPath     = "/etc/azure-vnet/cilium-pool.json"
	OptEnvironmentCiliumPool = "cilium-pool"
)

var errCiliumPoolConfig = errors.New("invalid cilium pool configuration")

// CiliumPoolConfig is host configuration shared by the bridge and both CNI plugins.
// Installation and changes require a drained node; this is not conflist input.
type CiliumPoolConfig struct {
	NodeName    string `json:"nodeName"`
	StateDir    string `json:"stateDir"`
	Interface   string `json:"interface"`
	NICID       string `json:"nicID"`
	Subnet      string `json:"subnet"`
	MinAllocate int    `json:"minAllocate"`
	PreAllocate int    `json:"preAllocate"`
}

func LoadCiliumPoolConfig() (*CiliumPoolConfig, error) {
	return ReadCiliumPoolConfig(CiliumPoolConfigPath)
}

func ReadCiliumPoolConfig(path string) (*CiliumPoolConfig, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening cilium pool config: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("reading cilium pool config metadata: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("%w: config must be a regular file without group/other write access", errCiliumPoolConfig)
	}
	var cfg CiliumPoolConfig
	d := json.NewDecoder(io.LimitReader(f, 65536))
	d.DisallowUnknownFields()
	if err := d.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decoding cilium pool config: %w", err)
	}
	if err := d.Decode(new(interface{})); err != io.EOF {
		return nil, fmt.Errorf("%w: config contains trailing data", errCiliumPoolConfig)
	}
	if cfg.NodeName == "" || cfg.Interface == "" || cfg.NICID == "" || cfg.Subnet == "" ||
		!filepath.IsAbs(cfg.StateDir) || filepath.Clean(cfg.StateDir) == "/" ||
		cfg.MinAllocate < 0 || cfg.PreAllocate < 1 {
		return nil, errCiliumPoolConfig
	}
	clean := filepath.Clean(cfg.StateDir)
	if clean != cfg.StateDir || clean == "/run" || clean == "/var/run" ||
		strings.HasPrefix(clean, "/run/") || strings.HasPrefix(clean, "/var/run/") {
		return nil, fmt.Errorf("%w: state requires a clean durable path outside /run", errCiliumPoolConfig)
	}
	if err := validateCiliumPoolPaths(path, cfg.StateDir); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *CiliumPoolConfig) StatePath(plugin string) string {
	return filepath.Join(c.StateDir, plugin+".db")
}
