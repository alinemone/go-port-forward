package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

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

// Load loads all services from disk
func (s *Storage) Load() (map[string]string, error) {
	services := make(map[string]string)

	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return services, nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, &services); err != nil {
		return nil, err
	}

	return services, nil
}

// Save saves all services to disk
func (s *Storage) Save(services map[string]string) error {
	data, err := json.MarshalIndent(services, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
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
