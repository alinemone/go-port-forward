package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func getDataFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return DataFile
	}
	exeDir := filepath.Dir(exe)
	return filepath.Join(exeDir, DataFile)
}

func LoadServices() map[string]string {
	services := make(map[string]string)
	dataFile := getDataFilePath()
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		return services
	}
	data, _ := os.ReadFile(dataFile)
	_ = json.Unmarshal(data, &services)

	return services
}

func SaveServices(services map[string]string) {
	data, _ := json.MarshalIndent(services, "", "  ")
	dataFile := getDataFilePath()
	_ = os.WriteFile(dataFile, data, 0644)
}
