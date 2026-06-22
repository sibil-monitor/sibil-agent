package collect

import (
	"strings"
)

type DockerResult struct {
	Status     string            `json:"status"` // detected | not_detected | permission_denied | unavailable
	Containers []DockerContainer `json:"containers,omitempty"`
}

type DockerContainer struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"` // running | exited | paused | ...
	Ports  string `json:"ports,omitempty"`
	Health string `json:"health"` // healthy | unhealthy | starting | none
}

type dockerPsItem struct {
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	State  string `json:"State"`
	Ports  string `json:"Ports"`
	Status string `json:"Status"`
}

func DockerInventory() DockerResult {
	out, err := execCommand("docker", "ps", "-a", "--format", "{{json .}}")
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "permission denied") || strings.Contains(msg, "Got permission denied") {
			return DockerResult{Status: "permission_denied"}
		}
		if strings.Contains(msg, "not found") || strings.Contains(msg, "no such file") || strings.Contains(msg, "executable file not found") {
			return DockerResult{Status: "not_detected"}
		}
		return DockerResult{Status: "unavailable"}
	}

	lines := splitLines(string(out))
	containers := make([]DockerContainer, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var item dockerPsItem
		if err := jsonUnmarshal([]byte(line), &item); err != nil {
			continue
		}
		health := "none"
		status := item.Status
		if strings.Contains(status, "(healthy)") {
			health = "healthy"
		} else if strings.Contains(status, "(unhealthy)") {
			health = "unhealthy"
		} else if strings.Contains(status, "(health: starting)") {
			health = "starting"
		}
		name := strings.TrimPrefix(item.Names, "/")
		containers = append(containers, DockerContainer{
			Name:   name,
			Image:  item.Image,
			State:  item.State,
			Ports:  item.Ports,
			Health: health,
		})
	}
	return DockerResult{Status: "detected", Containers: containers}
}
