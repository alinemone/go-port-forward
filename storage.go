package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// ساختار کامل داده‌های ذخیره‌سازی
type StorageData struct {
	Services map[string]string   `json:"services,omitempty"`
	Groups   map[string][]string `json:"groups,omitempty"`
	Legacy   map[string]string   `json:"-"`
}

// مدیریت ذخیره‌سازی سرویس‌ها و گروه‌ها
type Storage struct {
	filePath string
}

// ساخت نمونه ذخیره‌سازی بر اساس مسیر فایل اجرایی
func NewStorage() *Storage {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	return &Storage{
		filePath: filepath.Join(exeDir, "services.json"),
	}
}

// خواندن کامل داده‌ها از دیسک
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

// ذخیره کامل داده‌ها روی دیسک
func (s *Storage) writeStorage(data *StorageData) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, jsonData, 0644)
}

// بارگذاری سرویس‌ها (سازگار با نسخه‌های قدیمی)
func (s *Storage) LoadServices() (map[string]string, error) {
	data, err := s.readStorage()
	if err != nil {
		return nil, err
	}
	return data.Services, nil
}

// ذخیره سرویس‌ها (سازگار با نسخه‌های قدیمی)
func (s *Storage) saveServices(services map[string]string) error {
	data, err := s.readStorage()
	if err != nil {
		return err
	}
	data.Services = services
	return s.writeStorage(data)
}

// افزودن سرویس جدید
func (s *Storage) AddService(name, command string) error {
	services, err := s.LoadServices()
	if err != nil {
		return err
	}
	services[name] = command
	return s.saveServices(services)
}

// حذف سرویس از ذخیره‌سازی
func (s *Storage) DeleteService(name string) error {
	services, err := s.LoadServices()
	if err != nil {
		return err
	}

	if _, exists := services[name]; !exists {
		return fmt.Errorf("service '%s' not found", name)
	}

	delete(services, name)
	return s.saveServices(services)
}

// دریافت فرمان یک سرویس
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

// استخراج پورت‌ها از فرمان اجرا
func parsePortsFromCommand(command string) (local, remote string) {
	matches := portRegex.FindStringSubmatch(command)
	if len(matches) == 3 {
		return matches[1], matches[2]
	}
	return "", ""
}

// افزودن گروه جدید
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

// حذف گروه
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

// دریافت سرویس‌های یک گروه
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

// دریافت لیست گروه‌ها
func (s *Storage) ListGroups() (map[string][]string, error) {
	data, err := s.readStorage()
	if err != nil {
		return nil, err
	}
	return data.Groups, nil
}

// دریافت نام تمام سرویس‌ها برای حالت all
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

// بررسی تداخل نام بین سرویس و گروه
func (s *Storage) HasNameConflict(name string) (bool, error) {
	data, err := s.readStorage()
	if err != nil {
		return false, err
	}

	_, isService := data.Services[name]
	_, isGroup := data.Groups[name]

	return isService && isGroup, nil
}

// ساختار گزارش تداخل پورت
type PortConflict struct {
	Port     string
	Services []string
}

// بررسی تداخل پورت بین سرویس‌ها
func (s *Storage) FindPortConflicts(serviceNames []string) ([]PortConflict, error) {
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

		localPort, _ := parsePortsFromCommand(command)
		if localPort == "" {
			continue
		}

		portMap[localPort] = append(portMap[localPort], name)
	}

	conflicts := make([]PortConflict, 0)
	for port, services := range portMap {
		if len(services) > 1 {
			sort.Strings(services)
			conflicts = append(conflicts, PortConflict{
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
