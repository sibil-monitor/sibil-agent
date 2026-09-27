package server

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/sibil-monitor/sibil-agent/config"
)

const cliVersion = "1.4.2"

func Run(cfg *config.Config) error {
	mux := http.NewServeMux()
	token := cfg.Token
	det := cfg.Detector
	arm := &ArmSession{}

	// Health (no auth) — exposes version, allow_actions state and supported entitlement kids.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		kids := make([]string, 0, len(entitlementPublicKeys))
		for k := range entitlementPublicKeys {
			kids = append(kids, k)
		}
		sort.Strings(kids)
		jsonOK(w, map[string]any{
			"status":                     "ok",
			"service":                    "sibil-cli",
			"version":                    cliVersion,
			"allow_actions":              cfg.AllowActions,
			"actions_armed":              arm.IsArmed(),
			"supported_entitlement_kids": kids,
		})
	})

	// Protected read routes
	mux.HandleFunc("GET /v1/status", bearerAuth(token, handleStatus(det)))
	mux.HandleFunc("GET /v1/system", bearerAuth(token, handleSystem()))
	mux.HandleFunc("GET /v1/service/{name}/logs", bearerAuth(token, handleLogs(det)))

	// Actions management — enable/disable/arm via the app (no restart required)
	mux.HandleFunc("GET /v1/actions/status", bearerAuth(token, handleActionsStatus(cfg, arm)))
	mux.HandleFunc("POST /v1/actions/enable", bearerAuth(token, handleActionsEnable(cfg)))
	mux.HandleFunc("POST /v1/actions/disable", bearerAuth(token, handleActionsDisable(cfg, arm)))
	mux.HandleFunc("POST /v1/actions/arm", bearerAuth(token, handleActionsArm(cfg, arm)))
	mux.HandleFunc("POST /v1/actions/disarm", bearerAuth(token, handleActionsDisarm(arm)))

	// Protected action routes — triple lock: allow_actions + entitlement + arm session
	mux.HandleFunc("POST /v1/service/{name}/restart", bearerAuth(token, handleControl("restart", det, cfg, arm)))
	mux.HandleFunc("POST /v1/service/{name}/stop", bearerAuth(token, handleControl("stop", det, cfg, arm)))
	mux.HandleFunc("POST /v1/service/{name}/start", bearerAuth(token, handleControl("start", det, cfg, arm)))

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	actionsState := "disabled"
	if cfg.AllowActions {
		actionsState = "enabled (arm in app to execute)"
	}
	fmt.Printf("[sibil] Listening on %s  (detector: %s  actions: %s)\n", addr, det, actionsState)
	return http.ListenAndServe(addr, mux)
}
