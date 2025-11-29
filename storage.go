package main

import (
	"encoding/json"
	"os"
)

func LoadServices() map[string]string {
	services := make(map[string]string)
	if _, err := os.Stat(DataFile); os.IsNotExist(err) {
		return services
	}
	data, _ := os.ReadFile(DataFile)
	json.Unmarshal(data, &services)
	return services
}

func SaveServices(services map[string]string) {
	data, _ := json.MarshalIndent(services, "", "  ")
	os.WriteFile(DataFile, data, 0644)
}
