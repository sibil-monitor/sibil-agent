package cmd

import (
	"fmt"

	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/spf13/cobra"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel <enable|disable>",
	Short: "Enable or disable the WebSocket tunnel relay via monitor.cordee.ovh",
	Long: `When enabled, the CLI connects to the Sibil Monitor WebSocket relay
so the mobile app can reach it without a reverse proxy or open port.

The backend routes opaque bytes — no infrastructure data is ever stored there.
Your CLI token is the shared secret between the CLI and the mobile app.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("cannot load config: %w", err)
		}
		switch args[0] {
		case "enable":
			cfg.TunnelEnabled = true
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("cannot save config: %w", err)
			}
			fmt.Println("✓ Tunnel enabled.")
			fmt.Printf("  Relay : %s\n", cfg.TunnelBackend)
			fmt.Println("  Restart sibil for the change to take effect.")
			fmt.Println()
			fmt.Println("In the mobile app: choose \"Connect via Sibil Tunnel\" and enter your token.")
		case "disable":
			cfg.TunnelEnabled = false
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("cannot save config: %w", err)
			}
			fmt.Println("✓ Tunnel disabled.")
			fmt.Println("  The CLI will only be accessible via direct URL.")
			fmt.Println("  Restart sibil for the change to take effect.")
		default:
			return fmt.Errorf("unknown argument %q — use enable or disable", args[0])
		}
		return nil
	},
}
