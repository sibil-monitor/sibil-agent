package server

import (
	"testing"

	"github.com/sibil-monitor/sibil-agent/collect"
)

func TestPrioritizeServicesKeepsDetectedInventoryVisible(t *testing.T) {
	services := []collect.Service{
		{Name: "web", Status: "online"},
		{Name: "db", Status: "online"},
		{Name: "worker", Status: "offline"},
	}

	ordered := prioritizeServices(services, "worker,db")
	if len(ordered) != len(services) {
		t.Fatalf("priority configuration hid detected services: got %d want %d", len(ordered), len(services))
	}
	if ordered[0].Name != "worker" || ordered[1].Name != "db" || ordered[2].Name != "web" {
		t.Fatalf("unexpected priority order: %#v", ordered)
	}
}

func TestPrioritizeServicesIgnoresUnknownAndDuplicateNames(t *testing.T) {
	services := []collect.Service{
		{Name: "web", Status: "online"},
		{Name: "db", Status: "online"},
	}

	ordered := prioritizeServices(services, "unknown,db,db")
	if len(ordered) != len(services) || ordered[0].Name != "db" || ordered[1].Name != "web" {
		t.Fatalf("unexpected priority handling: %#v", ordered)
	}
}
