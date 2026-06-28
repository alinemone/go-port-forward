package main

import (
	"fmt"
	"sort"
	"testing"
)

type fakeRunTargetStore struct {
	services map[string]string
	groups   map[string][]string
}

func (f *fakeRunTargetStore) ListServiceNames() ([]string, error) {
	names := make([]string, 0, len(f.services))
	for name := range f.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (f *fakeRunTargetStore) HasNameConflict(name string) (bool, error) {
	_, isService := f.services[name]
	_, isGroup := f.groups[name]
	return isService && isGroup, nil
}

func (f *fakeRunTargetStore) GetService(name string) (string, error) {
	command, exists := f.services[name]
	if !exists {
		return "", fmt.Errorf("service '%s' not found", name)
	}
	return command, nil
}

func (f *fakeRunTargetStore) GetGroupServices(name string) ([]string, error) {
	services, exists := f.groups[name]
	if !exists {
		return nil, fmt.Errorf("group '%s' not found", name)
	}
	return services, nil
}

func TestLooksLikeRunTarget(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"db": "cmd", "redis": "cmd"},
		groups:   map[string][]string{"backend": {"db"}},
	}

	cases := []struct {
		input string
		want  bool
	}{
		{"db", true},          // a service
		{"backend", true},     // a group
		{"db,redis", true},    // comma list, first is a service
		{"redis api", true},   // space list, first is a service
		{"nope", false},       // unknown → not a run target
		{"", false},           // empty
		{"unknown,db", false}, // first token unknown → treat as unknown command
	}
	for _, c := range cases {
		if got := looksLikeRunTarget(st, c.input); got != c.want {
			t.Errorf("looksLikeRunTarget(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestResolveRunTargetsSingleService(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"db": "cmd"},
		groups:   map[string][]string{},
	}

	got, err := resolveRunTargets(st, "db")
	if err != nil {
		t.Fatalf("resolveRunTargets: %v", err)
	}
	if len(got) != 1 || got[0] != "db" {
		t.Fatalf("got %v, want [db]", got)
	}
}

func TestResolveRunTargetsSingleGroup(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"db": "cmd", "redis": "cmd"},
		groups:   map[string][]string{"backend": {"db", "redis"}},
	}

	got, err := resolveRunTargets(st, "backend")
	if err != nil {
		t.Fatalf("resolveRunTargets: %v", err)
	}
	if len(got) != 2 || got[0] != "db" || got[1] != "redis" {
		t.Fatalf("got %v, want [db redis]", got)
	}
}

func TestResolveRunTargetsMultipleGroups(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"db": "cmd", "redis": "cmd", "api": "cmd"},
		groups: map[string][]string{
			"backend": {"db", "api"},
			"cache":   {"redis"},
		},
	}

	got, err := resolveRunTargets(st, "backend,cache")
	if err != nil {
		t.Fatalf("resolveRunTargets: %v", err)
	}
	if len(got) != 3 || got[0] != "db" || got[1] != "api" || got[2] != "redis" {
		t.Fatalf("got %v, want [db api redis]", got)
	}
}

func TestResolveRunTargetsMixedGroupAndService(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"db": "cmd", "redis": "cmd", "api": "cmd"},
		groups: map[string][]string{
			"backend": {"db", "api"},
		},
	}

	got, err := resolveRunTargets(st, "backend,redis")
	if err != nil {
		t.Fatalf("resolveRunTargets: %v", err)
	}
	if len(got) != 3 || got[0] != "db" || got[1] != "api" || got[2] != "redis" {
		t.Fatalf("got %v, want [db api redis]", got)
	}
}

func TestResolveRunTargetsDeduplicates(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"db": "cmd", "api": "cmd"},
		groups:   map[string][]string{"backend": {"db", "api"}},
	}

	got, err := resolveRunTargets(st, "backend,db")
	if err != nil {
		t.Fatalf("resolveRunTargets: %v", err)
	}
	if len(got) != 2 || got[0] != "db" || got[1] != "api" {
		t.Fatalf("got %v, want [db api]", got)
	}
}

func TestResolveRunTargetsAll(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"db": "cmd", "api": "cmd"},
		groups:   map[string][]string{},
	}

	got, err := resolveRunTargets(st, "all")
	if err != nil {
		t.Fatalf("resolveRunTargets: %v", err)
	}
	if len(got) != 2 || got[0] != "api" || got[1] != "db" {
		t.Fatalf("got %v, want [api db]", got)
	}
}

func TestResolveRunTargetsAllIgnoresGroups(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"db": "cmd", "api": "cmd"},
		groups: map[string][]string{
			"backend": {"db", "api"},
			"cache":   {"db"},
		},
	}

	got, err := resolveRunTargets(st, "all")
	if err != nil {
		t.Fatalf("resolveRunTargets: %v", err)
	}
	if len(got) != 2 || got[0] != "api" || got[1] != "db" {
		t.Fatalf("got %v, want [api db]", got)
	}
}

func TestResolveRunTargetsSkipsEmptyTargets(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"db": "cmd", "api": "cmd"},
		groups:   map[string][]string{},
	}

	got, err := resolveRunTargets(st, "db,,api")
	if err != nil {
		t.Fatalf("resolveRunTargets: %v", err)
	}
	if len(got) != 2 || got[0] != "db" || got[1] != "api" {
		t.Fatalf("got %v, want [db api]", got)
	}
}

func TestResolveRunTargetsTrimsSpaces(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"auth": "cmd", "core": "cmd"},
		groups:   map[string][]string{},
	}

	for _, input := range []string{"auth, core", "auth ,core", " auth , core ", "auth core", "auth,core"} {
		got, err := resolveRunTargets(st, input)
		if err != nil {
			t.Fatalf("resolveRunTargets(%q): %v", input, err)
		}
		if len(got) != 2 || got[0] != "auth" || got[1] != "core" {
			t.Fatalf("resolveRunTargets(%q) = %v, want [auth core]", input, got)
		}
	}
}

func TestResolveRunTargetsNotFound(t *testing.T) {
	st := &fakeRunTargetStore{
		services: map[string]string{"db": "cmd"},
		groups:   map[string][]string{},
	}

	_, err := resolveRunTargets(st, "unknown")
	if err == nil {
		t.Fatal("expected error")
	}
}
