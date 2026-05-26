package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/alinemone/go-port-forward/internal/model"
)

// ساختار کامل داده‌های ذخیره‌سازی
// بدون omitempty تا فایل همیشه ساختار کامل (services + groups) داشته باشد
type StorageData struct {
	Services map[string]string   `json:"services"`
	Groups   map[string][]string `json:"groups"`
	Legacy   map[string]string   `json:"-"`
}

// مدیریت ذخیره‌سازی سرویس‌ها و گروه‌ها
type Storage struct {
	filePath string
}

// ساخت نمونه ذخیره‌سازی در ~/.pf/services.json با مهاجرت خودکار از مسیر قدیمی
func NewStorage() *Storage {
	newPath, ok := configStoragePath()
	if !ok {
		// fallback: رفتار قدیمی (کنار فایل اجرایی)
		return &Storage{filePath: legacyStoragePath()}
	}

	if old := legacyStoragePath(); old != "" {
		migrateLegacyStorage(newPath, old)
	}

	return &Storage{filePath: newPath}
}

// مسیر canonical جدید: ~/.pf/services.json
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

// مسیر قدیمی کنار فایل اجرایی
func legacyStoragePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "services.json")
}

// مهاجرت یک‌باره‌ی فایل قدیمی به مسیر جدید (فقط وقتی مسیر جدید هنوز ساخته نشده)
func migrateLegacyStorage(newPath, oldPath string) {
	if newPath == oldPath || oldPath == "" {
		return
	}
	if _, err := os.Stat(newPath); err == nil {
		return // مسیر جدید از قبل وجود دارد
	}
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return // فایل قدیمی وجود ندارد یا قابل خواندن نیست
	}
	_ = os.WriteFile(newPath, data, 0644)
}

// Path مسیر فایل ذخیره‌سازی فعلی را برمی‌گرداند
func (s *Storage) Path() string {
	return s.filePath
}

// SaveData کل داده‌ها (سرویس‌ها و گروه‌ها) را روی دیسک ذخیره می‌کند
func (s *Storage) SaveData(data *StorageData) error {
	return s.writeStorage(data)
}

// EnsureExists اگر فایل کانفیگ وجود نداشته باشد، یک اسکلت کامل (services + groups خالی) می‌سازد.
// برای بار اول نصب: کاربر تازه با اجرای pf یک فایل با ساختار کامل دریافت می‌کند.
func (s *Storage) EnsureExists() error {
	if s.filePath == "" {
		return nil
	}
	if _, err := os.Stat(s.filePath); err == nil {
		return nil // از قبل وجود دارد
	}
	return s.writeStorage(&StorageData{
		Services: make(map[string]string),
		Groups:   make(map[string][]string),
	})
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

// حذف سرویس از ذخیره‌سازی (و پاک‌سازی عضویت آن در همه‌ی گروه‌ها)
func (s *Storage) DeleteService(name string) error {
	data, err := s.readStorage()
	if err != nil {
		return err
	}

	if _, exists := data.Services[name]; !exists {
		return fmt.Errorf("service '%s' not found", name)
	}

	delete(data.Services, name)

	// حذف سرویس از عضویت همه‌ی گروه‌ها تا مرجع معلق نماند
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

// RenameService تغییر نام سرویس و به‌روزرسانی عضویت آن در گروه‌ها
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

	// به‌روزرسانی عضویت سرویس در همه‌ی گروه‌ها
	for groupName, members := range data.Groups {
		for i, member := range members {
			if member == oldName {
				data.Groups[groupName][i] = newName
			}
		}
	}

	return s.writeStorage(data)
}

// RenameGroup تغییر نام یک گروه
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

// ParsePortsFromCommand استخراج پورت‌ها از فرمان اجرا
func ParsePortsFromCommand(command string) (local, remote string) {
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

// AddServicesToGroup سرویس‌ها را به یک گروه موجود اضافه می‌کند (با حذف تکراری‌ها)
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

// RemoveServicesFromGroup سرویس‌ها را از یک گروه موجود حذف می‌کند
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

// بررسی تداخل پورت بین سرویس‌ها
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
