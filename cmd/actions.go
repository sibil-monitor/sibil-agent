package cmd

import (
	"fmt"

	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/spf13/cobra"
)

var actionsCmd = &cobra.Command{
	Use:   "actions <enable|disable>",
	Short: "Enable or disable destructive service actions (restart/stop/start)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("cannot load config: %w", err)
		}

		switch args[0] {
		case "enable":
			cfg.AllowActions = true
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("cannot save config: %w", err)
			}
			fmt.Println("✓ Actions enabled.")
			fmt.Println("  restart / stop / start are now available via the app.")
			fmt.Println("  Restart sibil for the change to take effect.")
		case "disable":
			cfg.AllowActions = false
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("cannot save config: %w", err)
			}
			fmt.Println("✓ Actions disabled.")
			fmt.Println("  The CLI will return 403 on restart / stop / start requests.")
			fmt.Println("  Restart sibil for the change to take effect.")
		default:
			return fmt.Errorf("unknown argument %q — use enable or disable", args[0])
		}
		return nil
	},
}
