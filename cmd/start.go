package cmd

import (
	"fmt"
	"os"

	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/sibil-monitor/sibil-agent/server"
	"github.com/sibil-monitor/sibil-agent/tunnel"
	"github.com/spf13/cobra"
)

var startHost string

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Sibil Monitor API server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintln(os.Stderr, "No config found. Run: sibil init")
				os.Exit(1)
			}
			return err
		}
		if startHost != "" {
			cfg.Host = startHost
		}
		if cfg.TunnelEnabled {
			go tunnel.Run(cfg.TunnelBackend, cfg.Token, cfg.Port)
		}
		return server.Run(cfg)
	},
}

func init() {
	startCmd.Flags().StringVar(&startHost, "host", "", "override bind address (e.g. 0.0.0.0 for LAN access via reverse proxy)")
}
