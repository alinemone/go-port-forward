package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/alinemone/go-port-forward/internal/model"
)

type StorageData struct {
	Services map[string]string   `json:"services"`
	Groups   map[string][]string `json:"groups"`
	Legacy   map[string]string   `json:"-"`
}

type Storage struct {
	filePath string
}

func NewStorage() *Storage {
	newPath, ok := configStoragePath()
	if !ok {
		return &Storage{filePath: legacyStoragePath()}
	}

	if old := legacyStoragePath(); old != "" {
		migrateLegacyStorage(newPath, old)
	}

	return &Storage{filePath: newPath}
}

func configStoragePath() (string, bool) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	configDir := filepath.Join(homeDir, ".pf")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", false
	}
	return filepath.Join(configDir, "services.json"), true
}

func legacyStoragePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "services.json")
}

func migrateLegacyStorage(newPath, oldPath string) {
	if newPath == oldPath || oldPath == "" {
		return
	}
	if _, err := os.Stat(newPath); err == nil {
		return
	}
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return
	}
	_ = os.WriteFile(newPath, data, 0600)
}

func (s *Storage) Path() string {
	return s.filePath
}

func (s *Storage) SaveData(data *StorageData) error {
	return s.writeStorage(data)
}

func (s *Storage) EnsureExists() error {
	if s.filePath == "" {
		return nil
	}
	if _, err := os.Stat(s.filePath); err == nil {
		return nil
	}
	return s.writeStorage(&StorageData{
		Services: make(map[string]string),
		Groups:   make(map[string][]string),
	})
}

func (s *Storage) readStorage() (*StorageData, error) {
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return &StorageData{
			Services: make(map[string]string),
			Groups:   make(map[string][]string),
		}, nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, err
	}

	var storageData StorageData
	if err := json.Unmarshal(data, &storageData); err == nil && storageData.Services != nil {
		if storageData.Groups == nil {
			storageData.Groups = make(map[string][]string)
		}
		return &storageData, nil
	}

	var legacy map[string]string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}

	return &StorageData{
		Services: legacy,
		Groups:   make(map[string][]string),
	}, nil
}

func (s *Storage) writeStorage(data *StorageData) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.filePath)
	tmp, err := os.CreateTemp(dir, ".services-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(jsonData); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return renameWithRetry(tmpName, s.filePath)
}

func renameWithRetry(oldPath, newPath string) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if err = os.Rename(oldPath, newPath); err == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return err
}

func (s *Storage) LoadServices() (map[string]string, error) {
	data, err := s.readStorage()
	if err != nil {
		return nil, err
	}
	return data.Services, nil
}

func (s *Storage) saveServices(services map[string]string) error {
	data, err := s.readStorage()
	if err != nil {
		return err
	}
	data.Services = services
	return s.writeStorage(data)
}

func (s *Storage) AddService(name, command string) error {
	services, err := s.LoadServices()
	if err != nil {
		return err
	}
	services[name] = command
	return s.saveServices(services)
}

func (s *Storage) DeleteService(name string) error {
	data, err := s.readStorage()
	if err != nil {
		return err
	}

	if _, exists := data.Services[name]; !exists {
		return fmt.Errorf("service '%s' not found", name)
	}

	delete(data.Services, name)

	for groupName, members := range data.Groups {
		filtered := make([]string, 0, len(members))
		for _, m := range members {
			if m != name {
				filtered = append(filtered, m)
			}
		}
		data.Groups[groupName] = filtered
	}

	return s.writeStorage(data)
}

func (s *Storage) RenameService(oldName, newName string) error {
	if oldName == newName {
		return fmt.Errorf("new name is the same as the old name")
	}

	data, err := s.readStorage()
	if err != nil {
		return err
	}

	command, exists := data.Services[oldName]
	if !exists {
		return fmt.Errorf("service '%s' not found", oldName)
	}
	if _, exists := data.Services[newName]; exists {
		return fmt.Errorf("a service with name '%s' already exists", newName)
	}
	if _, exists := data.Groups[newName]; exists {
		return fmt.Errorf("a group with name '%s' already exists", newName)
	}

	delete(data.Services, oldName)
	data.Services[newName] = command

	for groupName, members := range data.Groups {
		for i, member := range members {
			if member == oldName {
				data.Groups[groupName][i] = newName
			}
		}
	}

	return s.writeStorage(data)
}

func (s *Storage) RenameGroup(oldName, newName string) error {
	if oldName == newName {
		return fmt.Errorf("new name is the same as the old name")
	}

	data, err := s.readStorage()
	if err != nil {
		return err
	}

	members, exists := data.Groups[oldName]
	if !exists {
		return fmt.Errorf("group '%s' not found", oldName)
	}
	if _, exists := data.Services[newName]; exists {
		return fmt.Errorf("a service with name '%s' already exists", newName)
	}
	if _, exists := data.Groups[newName]; exists {
		return fmt.Errorf("a group with name '%s' already exists", newName)
	}

	delete(data.Groups, oldName)
	data.Groups[newName] = members

	return s.writeStorage(data)
}

func (s *Storage) GetService(name string) (string, error) {
	services, err := s.LoadServices()
	if err != nil {
		return "", err
	}

	cmd, exists := services[name]
	if !exists {
		return "", fmt.Errorf("service '%s' not found", name)
	}

	return cmd, nil
}

var portRegex = regexp.MustCompile(`(\d+):(\d+)`)

func ParsePortsFromCommand(command string) (local, remote string) {
	matches := portRegex.FindStringSubmatch(command)
	if len(matches) == 3 {
		return matches[1], matches[2]
	}
	return "", ""
}

func (s *Storage) AddGroup(name string, services []string) error {
	data, err := s.readStorage()
	if err != nil {
		return err
	}

	if _, exists := data.Services[name]; exists {
		return fmt.Errorf("a service with name '%s' already exists, cannot create group with same name", name)
	}

	for _, svcName := range services {
		if _, exists := data.Services[svcName]; !exists {
			return fmt.Errorf("service '%s' not found", svcName)
		}
	}

	data.Groups[name] = services
	return s.writeStorage(data)
}

func (s *Storage) AddServicesToGroup(groupName string, services []string) error {
	data, err := s.readStorage()
	if err != nil {
		return err
	}

	members, exists := data.Groups[groupName]
	if !exists {
		return fmt.Errorf("group '%s' not found", groupName)
	}

	existing := make(map[string]bool, len(members))
	for _, m := range members {
		existing[m] = true
	}

	for _, svc := range services {
		if _, ok := data.Services[svc]; !ok {
			return fmt.Errorf("service '%s' not found", svc)
		}
		if !existing[svc] {
			members = append(members, svc)
			existing[svc] = true
		}
	}

	data.Groups[groupName] = members
	return s.writeStorage(data)
}

func (s *Storage) RemoveServicesFromGroup(groupName string, services []string) error {
	data, err := s.readStorage()
	if err != nil {
		return err
	}

	members, exists := data.Groups[groupName]
	if !exists {
		return fmt.Errorf("group '%s' not found", groupName)
	}

	toRemove := make(map[string]bool, len(services))
	for _, svc := range services {
		toRemove[svc] = true
	}

	filtered := make([]string, 0, len(members))
	for _, m := range members {
		if !toRemove[m] {
			filtered = append(filtered, m)
		}
	}

	data.Groups[groupName] = filtered
	return s.writeStorage(data)
}

func (s *Storage) DeleteGroup(name string) error {
	data, err := s.readStorage()
	if err != nil {
		return err
	}

	if _, exists := data.Groups[name]; !exists {
		return fmt.Errorf("group '%s' not found", name)
	}

	delete(data.Groups, name)
	return s.writeStorage(data)
}

func (s *Storage) GetGroupServices(name string) ([]string, error) {
	data, err := s.readStorage()
	if err != nil {
		return nil, err
	}

	services, exists := data.Groups[name]
	if !exists {
		return nil, fmt.Errorf("group '%s' not found", name)
	}

	return services, nil
}

func (s *Storage) ListGroups() (map[string][]string, error) {
	data, err := s.readStorage()
	if err != nil {
		return nil, err
	}
	return data.Groups, nil
}

func (s *Storage) ListServiceNames() ([]string, error) {
	data, err := s.readStorage()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(data.Services))
	for name := range data.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (s *Storage) HasNameConflict(name string) (bool, error) {
	data, err := s.readStorage()
	if err != nil {
		return false, err
	}

	_, isService := data.Services[name]
	_, isGroup := data.Groups[name]

	return isService && isGroup, nil
}

func (s *Storage) FindPortConflicts(serviceNames []string) ([]model.PortConflict, error) {
	data, err := s.readStorage()
	if err != nil {
		return nil, err
	}

	portMap := make(map[string][]string)
	for _, name := range serviceNames {
		command, exists := data.Services[name]
		if !exists {
			continue
		}

		localPort, _ := ParsePortsFromCommand(command)
		if localPort == "" {
			continue
		}

		portMap[localPort] = append(portMap[localPort], name)
	}

	conflicts := make([]model.PortConflict, 0)
	for port, services := range portMap {
		if len(services) > 1 {
			sort.Strings(services)
			conflicts = append(conflicts, model.PortConflict{
				Port:     port,
				Services: services,
			})
		}
	}

	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].Port < conflicts[j].Port
	})

	return conflicts, nil
}
