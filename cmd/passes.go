package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/sibil-monitor/sibil-agent/server"
	"github.com/spf13/cobra"
)

var passesCmd = &cobra.Command{
	Use:   "passes",
	Short: "Show local service pass status",
	Long: `sibil passes lists stored service passes and their current status.
Passes are created by: sibil redeem --code <CLAIM-CODE>
They are stored locally in .sibil-passes/ and never uploaded.`,
	RunE: runPasses,
}

func runPasses(_ *cobra.Command, _ []string) error {
	cfg, _ := config.Load()

	agentID := ""
	if cfg != nil && cfg.Token != "" {
		agentID = tokenToAgentID(cfg.Token)
	}

	offers := []struct {
		key   string
		label string
	}{
		{"setup_199", "VPS Health Setup — EUR 199"},
		{"audit_690", "AI Stack Observability Audit — EUR 690"},
	}

	fmt.Println("Service passes:")
	fmt.Println()

	anyFound := false
	for _, o := range offers {
		path := filepath.Join(passStorageDir, o.key+".pass")
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("  %-12s  not found\n", o.key)
			continue
		}
		anyFound = true

		token := strings.TrimSpace(string(data))
		claims, err := server.VerifyServicePass(token, agentID)
		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "expired") {
				fmt.Printf("  %-12s  expired       %s\n", o.key, o.label)
			} else if strings.Contains(msg, "different server") {
				fmt.Printf("  %-12s  wrong server  %s\n", o.key, o.label)
			} else {
				fmt.Printf("  %-12s  invalid       %s  (%v)\n", o.key, o.label, err)
			}
			continue
		}

		remaining := time.Until(time.Unix(claims.Exp, 0))
		until := time.Unix(claims.Exp, 0).UTC().Format("2006-01-02 15:04 UTC")
		hrs := int(remaining.Hours())
		mins := int(remaining.Minutes()) % 60

		var timeLeft string
		switch {
		case remaining < 0:
			timeLeft = "expired"
		case hrs >= 24:
			timeLeft = fmt.Sprintf("%dd remaining", hrs/24)
		case hrs > 0:
			timeLeft = fmt.Sprintf("%dh%dm remaining", hrs, mins)
		default:
			timeLeft = fmt.Sprintf("%dm remaining", mins)
		}

		fmt.Printf("  %-12s  active        %s\n", o.key, o.label)
		fmt.Printf("  %s  expires: %s  (%s)\n", strings.Repeat(" ", 14), until, timeLeft)
		fmt.Printf("  %s  scopes:  %s\n", strings.Repeat(" ", 14), strings.Join(claims.Scopes, ", "))
		fmt.Println()
	}

	if !anyFound {
		fmt.Println("  No passes stored locally.")
		fmt.Println()
		fmt.Println("  To redeem a claim code:  sibil redeem --code <SIB-SETUP-... or SIB-AUDIT-...>")
	}
	return nil
}
