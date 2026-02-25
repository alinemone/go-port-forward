package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	dir := t.TempDir()
	return &Storage{
		filePath: filepath.Join(dir, "services.json"),
	}
}

func TestNewStorageCreatesInstance(t *testing.T) {
	s := NewStorage()
	if s == nil {
		t.Fatal("NewStorage returned nil")
	}
	if s.filePath == "" {
		t.Fatal("filePath is empty")
	}
}

func TestAddAndGetService(t *testing.T) {
	s := newTestStorage(t)

	err := s.AddService("db", "kubectl port-forward svc/db 5432:5432")
	if err != nil {
		t.Fatalf("AddService: %v", err)
	}

	cmd, err := s.GetService("db")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if cmd != "kubectl port-forward svc/db 5432:5432" {
		t.Errorf("got %q", cmd)
	}
}

func TestGetServiceNotFound(t *testing.T) {
	s := newTestStorage(t)

	_, err := s.GetService("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent service")
	}
}

func TestDeleteService(t *testing.T) {
	s := newTestStorage(t)

	s.AddService("db", "kubectl port-forward svc/db 5432:5432")
	err := s.DeleteService("db")
	if err != nil {
		t.Fatalf("DeleteService: %v", err)
	}

	_, err = s.GetService("db")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteServiceNotFound(t *testing.T) {
	s := newTestStorage(t)

	err := s.DeleteService("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent service")
	}
}

func TestLoadServicesEmpty(t *testing.T) {
	s := newTestStorage(t)

	services, err := s.LoadServices()
	if err != nil {
		t.Fatalf("LoadServices: %v", err)
	}
	if len(services) != 0 {
		t.Errorf("expected empty, got %d", len(services))
	}
}

func TestListServiceNames(t *testing.T) {
	s := newTestStorage(t)

	s.AddService("beta", "cmd-beta")
	s.AddService("alpha", "cmd-alpha")

	names, err := s.ListServiceNames()
	if err != nil {
		t.Fatalf("ListServiceNames: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2, got %d", len(names))
	}
	// Should be sorted
	if names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("names = %v", names)
	}
}

func TestParsePortsFromCommand(t *testing.T) {
	tests := []struct {
		command   string
		wantLocal string
		wantRemote string
	}{
		{"kubectl port-forward svc/db 5432:5432", "5432", "5432"},
		{"kubectl port-forward svc/redis 6379:6379", "6379", "6379"},
		{"kubectl port-forward svc/web 8080:80", "8080", "80"},
		{"no ports here", "", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			local, remote := ParsePortsFromCommand(tt.command)
			if local != tt.wantLocal {
				t.Errorf("local = %q, want %q", local, tt.wantLocal)
			}
			if remote != tt.wantRemote {
				t.Errorf("remote = %q, want %q", remote, tt.wantRemote)
			}
		})
	}
}

func TestGroupOperations(t *testing.T) {
	s := newTestStorage(t)

	// Add services first
	s.AddService("auth", "kubectl port-forward svc/auth 8081:80")
	s.AddService("core", "kubectl port-forward svc/core 8082:80")

	// Add group
	err := s.AddGroup("backend", []string{"auth", "core"})
	if err != nil {
		t.Fatalf("AddGroup: %v", err)
	}

	// List groups
	groups, err := s.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	// Get group services
	services, err := s.GetGroupServices("backend")
	if err != nil {
		t.Fatalf("GetGroupServices: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}

	// Delete group
	err = s.DeleteGroup("backend")
	if err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}

	_, err = s.GetGroupServices("backend")
	if err == nil {
		t.Fatal("expected error after group delete")
	}
}

func TestAddGroupWithNonexistentService(t *testing.T) {
	s := newTestStorage(t)

	err := s.AddGroup("bad-group", []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent service in group")
	}
}

func TestAddGroupConflictsWithServiceName(t *testing.T) {
	s := newTestStorage(t)

	s.AddService("myname", "kubectl port-forward svc/x 8080:80")
	err := s.AddGroup("myname", []string{"myname"})
	if err == nil {
		t.Fatal("expected error for group name conflicting with service name")
	}
}

func TestHasNameConflict(t *testing.T) {
	s := newTestStorage(t)

	s.AddService("svc1", "kubectl port-forward svc/x 8080:80")

	conflict, err := s.HasNameConflict("svc1")
	if err != nil {
		t.Fatalf("HasNameConflict: %v", err)
	}
	// Only a service, not a group — no conflict
	if conflict {
		t.Error("expected no conflict when only service exists")
	}
}

func TestFindPortConflicts(t *testing.T) {
	s := newTestStorage(t)

	s.AddService("svc-a", "kubectl port-forward svc/a 8080:80")
	s.AddService("svc-b", "kubectl port-forward svc/b 8080:80")
	s.AddService("svc-c", "kubectl port-forward svc/c 9090:90")

	conflicts, err := s.FindPortConflicts([]string{"svc-a", "svc-b", "svc-c"})
	if err != nil {
		t.Fatalf("FindPortConflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Port != "8080" {
		t.Errorf("conflict port = %q", conflicts[0].Port)
	}
	if len(conflicts[0].Services) != 2 {
		t.Errorf("conflict services = %v", conflicts[0].Services)
	}
}

func TestFindPortConflictsNoConflict(t *testing.T) {
	s := newTestStorage(t)

	s.AddService("svc-a", "kubectl port-forward svc/a 8080:80")
	s.AddService("svc-b", "kubectl port-forward svc/b 9090:90")

	conflicts, err := s.FindPortConflicts([]string{"svc-a", "svc-b"})
	if err != nil {
		t.Fatalf("FindPortConflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}
}

func TestLegacyFormatMigration(t *testing.T) {
	s := newTestStorage(t)

	// Write legacy format (flat map)
	legacy := map[string]string{
		"db":    "kubectl port-forward svc/db 5432:5432",
		"redis": "kubectl port-forward svc/redis 6379:6379",
	}
	data, _ := json.Marshal(legacy)
	os.WriteFile(s.filePath, data, 0644)

	// Should read legacy format correctly
	services, err := s.LoadServices()
	if err != nil {
		t.Fatalf("LoadServices: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
}
