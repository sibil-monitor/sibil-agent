package cmd

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/sibil-monitor/sibil-agent/server"
	"github.com/spf13/cobra"
)

const redeemEndpoint = "https://monitor.cordee.ovh/api/v1/service-pass/redeem"
const passStorageDir = ".sibil-passes"

var redeemCmd = &cobra.Command{
	Use:   "redeem",
	Short: "Redeem a one-shot claim code (setup or audit)",
	Long: `sibil redeem exchanges a claim code from a paid offer against a
local service pass. The pass is stored in .sibil-passes/ and used
automatically by sibil setup-receipt and sibil audit.

Claim code formats:
  SIB-SETUP-XXXX-XXXX-XXXX   (VPS Health Setup, EUR 199)
  SIB-AUDIT-XXXX-XXXX-XXXX   (AI Stack Observability Audit, EUR 690)`,
	RunE: runRedeem,
}

func init() {
	redeemCmd.Flags().String("code", "", "claim code (prompted if omitted)")
}

func runRedeem(cmd *cobra.Command, args []string) error {
	code, _ := cmd.Flags().GetString("code")
	if code == "" {
		fmt.Print("Enter claim code: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			code = strings.TrimSpace(scanner.Text())
		}
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return fmt.Errorf("claim code required")
	}

	offerKey := ""
	switch {
	case strings.HasPrefix(code, "SIB-SETUP-"):
		offerKey = "setup_199"
	case strings.HasPrefix(code, "SIB-AUDIT-"):
		offerKey = "audit_690"
	default:
		return fmt.Errorf("unrecognised claim code format (expected SIB-SETUP-... or SIB-AUDIT-...)")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("no sibil config found — run: sibil init first")
	}

	agentID := tokenToAgentID(cfg.Token)
	fmt.Printf("Redeeming %s for this server (agent %s...)...\n", offerKey, agentID[:8])

	body, _ := json.Marshal(map[string]string{
		"claim_code":  code,
		"agent_id":    agentID,
		"cli_version": auditCLIVersion,
	})

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(redeemEndpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cannot reach monitor.cordee.ovh: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("invalid response from server")
	}

	if resp.StatusCode != 200 {
		errMsg, _ := result["error"].(string)
		return fmt.Errorf("redemption failed (%d): %s", resp.StatusCode, errMsg)
	}

	pass, _ := result["service_pass"].(string)
	activeUntil, _ := result["active_until"].(string)
	if pass == "" {
		return fmt.Errorf("server returned no service pass")
	}

	passPath, err := storePass(offerKey, pass)
	if err != nil {
		return fmt.Errorf("cannot store service pass: %w", err)
	}

	fmt.Printf("\n✓ Service pass activated\n")
	fmt.Printf("  Offer:        %s\n", offerKey)
	fmt.Printf("  Active until: %s\n", activeUntil)
	fmt.Printf("  Stored at:    %s\n", passPath)

	switch offerKey {
	case "setup_199":
		fmt.Println("\nNext: sibil setup-receipt")
	case "audit_690":
		fmt.Println("\nNext: sibil audit")
	}
	return nil
}

// storePass writes the service pass token to a local file.
func storePass(offerKey, token string) (string, error) {
	if err := os.MkdirAll(passStorageDir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(passStorageDir, offerKey+".pass")
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		return "", err
	}
	return path, nil
}

// LoadPass reads and verifies the stored service pass for the given offer.
// Returns an error if no pass exists, is expired, or bound to a different agent.
func LoadPass(offerKey, agentID string) (*server.ServicePassClaims, error) {
	path := filepath.Join(passStorageDir, offerKey+".pass")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no service pass found — run: sibil redeem")
	}
	token := strings.TrimSpace(string(data))
	claims, err := server.VerifyServicePass(token, agentID)
	if err != nil {
		return nil, fmt.Errorf("service pass invalid (%w) — run: sibil redeem", err)
	}
	return claims, nil
}

// tokenToAgentID replicates the backend's computeAgentIdFromToken:
// SHA-256(token) → full hex string (matches serverIdentity.js).
func tokenToAgentID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:])
}
