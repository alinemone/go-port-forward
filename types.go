package main

import "sync"

const DataFile = "services.json"

type ServiceStatus struct {
	Name   string
	Status string
	Local  string
	Remote string
}

var (
	mu       sync.Mutex
	statuses = make(map[string]ServiceStatus)
)
