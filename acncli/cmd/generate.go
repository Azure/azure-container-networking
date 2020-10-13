package cmd

import (
	"fmt"

	c "github.com/Azure/azure-container-networking/acncli/api"
	"github.com/nxadm/tail"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// installCmd can register an object
func GenerateCmd() *cobra.Command {
	var registercmd = &cobra.Command{
		Use:   "generate",
		Short: "Generates a conflist or manifest for a specific component",
		Long:  "The logs command is used to fetch and/or watch the logs of an ACN component",
	}
	registercmd.AddCommand(LogsCNICmd())
	return registercmd
}

func GenerateConflistCmd() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "conflist",
		Short: fmt.Sprintf("Retrieves the logs of %s binary", c.AzureCNIBin),
		RunE: func(cmd *cobra.Command, args []string) error {

			fmt.Printf("%+v", viper.GetBool(c.FlagFollow))
			// this loop exists for when the logfile gets rotated, and tail loses the original file
			for {
				t, err := tail.TailFile(viper.GetString(c.FlagLogFilePath), tail.Config{Follow: viper.GetBool(c.FlagFollow)})

				if err != nil {
					return err
				}
				for line := range t.Lines {
					fmt.Println(line.Text)
				}
				if viper.GetBool(c.FlagFollow) == false {
					return nil
				}
			}
		}}

	cmd.Flags().String(c.FlagMode, c.Defaults[c.FlagMode], fmt.Sprintf("Datapath mode for Azure CNI, options are %s and %s", c.Transparent, c.Bridge))
	cmd.Flags().String(c.FlagIPAM, c.Defaults[c.FlagIPAM], fmt.Sprintf("Specify which IPAM source to use, options are %s and %s", c.AzureVNETIPAM, c.AzureCNSIPAM))
	cmd.Flags().String(c.FlagOS, c.Defaults[c.FlagOS], fmt.Sprintf("Specify which operating system, options are %s and %s", c.Linux, c.Windows))
	cmd.Flags().String(c.FlagTenancy, c.Defaults[c.FlagTenancy], fmt.Sprintf("Tenancy option for Azure CNI, options are %s and %s", c.Singletenancy, c.Multitenancy))
	cmd.Flags().String(c.FlagConflistDirectory, c.Defaults[c.FlagConflistDirectory], "Destination where Azure CNI conflists will be installed")
	cmd.Flags().String(c.FlagVersion, c.Defaults[c.FlagVersion], fmt.Sprintf("Version of Azure CNI to be installed, when running in manager mode, use %s as the version to install", c.Packaged))

	cmd.MarkFlagRequired(c.FlagMode)
	cmd.MarkFlagRequired(c.FlagIPAM)

	return cmd
}
