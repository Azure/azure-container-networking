package cli

func init() {
	debugCmd.AddCommand(getTuplesCmd)
	getTuplesCmd.Flags().StringP("src", "s", "", "set the source")
	getTuplesCmd.Flags().StringP("dst", "d", "", "set the destination")
	getTuplesCmd.Flags().StringP("iptF", "i", "", "Set the iptable-save file path (optional)")
	getTuplesCmd.Flags().StringP("npmF", "n", "", "Set the NPM cache file path (optional)")
}
