package configedit

import "testing"

func TestValidateValid(t *testing.T) {
	data := []byte(`{
		"services": {"db": "kubectl port-forward svc/db 5432:5432"},
		"groups": {"backend": ["db"]}
	}`)

	sd, err := Validate(data)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(sd.Services) != 1 || len(sd.Groups) != 1 {
		t.Fatalf("got %d services, %d groups", len(sd.Services), len(sd.Groups))
	}
}

func TestValidateAcceptsIconConfig(t *testing.T) {
	data := []byte(`{
		"icon": {"enable": true},
		"services": {"db": "kubectl port-forward svc/db 5432:5432"},
		"groups": {"backend": ["db"]}
	}`)

	sd, err := Validate(data)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if sd.Icon == nil || !sd.Icon.Enable {
		t.Fatalf("expected enabled icon config, got %#v", sd.Icon)
	}
}

func TestValidateEmptyIsOK(t *testing.T) {
	sd, err := Validate([]byte(`{}`))
	if err != nil {
		t.Fatalf("Validate empty: %v", err)
	}
	if sd.Services == nil || sd.Groups == nil {
		t.Fatal("expected non-nil maps after validation")
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]string{
		"bad json":            `{not json`,
		"invalid name":        `{"services": {"bad name": "kubectl port-forward svc/x 1:2"}}`,
		"dangerous command":   `{"services": {"x": "rm -rf /"}}`,
		"unknown group ref":   `{"services": {}, "groups": {"g": ["ghost"]}}`,
		"group/service clash": `{"services": {"db": "kubectl port-forward svc/db 5432:5432"}, "groups": {"db": ["db"]}}`,
		"invalid icon type":   `{"icon": {"enable": "yes"}, "services": {}}`,
	}

	for name, payload := range cases {
		if _, err := Validate([]byte(payload)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
