//go:build !ignore_uncovered
// +build !ignore_uncovered

package cni

import (
	"fmt"
	"strings"

	c "github.com/Azure/azure-container-networking/tools/acncli/api"
	i "github.com/Azure/azure-container-networking/tools/acncli/installer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// install CNI will install CNI to a local machine
func InstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Installs an ACN component",
	}
	cmd.AddCommand(InstallCNICmd())
	return cmd
}

func InstallCNICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cni",
		Short: "Installs CNI and conflist ",
		RunE: func(cmd *cobra.Command, args []string) error {
			envs := i.InstallerConfig{
				ExemptBins: make(map[string]bool),
			}

			// only allow windows and linux binaries
			if err := envs.SetOSType(viper.GetString(c.FlagOS)); err != nil {
				return err
			}

			// only allow windows and linux binaries
			if err := envs.SetCNIType(viper.GetString(c.FlagTenancy)); err != nil {
				return err
			}

			// only allow windows and linux binaries
			if err := envs.SetCNIDatapathMode(viper.GetString(c.FlagMode)); err != nil {
				return err
			}

			envs.SetExempt(strings.Split(strings.Replace(strings.ToLower(viper.GetString(c.FlagExempt)), " ", "", -1), ","))

			envs.SrcDir = fmt.Sprintf("%s%s/%s/", c.DefaultSrcDirLinux, envs.OSType, envs.CNITenancy)
			envs.DstBinDir = viper.GetString(c.FlagBinDirectory)
			envs.DstConflistDir = viper.GetString(c.FlagConflistDirectory)
			envs.IPAMType = viper.GetString(c.FlagIPAM)

			return i.InstallLocal(envs)
		},
	}

	cmd.Flags().String(c.FlagMode, c.Defaults[c.FlagMode], fmt.Sprintf("Datapath mode for Azure CNI, options are %s and %s", c.Transparent, c.Bridge))
	cmd.Flags().String(c.FlagTarget, c.Defaults[c.FlagTarget], fmt.Sprintf("Location to install Azure CNI, options are %s and %s", c.Local, c.Cluster))
	cmd.Flags().String(c.FlagIPAM, c.Defaults[c.FlagIPAM], fmt.Sprintf("Specify which IPAM source to use, options are %s and %s", c.AzureVNETIPAM, c.AzureCNSIPAM))
	cmd.Flags().String(c.FlagOS, c.Defaults[c.FlagOS], fmt.Sprintf("Specify which operating system to install, options are %s and %s", c.Linux, c.Windows))
	cmd.Flags().String(c.FlagTenancy, c.Defaults[c.FlagTenancy], fmt.Sprintf("Tenancy option for Azure CNI, options are %s and %s", c.Singletenancy, c.Multitenancy))
	cmd.Flags().String(c.FlagBinDirectory, c.Defaults[c.FlagBinDirectory], "Destination where Azure CNI binaries will be installed")
	cmd.Flags().String(c.FlagConflistDirectory, c.Defaults[c.FlagConflistDirectory], "Destination where Azure CNI conflists will be installed")
	cmd.Flags().String(c.FlagExempt, c.Defaults[c.FlagExempt], "Exempt files that won't be installed")

	return cmd
}
