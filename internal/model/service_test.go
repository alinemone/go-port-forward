package model

import (
	"testing"
	"time"
)

func TestServiceStatusConstants(t *testing.T) {
	if StatusConnecting != "connecting" {
		t.Errorf("StatusConnecting = %q", StatusConnecting)
	}
	if StatusHealthy != "healthy" {
		t.Errorf("StatusHealthy = %q", StatusHealthy)
	}
	if StatusError != "error" {
		t.Errorf("StatusError = %q", StatusError)
	}
}

func TestServiceStruct(t *testing.T) {
	svc := Service{
		Name:         "test-svc",
		Command:      "kubectl port-forward svc/test 8080:80",
		LocalPort:    "8080",
		Status:       StatusConnecting,
		StartTime:    time.Now(),
		RestartCount: 0,
		Logs:         []LogEntry{},
	}

	if svc.Name != "test-svc" {
		t.Errorf("Name = %q", svc.Name)
	}
	if svc.LocalPort != "8080" {
		t.Errorf("LocalPort = %q", svc.LocalPort)
	}
}

func TestPortConflictStruct(t *testing.T) {
	conflict := PortConflict{
		Port:     "8080",
		Services: []string{"svc-a", "svc-b"},
	}

	if conflict.Port != "8080" {
		t.Errorf("Port = %q", conflict.Port)
	}
	if len(conflict.Services) != 2 {
		t.Errorf("Services len = %d", len(conflict.Services))
	}
}
