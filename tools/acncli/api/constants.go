//go:build !ignore_uncovered
// +build !ignore_uncovered

package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	CNI = "cni"

	EnvPrefix = "AZURE_CNI"

	// CNI Install Flags
	FlagMode              = "mode"
	FlagTarget            = "target"
	FlagIPAM              = "ipam"
	FlagOS                = "os"
	FlagTenancy           = "tenancy"
	FlagExempt            = "exempt"
	FlagBinDirectory      = "bin-directory"
	FlagConflistDirectory = "conflist-directory"
	FlagVersion           = "version"

	// CNI Log Flags
	FlagFollow      = "follow"
	FlagLogFilePath = "log-file"

	// tenancy flags
	Singletenancy = "singletenancy"
	Multitenancy  = "multitenancy"

	// os flags
	Linux   = "linux"
	Windows = "windows"

	// arch flags
	Amd64 = "amd64"

	// target mode flags
	Local   = "local"
	Cluster = "cluster"

	// File permissions
	BinPerm      = 755
	ConflistPerm = 644

	// CNI versions
	Latest   = "latest"
	Packaged = "packaged"

	AzureCNIBin          = "azure-vnet"
	AzureTelemetryBin    = "azure-vnet-telemetry"
	AzureTelemetryConfig = "azure-vnet-telemetry.config"
	AzureCNSIPAM         = "azure-cns"
	AzureVNETIPAM        = "azure-vnet-ipam"
	ConflistExtension    = ".conflist"

	DefaultSrcDirLinux      = "/output/"
	DefaultBinDirLinux      = "/opt/cni/bin/"
	DefaultConflistDirLinux = "/etc/cni/net.d/"
	DefaultLogFile          = "/var/log/azure-vnet.log"
	Transparent             = "transparent"
	Bridge                  = "bridge"
	Azure0                  = "azure0"
)

var (
	// Concatenating flags to the env ensures consistency between flags and env's for viper and cobra
	EnvCNIOS                     = EnvPrefix + "_" + strings.ToUpper(FlagOS)
	EnvCNIType                   = EnvPrefix + "_" + strings.ToUpper(FlagTenancy)
	EnvCNISourceDir              = EnvPrefix + "_" + "SRC_DIR"
	EnvCNIDestinationBinDir      = EnvPrefix + "_" + "BIN_DIR"
	EnvCNIDestinationConflistDir = EnvPrefix + "_" + "CONFLIST_DIR"
	EnvCNIIPAMType               = EnvPrefix + "_" + strings.ToUpper(FlagIPAM)
	EnvCNIMode                   = EnvPrefix + "_" + strings.ToUpper(FlagMode)
	EnvCNIExemptBins             = EnvPrefix + "_" + strings.ToUpper(FlagExempt)
	EnvCNILogFile                = EnvPrefix + "_" + "LOG_FILE"

	Defaults = map[string]string{
		FlagOS:                       Linux,
		FlagTenancy:                  Singletenancy,
		FlagIPAM:                     AzureVNETIPAM,
		FlagExempt:                   AzureTelemetryBin + "," + AzureTelemetryConfig,
		FlagMode:                     Transparent,
		FlagTarget:                   Local,
		FlagBinDirectory:             DefaultBinDirLinux,
		FlagConflistDirectory:        DefaultConflistDirLinux,
		FlagVersion:                  Packaged,
		FlagLogFilePath:              DefaultLogFile,
		EnvCNILogFile:                EnvCNILogFile,
		EnvCNISourceDir:              DefaultSrcDirLinux,
		EnvCNIDestinationBinDir:      DefaultBinDirLinux,
		EnvCNIDestinationConflistDir: DefaultConflistDirLinux,
	}

	DefaultToggles = map[string]bool{
		FlagFollow: false,
	}
)

func GetDefaults() map[string]string {
	return Defaults
}

func PrettyPrint(b interface{}) {
	s, _ := json.MarshalIndent(b, "", "\t")
	fmt.Print(string(s))
}
