package kube

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    Spec
	}{
		{
			name:    "namespace before verb",
			command: "kubectl -n prod-accounting port-forward svc/prod-accounting-psql 1152:5432",
			want: Spec{Namespace: "prod-accounting", Kind: "service", Name: "prod-accounting-psql",
				Ports: []PortMapping{{Local: 1152, Remote: "5432"}}},
		},
		{
			name:    "flags after ports and double spaces",
			command: "kubectl port-forward  deploy/redis-haproxy 6399:6379 -n prod-api",
			want: Spec{Namespace: "prod-api", Kind: "deployment", Name: "redis-haproxy",
				Ports: []PortMapping{{Local: 6399, Remote: "6379"}}},
		},
		{
			name:    "address and context with equals",
			command: "kubectl --context=stage port-forward service/ch --address 0.0.0.0 9001:9000 --namespace=ads",
			want: Spec{Context: "stage", Namespace: "ads", Kind: "service", Name: "ch",
				Addresses: []string{"0.0.0.0"}, Ports: []PortMapping{{Local: 9001, Remote: "9000"}}},
		},
		{
			name:    "bare pod name, single port, multiple ports",
			command: "kubectl port-forward mongo-0 27017 8080:http",
			want: Spec{Kind: "pod", Name: "mongo-0",
				Ports: []PortMapping{{Local: 27017, Remote: "27017"}, {Local: 8080, Remote: "http"}}},
		},
		{
			name:    "quoted windows paths and group-qualified kind",
			command: `C:\tools\kubectl.exe --kubeconfig "C:\Users\ali mohammadi\.kube\config" --client-certificate="C:\a b\c.pem" --client-key=C:\k.pem -nprod port-forward deployment.apps/api 8080:80`,
			want: Spec{Kubeconfig: `C:\Users\ali mohammadi\.kube\config`, ClientCert: `C:\a b\c.pem`, ClientKey: `C:\k.pem`,
				Namespace: "prod", Kind: "deployment", Name: "api", Ports: []PortMapping{{Local: 8080, Remote: "80"}}},
		},
		{
			name:    "ignored flags",
			command: "kubectl port-forward --pod-running-timeout=2m sts/kafka 9092:9092 --insecure-skip-tls-verify",
			want: Spec{Kind: "statefulset", Name: "kafka", InsecureSkipTLSVerify: true,
				Ports: []PortMapping{{Local: 9092, Remote: "9092"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCommand(tt.command)
			if err != nil {
				t.Fatalf("ParseCommand: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestParseCommandRejects(t *testing.T) {
	tests := map[string]string{
		"":                                        "empty",
		"ssh -L 8080:localhost:80 u@h":            "only kubectl",
		"kubectl get pods":                        "only kubectl port-forward",
		"kubectl port-forward svc/db":             "needs a target",
		"kubectl port-forward svc/db :5432":       "random local port",
		"kubectl port-forward svc/db 0:5432":      "invalid local port",
		"kubectl port-forward svc/db 1:99999":     "invalid remote port",
		"kubectl port-forward job/x 1:2":          "unsupported port-forward target type",
		"kubectl port-forward svc/ 1:2":           "has no name",
		"kubectl --weird port-forward svc/db 1:2": "unsupported flag",
		"kubectl -n":                              "needs a value",
		`kubectl port-forward "svc/db 1:2`:        "unterminated quote",
	}
	for command, wantErr := range tests {
		_, err := ParseCommand(command)
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("ParseCommand(%q) error = %v, want it to mention %q", command, err, wantErr)
		}
	}
}
