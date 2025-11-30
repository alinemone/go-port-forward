// Package storage provides persistence for service definitions.
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const DataFile = "services.json"

// HealthCheckType defines how to check service health.
type HealthCheckType string

const (
	// HealthCheckAuto automatically detects TCP or HTTP
	HealthCheckAuto HealthCheckType = "auto"
	// HealthCheckTCP uses TCP connection test
	HealthCheckTCP HealthCheckType = "tcp"
	// HealthCheckHTTP uses HTTP request
	HealthCheckHTTP HealthCheckType = "http"
)

// ServiceDefinition represents a service with all its metadata.
type ServiceDefinition struct {
	Command     string          `json:"command"`
	HealthCheck HealthCheckType `json:"health_check,omitempty"`
	HealthPath  string          `json:"health_path,omitempty"`
	Description string          `json:"description,omitempty"`
}

// Storage manages service persistence.
type Storage struct {
	filePath string
}

// New creates a new storage instance.
func New() *Storage {
	return &Storage{
		filePath: getDataFilePath(),
	}
}

// LoadServices loads all services from storage.
// It handles both old format (string) and new format (object).
func (s *Storage) LoadServices() (map[string]*ServiceDefinition, error) {
	services := make(map[string]*ServiceDefinition)

	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return services, nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read services file: %w", err)
	}

	// Try to unmarshal as new format first
	var newFormat map[string]*ServiceDefinition
	if err := json.Unmarshal(data, &newFormat); err == nil {
		// Check if it's actually new format (has objects, not strings)
		if len(newFormat) > 0 {
			for _, svc := range newFormat {
				if svc != nil && svc.Command != "" {
					// It's new format
					services = newFormat
					return services, nil
				}
				// If we get here, might be old format, continue to try
				break
			}
		}
	}

	// Try old format (map[string]string)
	var oldFormat map[string]string
	if err := json.Unmarshal(data, &oldFormat); err != nil {
		return nil, fmt.Errorf("failed to parse services file: %w", err)
	}

	// Migrate from old format to new format
	for name, command := range oldFormat {
		services[name] = &ServiceDefinition{
			Command:     command,
			HealthCheck: HealthCheckAuto,
		}
	}

	return services, nil
}

// SaveServices saves all services to storage.
func (s *Storage) SaveServices(services map[string]*ServiceDefinition) error {
	data, err := json.MarshalIndent(services, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal services: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write services file: %w", err)
	}

	return nil
}

// AddService adds a new service.
func (s *Storage) AddService(name, command string) error {
	services, err := s.LoadServices()
	if err != nil {
		return err
	}

	services[name] = &ServiceDefinition{
		Command:     command,
		HealthCheck: HealthCheckAuto,
	}

	return s.SaveServices(services)
}

// DeleteService deletes a service.
func (s *Storage) DeleteService(name string) error {
	services, err := s.LoadServices()
	if err != nil {
		return err
	}

	if _, exists := services[name]; !exists {
		return fmt.Errorf("service %q not found", name)
	}

	delete(services, name)
	return s.SaveServices(services)
}

// GetService retrieves a single service.
func (s *Storage) GetService(name string) (*ServiceDefinition, error) {
	services, err := s.LoadServices()
	if err != nil {
		return nil, err
	}

	svc, exists := services[name]
	if !exists {
		return nil, fmt.Errorf("service %q not found", name)
	}

	return svc, nil
}

// ExtractPorts extracts local and remote ports from a command string.
func ExtractPorts(command string) (local, remote string, ok bool) {
	portRegex := regexp.MustCompile(`(\d+):(\d+)`)
	matches := portRegex.FindStringSubmatch(command)
	if len(matches) == 3 {
		return matches[1], matches[2], true
	}
	return "", "", false
}

func getDataFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return DataFile
	}
	exeDir := filepath.Dir(exe)
	return filepath.Join(exeDir, DataFile)
}
