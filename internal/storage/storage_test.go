package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestDeleteServiceRemovesFromGroups(t *testing.T) {
	s := newTestStorage(t)
	s.AddService("auth", "kubectl port-forward svc/auth 8081:80")
	s.AddService("core", "kubectl port-forward svc/core 8082:80")
	s.AddGroup("backend", []string{"auth", "core"})

	if err := s.DeleteService("auth"); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}

	members, err := s.GetGroupServices("backend")
	if err != nil {
		t.Fatalf("GetGroupServices: %v", err)
	}
	if len(members) != 1 || members[0] != "core" {
		t.Fatalf("expected group to be [core] after delete, got %v", members)
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
		command    string
		wantLocal  string
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

func TestAddServicesToGroup(t *testing.T) {
	s := newTestStorage(t)
	s.AddService("auth", "kubectl port-forward svc/auth 8081:80")
	s.AddService("core", "kubectl port-forward svc/core 8082:80")
	s.AddService("crm", "kubectl port-forward svc/crm 8083:80")
	s.AddGroup("backend", []string{"auth"})

	if err := s.AddServicesToGroup("backend", []string{"core", "crm"}); err != nil {
		t.Fatalf("AddServicesToGroup: %v", err)
	}

	members, _ := s.GetGroupServices("backend")
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %v", members)
	}
}

func TestAddServicesToGroupDedup(t *testing.T) {
	s := newTestStorage(t)
	s.AddService("auth", "kubectl port-forward svc/auth 8081:80")
	s.AddGroup("backend", []string{"auth"})

	if err := s.AddServicesToGroup("backend", []string{"auth"}); err != nil {
		t.Fatalf("AddServicesToGroup: %v", err)
	}

	members, _ := s.GetGroupServices("backend")
	if len(members) != 1 {
		t.Errorf("expected no duplicate, got %v", members)
	}
}

func TestAddServicesToGroupErrors(t *testing.T) {
	s := newTestStorage(t)
	s.AddService("auth", "kubectl port-forward svc/auth 8081:80")
	s.AddGroup("backend", []string{"auth"})

	if err := s.AddServicesToGroup("missing-group", []string{"auth"}); err == nil {
		t.Error("expected error for nonexistent group")
	}
	if err := s.AddServicesToGroup("backend", []string{"ghost"}); err == nil {
		t.Error("expected error for nonexistent service")
	}
}

func TestRemoveServicesFromGroup(t *testing.T) {
	s := newTestStorage(t)
	s.AddService("auth", "kubectl port-forward svc/auth 8081:80")
	s.AddService("core", "kubectl port-forward svc/core 8082:80")
	s.AddGroup("backend", []string{"auth", "core"})

	if err := s.RemoveServicesFromGroup("backend", []string{"auth"}); err != nil {
		t.Fatalf("RemoveServicesFromGroup: %v", err)
	}

	members, _ := s.GetGroupServices("backend")
	if len(members) != 1 || members[0] != "core" {
		t.Fatalf("expected [core], got %v", members)
	}

	// حذف عضو ناموجود → بدون خطا، بدون تغییر
	if err := s.RemoveServicesFromGroup("backend", []string{"not-a-member"}); err != nil {
		t.Fatalf("removing non-member should not error: %v", err)
	}

	if err := s.RemoveServicesFromGroup("missing", []string{"auth"}); err == nil {
		t.Error("expected error for nonexistent group")
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

func TestEnsureExistsCreatesFullSkeleton(t *testing.T) {
	s := newTestStorage(t)

	if err := s.EnsureExists(); err != nil {
		t.Fatalf("EnsureExists: %v", err)
	}

	raw, err := os.ReadFile(s.filePath)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// باید هر دو کلید services و groups را داشته باشد
	if !strings.Contains(string(raw), `"services"`) || !strings.Contains(string(raw), `"groups"`) {
		t.Errorf("skeleton must contain both keys, got: %s", raw)
	}

	var sd StorageData
	if err := json.Unmarshal(raw, &sd); err != nil {
		t.Fatalf("invalid JSON skeleton: %v", err)
	}
	if sd.Services == nil || sd.Groups == nil {
		t.Errorf("skeleton maps should be non-nil: %s", raw)
	}
}

func TestEnsureExistsDoesNotOverwrite(t *testing.T) {
	s := newTestStorage(t)

	if err := s.AddService("db", "kubectl port-forward svc/db 5432:5432"); err != nil {
		t.Fatal(err)
	}

	if err := s.EnsureExists(); err != nil {
		t.Fatalf("EnsureExists: %v", err)
	}

	if _, err := s.GetService("db"); err != nil {
		t.Fatalf("existing data lost after EnsureExists: %v", err)
	}
}

func TestMigrateLegacyStorage(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old", "services.json")
	newPath := filepath.Join(dir, "new", "services.json")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		t.Fatal(err)
	}

	content := []byte(`{"services":{"db":"kubectl port-forward svc/db 5432:5432"}}`)
	if err := os.WriteFile(oldPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	migrateLegacyStorage(newPath, oldPath)

	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("expected new file created: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("migrated content = %q, want %q", got, content)
	}
}

func TestMigrateLegacyStorageDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.json")
	newPath := filepath.Join(dir, "new.json")

	os.WriteFile(oldPath, []byte(`{"services":{"old":"x"}}`), 0644)
	existing := []byte(`{"services":{"keep":"y"}}`)
	os.WriteFile(newPath, existing, 0644)

	migrateLegacyStorage(newPath, oldPath)

	got, _ := os.ReadFile(newPath)
	if string(got) != string(existing) {
		t.Errorf("existing new file should not be overwritten, got %q", got)
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
