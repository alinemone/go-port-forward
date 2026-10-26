package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// getter fetches one API path (e.g. "api/v1/namespaces/x/pods/y") as JSON into
// out. resolveTarget needs only a handful of reads, so pf does them with plain
// HTTP rather than client-go's typed clientsets, whose scheme registration
// links every Kubernetes API group into the binary (≈20 MB).
type getter func(ctx context.Context, path string, query url.Values, out any) error

type apiClient struct {
	http *http.Client
	base *url.URL
}

func newAPIClient(cfg *rest.Config) (*apiClient, error) {
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, err
	}
	base, _, err := rest.DefaultServerUrlFor(cfg)
	if err != nil {
		return nil, err
	}
	return &apiClient{http: httpClient, base: base}, nil
}

func (c *apiClient) url(path string) *url.URL {
	return c.base.JoinPath(path)
}

func (c *apiClient) get(ctx context.Context, path string, query url.Values, out any) error {
	u := c.url(path)
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		// The API server explains failures in a Status object, e.g.
		// `services "db" not found` or a forbidden message.
		var status metav1.Status
		if json.Unmarshal(body, &status) == nil && status.Message != "" {
			return errors.New(status.Message)
		}
		return fmt.Errorf("GET %s: %s", u.Path, resp.Status)
	}
	return json.Unmarshal(body, out)
}
