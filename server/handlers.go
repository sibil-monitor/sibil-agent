package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sibil-monitor/sibil-agent/collect"
	"github.com/sibil-monitor/sibil-agent/config"
)

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// GET /v1/status
// The optional services query orders configured priority services first; it does not
// hide the rest of the detected inventory. When entitlement is active, the ordered
// list is truncated to max_modules and served from cache within the plan interval.
var (
	statusMu   sync.Mutex
	statusBody []byte
	statusAt   time.Time
	statusKey  string
)

func handleStatus(detector string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := parseEntitlement(r.Header.Get("X-Entitlement"))
		cacheKey := ""
		preferredServices := r.URL.Query().Get("services")

		if claims != nil {
			maxModules := "unlimited"
			if claims.MaxModules != nil {
				maxModules = strconv.Itoa(*claims.MaxModules)
			}
			cacheKey = fmt.Sprintf("%s:%s:%t:%s", claims.Plan, maxModules, claims.IsGrace, preferredServices)
			minInterval := time.Duration(claims.RefreshIntervalS * float64(time.Second))
			statusMu.Lock()
			if statusBody != nil && statusKey == cacheKey && time.Since(statusAt) < minInterval {
				body := statusBody
				statusMu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Sibil-Cache", "true")
				if claims.IsGrace {
					w.Header().Set("X-Sibil-Entitlement-Status", "grace")
				}
				w.Write(body)
				return
			}
			statusMu.Unlock()
		}

		svcs := collect.Services(detector)
		svcs = prioritizeServices(svcs, preferredServices)
		if claims != nil && claims.MaxModules != nil && len(svcs) > *claims.MaxModules {
			svcs = svcs[:*claims.MaxModules]
		}

		body, _ := json.Marshal(map[string]any{"services": svcs})

		if claims != nil {
			statusMu.Lock()
			statusBody = body
			statusAt = time.Now()
			statusKey = cacheKey
			statusMu.Unlock()
			if claims.IsGrace {
				w.Header().Set("X-Sibil-Entitlement-Status", "grace")
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}
}

func prioritizeServices(svcs []collect.Service, preferredServices string) []collect.Service {
	if preferredServices == "" {
		return svcs
	}

	byName := make(map[string]collect.Service, len(svcs))
	for _, service := range svcs {
		byName[service.Name] = service
	}

	ordered := make([]collect.Service, 0, len(svcs))
	added := make(map[string]bool, len(svcs))
	for _, name := range strings.Split(preferredServices, ",") {
		name = strings.TrimSpace(name)
		service, ok := byName[name]
		if ok && !added[name] {
			ordered = append(ordered, service)
			added[name] = true
		}
	}
	for _, service := range svcs {
		if !added[service.Name] {
			ordered = append(ordered, service)
		}
	}
	return ordered
}

// GET /v1/system
func handleSystem() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := parseEntitlement(r.Header.Get("X-Entitlement"))
		if claims != nil && !claims.CanViewSystem {
			jsonError(w, "your plan does not include system metrics", http.StatusForbidden)
			return
		}
		sys, err := collect.System()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]any{"system": sys})
	}
}

// GET /v1/service/{name}/logs?lines=100
func handleLogs(detector string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := parseEntitlement(r.Header.Get("X-Entitlement"))
		if claims != nil && !claims.CanViewLogs {
			jsonError(w, "your plan does not include service logs", http.StatusForbidden)
			return
		}
		name := r.PathValue("name")
		lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
		if lines == 0 {
			lines = 100
		}
		logs, err := collect.ServiceLogs(name, lines, detector)
		if err != nil {
			jsonError(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonOK(w, map[string]string{"logs": collect.Redact(logs)})
	}
}

// POST /v1/service/{name}/restart|stop|start
// Triple lock: allow_actions (config) + can_control_services (entitlement) + arm session (ephemeral).
// All three must be satisfied. No CLI command or restart required after enabling via the app.
func handleControl(action, detector string, cfg *config.Config, arm *ArmSession) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.AllowActions {
			jsonError(w, "actions not enabled — enable via the app", http.StatusForbidden)
			return
		}
		claims := parseEntitlement(r.Header.Get("X-Entitlement"))
		if claims == nil || !claims.CanControlServices {
			jsonError(w, "service control requires a Pro plan", http.StatusForbidden)
			return
		}
		if !arm.IsArmed() {
			jsonError(w, "actions are not armed — arm them in the app first", http.StatusForbidden)
			return
		}
		name := r.PathValue("name")
		if err := collect.Control(action, name, detector); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}
