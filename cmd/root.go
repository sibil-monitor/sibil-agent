package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sibil",
	Short: "Sibil Monitor CLI — expose your server metrics to the Sibil Monitor app",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(tokenCmd)
	rootCmd.AddCommand(actionsCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(tunnelCmd)
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(receiptCmd)
	rootCmd.AddCommand(redeemCmd)
	rootCmd.AddCommand(passesCmd)
	rootCmd.AddCommand(preflightCmd)
	rootCmd.AddCommand(uninstallCmd)
}
