package cmd

import (
	"fmt"

	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Show the current API token",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("no config found — run: sibil init")
		}
		fmt.Println(cfg.Token)
		return nil
	},
}
