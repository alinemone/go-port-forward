package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// StorageData represents the complete storage structure
type StorageData struct {
	Services map[string]string   `json:"services,omitempty"`
	Groups   map[string][]string `json:"groups,omitempty"`
	Legacy   map[string]string   `json:"-"` // For backward compatibility
}

// Storage manages service persistence
type Storage struct {
	filePath string
}

// NewStorage creates a new storage instance
func NewStorage() *Storage {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	return &Storage{
		filePath: filepath.Join(exeDir, "services.json"),
	}
}

// LoadData loads complete storage data from disk
func (s *Storage) LoadData() (*StorageData, error) {
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

	// Try new format first
	var storageData StorageData
	if err := json.Unmarshal(data, &storageData); err == nil && storageData.Services != nil {
		if storageData.Groups == nil {
			storageData.Groups = make(map[string][]string)
		}
		return &storageData, nil
	}

	// Fallback to legacy format (backward compatibility)
	var legacy map[string]string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}

	return &StorageData{
		Services: legacy,
		Groups:   make(map[string][]string),
	}, nil
}

// SaveData saves complete storage data to disk
func (s *Storage) SaveData(data *StorageData) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, jsonData, 0644)
}

// Load loads all services from disk (backward compatibility)
func (s *Storage) Load() (map[string]string, error) {
	data, err := s.LoadData()
	if err != nil {
		return nil, err
	}
	return data.Services, nil
}

// Save saves all services to disk (backward compatibility)
func (s *Storage) Save(services map[string]string) error {
	data, err := s.LoadData()
	if err != nil {
		return err
	}
	data.Services = services
	return s.SaveData(data)
}

// Add adds a new service
func (s *Storage) Add(name, command string) error {
	services, err := s.Load()
	if err != nil {
		return err
	}
	services[name] = command
	return s.Save(services)
}

// Delete deletes a service
func (s *Storage) Delete(name string) error {
	services, err := s.Load()
	if err != nil {
		return err
	}

	if _, exists := services[name]; !exists {
		return fmt.Errorf("service '%s' not found", name)
	}

	delete(services, name)
	return s.Save(services)
}

// Get retrieves a single service
func (s *Storage) Get(name string) (string, error) {
	services, err := s.Load()
	if err != nil {
		return "", err
	}

	cmd, exists := services[name]
	if !exists {
		return "", fmt.Errorf("service '%s' not found", name)
	}

	return cmd, nil
}

// ExtractPorts extracts local and remote ports from command
func ExtractPorts(command string) (local, remote string) {
	re := regexp.MustCompile(`(\d+):(\d+)`)
	matches := re.FindStringSubmatch(command)
	if len(matches) == 3 {
		return matches[1], matches[2]
	}
	return "", ""
}

// AddGroup adds a new group
func (s *Storage) AddGroup(name string, services []string) error {
	data, err := s.LoadData()
	if err != nil {
		return err
	}

	// Check for conflicts
	if _, exists := data.Services[name]; exists {
		return fmt.Errorf("a service with name '%s' already exists, cannot create group with same name", name)
	}

	// Validate all services exist
	for _, svcName := range services {
		if _, exists := data.Services[svcName]; !exists {
			return fmt.Errorf("service '%s' not found", svcName)
		}
	}

	data.Groups[name] = services
	return s.SaveData(data)
}

// DeleteGroup deletes a group
func (s *Storage) DeleteGroup(name string) error {
	data, err := s.LoadData()
	if err != nil {
		return err
	}

	if _, exists := data.Groups[name]; !exists {
		return fmt.Errorf("group '%s' not found", name)
	}

	delete(data.Groups, name)
	return s.SaveData(data)
}

// GetGroup retrieves a group's services
func (s *Storage) GetGroup(name string) ([]string, error) {
	data, err := s.LoadData()
	if err != nil {
		return nil, err
	}

	services, exists := data.Groups[name]
	if !exists {
		return nil, fmt.Errorf("group '%s' not found", name)
	}

	return services, nil
}

// ListGroups returns all groups sorted by name
func (s *Storage) ListGroups() (map[string][]string, error) {
	data, err := s.LoadData()
	if err != nil {
		return nil, err
	}
	return data.Groups, nil
}

// GetAllServiceNames returns all service names (for "all" command)
func (s *Storage) GetAllServiceNames() ([]string, error) {
	data, err := s.LoadData()
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

// CheckNameConflict checks if a name exists as both service and group
func (s *Storage) CheckNameConflict(name string) (hasConflict bool, err error) {
	data, err := s.LoadData()
	if err != nil {
		return false, err
	}

	_, isService := data.Services[name]
	_, isGroup := data.Groups[name]

	return isService && isGroup, nil
}
