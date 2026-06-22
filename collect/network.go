package collect

import (
	"regexp"
	"strconv"
	"strings"
)

type NetworkListener struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Process  string `json:"process,omitempty"`
	Exposure string `json:"exposure"`  // public_interface | local_only | private_interface | unknown
	RiskHint string `json:"risk_hint"` // public_listener | internal_listener | ""
}

var ssProcessRe = regexp.MustCompile(`"([^"]+)"`)

func NetworkListeners() ([]NetworkListener, error) {
	out, err := execCommand("ss", "-tlnp")
	if err != nil {
		return nil, err
	}
	return parseSSOutput(string(out)), nil
}

func parseSSOutput(output string) []NetworkListener {
	lines := splitLines(output)
	var result []NetworkListener
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "Netid") || strings.HasPrefix(line, "State") {
			continue
		}
		fields := splitFields(line)
		// ss -tlnp (TCP-filtered) columns: State Recv-Q Send-Q Local:Port Peer:Port [Process]
		// ss without -t adds Netid as field[0], shifting everything by 1.
		// We detect which layout by checking if field[0] is a known state keyword.
		localIdx := 3
		if fields[0] != "LISTEN" && fields[0] != "ESTAB" && fields[0] != "CLOSE-WAIT" {
			// Netid is present: Netid State Recv-Q Send-Q Local Peer [Process]
			localIdx = 4
		}
		if len(fields) <= localIdx {
			continue
		}
		localAddr := fields[localIdx]
		lastColon := strings.LastIndex(localAddr, ":")
		if lastColon < 0 {
			continue
		}
		rawAddr := localAddr[:lastColon]
		portStr := localAddr[lastColon+1:]

		port, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}

		// Strip IPv6 brackets
		addr := strings.TrimPrefix(rawAddr, "[")
		addr = strings.TrimSuffix(addr, "]")

		process := ""
		if len(fields) >= 6 {
			last := fields[len(fields)-1]
			if m := ssProcessRe.FindStringSubmatch(last); len(m) >= 2 {
				process = m[1]
			}
		}

		exposure, riskHint := classifyExposure(addr)

		result = append(result, NetworkListener{
			Protocol: "tcp",
			Address:  addr,
			Port:     port,
			Process:  process,
			Exposure: exposure,
			RiskHint: riskHint,
		})
	}
	return result
}

func classifyExposure(addr string) (exposure, riskHint string) {
	switch addr {
	case "0.0.0.0", "::", "*":
		return "public_interface", "public_listener"
	case "127.0.0.1", "::1":
		return "local_only", "internal_listener"
	default:
		return "private_interface", ""
	}
}
