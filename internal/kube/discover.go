package kube

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
)

// Discovery: list the services of a namespace and recognize the ones worth
// forwarding — databases, caches, brokers — so they can be added in one go.

// Tier orders discovered services by how likely they are what you want.
type Tier int

const (
	// TierSuggested is a data store's main entry point (e.g. the Postgres
	// proxy, the Redis haproxy, the Kafka bootstrap). Pre-selected.
	TierSuggested Tier = iota
	// TierMore is a data store reached another way: a replica, a per-pod
	// "announce" service, a headless service.
	TierMore
	// TierOther is everything else (apps, exporters, UIs).
	TierOther
)

// Candidate is one service of the namespace that can be forwarded.
type Candidate struct {
	Service string
	Kind    string // human label: "postgres", "redis", … or "" when unknown
	Short   string // name suffix for a saved service: "pg", "redis", …
	Port    int    // the remote port to forward (the data port, not metrics)
	Ports   []int  // every port the service exposes
	Tier    Tier
	Note    string // why it isn't suggested: "replica", "per-pod", "headless"
}

type storeKind struct {
	label, short string
}

// dataPorts maps well-known data ports to their store. Metrics/exporter ports
// are deliberately absent so they never win as "the" port to forward.
var dataPorts = map[int]storeKind{
	5432:  {"postgres", "pg"},
	6432:  {"pgbouncer", "pgb"},
	3306:  {"mysql", "mysql"},
	27017: {"mongodb", "mongo"},
	6379:  {"redis", "redis"},
	9092:  {"kafka", "kafka"},
	8123:  {"clickhouse", "ch"},
	9200:  {"elasticsearch", "es"},
	5672:  {"rabbitmq", "rabbit"},
	11211: {"memcached", "memcached"},
	9042:  {"cassandra", "cassandra"},
	7687:  {"neo4j", "neo4j"},
	2181:  {"zookeeper", "zk"},
	8086:  {"influxdb", "influx"},
	9000:  {"clickhouse", "ch"}, // only counted when the name says clickhouse
}

var secondaryName = regexp.MustCompile(`(?:-announce-\d+|-slave|-replica|-read(?:er|only)?|-headless|-backup|-keeper|-sentinel|-brokers|-nodes)$`)

// classify recognizes a service's store and how it should be offered.
func classify(svc *corev1.Service) Candidate {
	c := Candidate{Service: svc.Name, Tier: TierOther}
	for _, p := range svc.Spec.Ports {
		c.Ports = append(c.Ports, int(p.Port))
	}
	sort.Ints(c.Ports)

	for _, p := range svc.Spec.Ports {
		kind, ok := dataPorts[int(p.Port)]
		if !ok || (p.Port == 9000 && !strings.Contains(svc.Name, "clickhouse")) {
			continue
		}
		c.Kind, c.Short, c.Port = kind.label, kind.short, int(p.Port)
		break
	}
	if c.Port == 0 && len(c.Ports) > 0 {
		c.Port = c.Ports[0]
	}
	// A pooler in front of a store shares its port; name it for what it is.
	for _, pooler := range []string{"pgbouncer", "pgpool"} {
		if c.Kind == "postgres" && strings.Contains(svc.Name, pooler) {
			c.Kind, c.Short = pooler, pooler
		}
	}
	if c.Kind == "" {
		return c
	}

	headless := svc.Spec.ClusterIP == corev1.ClusterIPNone
	switch m := secondaryName.FindString(svc.Name); {
	case strings.HasPrefix(m, "-announce"):
		c.Tier, c.Note = TierMore, "per-pod"
	case m == "-slave" || m == "-replica" || strings.HasPrefix(m, "-read"):
		c.Tier, c.Note = TierMore, "replica"
	case headless || m != "":
		c.Tier, c.Note = TierMore, "headless"
		if m != "" && !headless {
			c.Note = strings.TrimPrefix(m, "-")
		}
	case c.Kind == "zookeeper":
		c.Tier = TierMore
	default:
		c.Tier = TierSuggested
	}
	return c
}

// DiscoverServices lists the forwardable services of namespace, best first.
// kubeContext "" means the kubeconfig's current context.
func DiscoverServices(ctx context.Context, kubeContext, namespace string, opts Options) ([]Candidate, error) {
	api, _, err := discoveryClient(kubeContext, opts)
	if err != nil {
		return nil, err
	}
	var list corev1.ServiceList
	if err := api.get(ctx, "api/v1/namespaces/"+namespace+"/services", nil, &list); err != nil {
		return nil, err
	}

	out := make([]Candidate, 0, len(list.Items))
	for i := range list.Items {
		svc := &list.Items[i]
		if len(svc.Spec.Selector) == 0 || len(svc.Spec.Ports) == 0 {
			continue // no pods behind it: nothing to forward to
		}
		out = append(out, classify(svc))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Tier != out[j].Tier {
			return out[i].Tier < out[j].Tier
		}
		return out[i].Service < out[j].Service
	})
	return out, nil
}

// ListNamespaces returns the namespace names visible to the context, for
// autocomplete. Many users may not list namespaces; callers treat an error as
// "no suggestions".
func ListNamespaces(ctx context.Context, kubeContext string, opts Options) ([]string, error) {
	api, _, err := discoveryClient(kubeContext, opts)
	if err != nil {
		return nil, err
	}
	var list corev1.NamespaceList
	if err := api.get(ctx, "api/v1/namespaces", nil, &list); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list.Items))
	for _, ns := range list.Items {
		names = append(names, ns.Name)
	}
	sort.Strings(names)
	return names, nil
}

func discoveryClient(kubeContext string, opts Options) (*apiClient, string, error) {
	cfg, ns, err := restConfig(Spec{Context: kubeContext}, opts)
	if err != nil {
		return nil, "", err
	}
	api, err := newAPIClient(cfg)
	return api, ns, err
}

// Contexts lists the kubeconfig's contexts and the current one.
func Contexts() (names []string, current string, err error) {
	raw, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return nil, "", err
	}
	for name := range raw.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, raw.CurrentContext, nil
}

// BuildCommand writes the kubectl port-forward command for a discovered
// service, in the same shape as hand-written ones. --context is pinned only
// when it differs from the current context, so a saved command keeps
// following the current context like the rest.
func BuildCommand(kubeContext, currentContext, namespace, service string, local, remote int) string {
	ctxFlag := ""
	if kubeContext != "" && kubeContext != currentContext {
		ctxFlag = " --context=" + kubeContext
	}
	return fmt.Sprintf("kubectl%s -n %s port-forward svc/%s %d:%d", ctxFlag, namespace, service, local, remote)
}
