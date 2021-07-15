package cmd

import (
	"fmt"

	c "github.com/Azure/azure-container-networking/tools/acncli/api"
	"github.com/nxadm/tail"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// LogsCmd will write the logs of the Azure CNI logs
func LogsCmd() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "logs",
		Short: "Fetches the logs of an ACN component",
		Long:  "The logs command is used to fetch and/or watch the logs of an ACN component",
	}
	cmd.AddCommand(LogsCNICmd())
	return cmd
}

func LogsCNICmd() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "cni",
		Short: fmt.Sprintf("Retrieves the logs of %s binary", c.AzureCNIBin),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("📃 - started watching %s\n", viper.GetString(c.FlagLogFilePath))

			// this loop exists for when the logfile gets rotated, and tail loses the original file
			for {
				t, err := tail.TailFile(viper.GetString(c.FlagLogFilePath), tail.Config{Follow: viper.GetBool(c.FlagFollow), ReOpen: true})

				if err != nil {
					return err
				}
				for line := range t.Lines {
					fmt.Println(line.Text)
				}
				if !viper.GetBool(c.FlagFollow) {
					return nil
				}
			}
		}}

	cmd.Flags().BoolP(c.FlagFollow, "f", c.DefaultToggles[c.FlagFollow], "Follow the log file, similar to 'tail -f'")
	cmd.Flags().String(c.FlagLogFilePath, c.Defaults[c.FlagLogFilePath], "Path of the Azure CNI log file")

	return cmd
}
