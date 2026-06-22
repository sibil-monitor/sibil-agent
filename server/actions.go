package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/sibil-monitor/sibil-agent/config"
)

// ArmSession holds the ephemeral arm state — in-memory only, intentionally not persisted.
// An arm session expires automatically after the configured duration.
type ArmSession struct {
	mu         sync.Mutex
	armed      bool
	armedUntil time.Time
	armedBy    string
}

func (s *ArmSession) IsArmed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.armed && time.Now().Before(s.armedUntil)
}

func (s *ArmSession) Arm(minutes int, by string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.armed = true
	s.armedUntil = time.Now().Add(time.Duration(minutes) * time.Minute)
	s.armedBy = by
	return s.armedUntil
}

func (s *ArmSession) Disarm() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.armed = false
	s.armedUntil = time.Time{}
	s.armedBy = ""
}

func (s *ArmSession) snapshot() (armed bool, until time.Time, by string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.armed && time.Now().Before(s.armedUntil) {
		return true, s.armedUntil, s.armedBy
	}
	return false, time.Time{}, ""
}

// GET /v1/actions/status
func handleActionsStatus(cfg *config.Config, arm *ArmSession) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := parseEntitlement(r.Header.Get("X-Entitlement"))
		canControl := claims != nil && claims.CanControlServices
		armed, until, by := arm.snapshot()

		resp := map[string]any{
			"allow_actions":        cfg.AllowActions,
			"can_control_services": canControl,
			"actions_armed":        armed,
			"armed_until":          nil,
			"armed_by":             nil,
			"minutes_remaining":    nil,
			"requires_plan":        !canControl,
		}
		if armed {
			resp["armed_until"] = until.UTC().Format(time.RFC3339)
			resp["armed_by"] = by
			remaining := int(time.Until(until).Minutes()) + 1
			resp["minutes_remaining"] = remaining
		}
		jsonOK(w, resp)
	}
}

// POST /v1/actions/enable
// Body: { "confirm": "ENABLE_SERVER_ACTIONS", "device_label": "..." }
// Enables allow_actions in the config file without requiring a restart.
func handleActionsEnable(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Confirm     string `json:"confirm"`
			DeviceLabel string `json:"device_label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Confirm != "ENABLE_SERVER_ACTIONS" {
			jsonError(w, `confirm field must be "ENABLE_SERVER_ACTIONS"`, http.StatusBadRequest)
			return
		}

		claims := parseEntitlement(r.Header.Get("X-Entitlement"))
		if claims == nil || !claims.CanControlServices {
			jsonError(w, "service control requires a compatible paid plan (Pro)", http.StatusForbidden)
			return
		}

		cfg.AllowActions = true
		if err := config.Save(cfg); err != nil {
			jsonError(w, "failed to save config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		jsonOK(w, map[string]any{
			"ok":            true,
			"allow_actions": true,
			"message":       "Actions enabled. Arm them in the app to execute commands.",
		})
	}
}

// POST /v1/actions/disable
func handleActionsDisable(cfg *config.Config, arm *ArmSession) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg.AllowActions = false
		arm.Disarm()
		if err := config.Save(cfg); err != nil {
			jsonError(w, "failed to save config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]any{"ok": true, "allow_actions": false})
	}
}

// POST /v1/actions/arm
// Body: { "duration_minutes": 15, "confirm": "ARM_ACTIONS_TEMPORARILY", "device_label": "..." }
// Creates a short-lived arm session. Actions are only executable while armed.
func handleActionsArm(cfg *config.Config, arm *ArmSession) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.AllowActions {
			jsonError(w, "actions are not enabled — enable them first in the app", http.StatusForbidden)
			return
		}

		claims := parseEntitlement(r.Header.Get("X-Entitlement"))
		if claims == nil || !claims.CanControlServices {
			jsonError(w, "service control requires a compatible paid plan (Pro)", http.StatusForbidden)
			return
		}

		var body struct {
			DurationMinutes int    `json:"duration_minutes"`
			Confirm         string `json:"confirm"`
			DeviceLabel     string `json:"device_label"`
		}
		body.DurationMinutes = 15
		json.NewDecoder(r.Body).Decode(&body) //nolint
		if body.Confirm != "ARM_ACTIONS_TEMPORARILY" {
			jsonError(w, `confirm field must be "ARM_ACTIONS_TEMPORARILY"`, http.StatusBadRequest)
			return
		}
		if body.DurationMinutes <= 0 || body.DurationMinutes > 60 {
			body.DurationMinutes = 15
		}

		armedUntil := arm.Arm(body.DurationMinutes, body.DeviceLabel)
		jsonOK(w, map[string]any{
			"ok":               true,
			"actions_armed":    true,
			"armed_until":      armedUntil.UTC().Format(time.RFC3339),
			"duration_minutes": body.DurationMinutes,
		})
	}
}

// POST /v1/actions/disarm
func handleActionsDisarm(arm *ArmSession) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		arm.Disarm()
		jsonOK(w, map[string]any{"ok": true, "actions_armed": false})
	}
}
