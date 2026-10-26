package kube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

func TestAPIClientGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/k8s/clusters/c-1/api/v1/namespaces/prod/pods":
			if got := r.URL.Query().Get("labelSelector"); got != "app=db" {
				t.Errorf("labelSelector = %q", got)
			}
			w.Write([]byte(`{"items":[{"metadata":{"name":"db-0"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"kind":"Status","message":"services \"db\" not found","code":404}`))
		}
	}))
	defer srv.Close()

	// A Rancher-style server URL with a path prefix must be preserved.
	api, err := newAPIClient(&rest.Config{Host: srv.URL + "/k8s/clusters/c-1", BearerToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	var list corev1.PodList
	if err := api.get(context.Background(), "api/v1/namespaces/prod/pods", map[string][]string{"labelSelector": {"app=db"}}, &list); err != nil {
		t.Fatalf("get pods: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "db-0" {
		t.Errorf("list = %+v", list.Items)
	}

	var svc corev1.Service
	err = api.get(context.Background(), "api/v1/namespaces/prod/services/db", nil, &svc)
	if err == nil || err.Error() != `services "db" not found` {
		t.Errorf("error = %v, want the API server's Status message", err)
	}

	if u := api.url("api/v1/namespaces/prod/pods/db-0/portforward"); !strings.HasSuffix(u.Path, "/k8s/clusters/c-1/api/v1/namespaces/prod/pods/db-0/portforward") {
		t.Errorf("portforward URL = %s", u)
	}
}
