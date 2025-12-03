package cert

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Manager manages certificate configuration
type Manager struct {
	configPath string
	config     *P12Config // Single global certificate
	mu         sync.RWMutex
}

// CertStorageConfig represents the JSON structure for certificate storage
type CertStorageConfig struct {
	P12Path  string `json:"p12_path"`
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
}

// NewManager creates a new certificate manager
func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".pf")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "certificate.json")

	manager := &Manager{
		configPath: configPath,
		config:     nil,
	}

	// Load existing config
	if err := manager.load(); err != nil {
		// If file doesn't exist, it's ok (first run)
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return manager, nil
}

// AddCertificate adds a global certificate
func (m *Manager) AddCertificate(p12Path, password string) error {
	// Extract P12
	config, err := ExtractP12(p12Path, password)
	if err != nil {
		return fmt.Errorf("failed to extract P12: %w", err)
	}

	m.mu.Lock()
	m.config = config
	m.mu.Unlock()

	// Save to disk
	return m.save()
}

// GetCertificate returns the global certificate config
func (m *Manager) GetCertificate() (*P12Config, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config == nil {
		return nil, false
	}

	return m.config, true
}

// RemoveCertificate removes the global certificate
func (m *Manager) RemoveCertificate() error {
	m.mu.Lock()
	exists := m.config != nil
	m.config = nil
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("no certificate configured")
	}

	return m.save()
}

// save persists certificate config to disk
func (m *Manager) save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// If no config, write empty file (or delete it)
	if m.config == nil {
		// Delete the file if it exists
		os.Remove(m.configPath)
		return nil
	}

	// Convert to storage format
	storage := &CertStorageConfig{
		P12Path:  m.config.P12Path,
		CertPath: m.config.CertPath,
		KeyPath:  m.config.KeyPath,
	}

	data, err := json.MarshalIndent(storage, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal certificate config: %w", err)
	}

	return os.WriteFile(m.configPath, data, 0600)
}

// load reads certificate config from disk
func (m *Manager) load() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return err
	}

	var storage CertStorageConfig
	if err := json.Unmarshal(data, &storage); err != nil {
		return fmt.Errorf("failed to unmarshal certificate config: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = &P12Config{
		P12Path:      storage.P12Path,
		CertPath:     storage.CertPath,
		KeyPath:      storage.KeyPath,
		extractedDir: filepath.Dir(storage.CertPath),
	}

	return nil
}
