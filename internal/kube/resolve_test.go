package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func testPod(name string, ready bool, restarts int32, age time.Duration, phase corev1.PodPhase) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "prod", Labels: map[string]string{"app": "db"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:  "db",
			Ports: []corev1.ContainerPort{{Name: "pg", ContainerPort: 5432}, {Name: "metrics", ContainerPort: 9187}},
		}}},
		Status: corev1.PodStatus{
			Phase:             phase,
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
			ContainerStatuses: []corev1.ContainerStatus{{RestartCount: restarts}},
		},
	}
}

func dbService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "prod"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "db"},
			Ports: []corev1.ServicePort{
				{Name: "postgres", Port: 5432, TargetPort: intstr.FromString("pg")},
				{Name: "proxy", Port: 6432, TargetPort: intstr.FromInt32(5432)},
				{Name: "exporter", Port: 9187},
			},
		},
	}
}

func TestResolveServicePicksBestPodAndTranslatesPorts(t *testing.T) {
	cs := fakeAPI(
		dbService(),
		testPod("pending", true, 0, time.Hour*5, corev1.PodPending),
		testPod("not-ready", false, 0, time.Hour*4, corev1.PodRunning),
		testPod("restarty", true, 3, time.Hour*3, corev1.PodRunning),
		testPod("young", true, 0, time.Hour, corev1.PodRunning),
		testPod("old", true, 0, time.Hour*2, corev1.PodRunning),
	)

	spec, err := ParseCommand("kubectl -n prod port-forward svc/db 1150:5432 1151:6432 1152:9187 1153:proxy")
	if err != nil {
		t.Fatal(err)
	}
	pod, ports, err := resolveTarget(context.Background(), cs, "prod", spec)
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if pod.Name != "old" {
		t.Errorf("picked pod %q, want the oldest ready pod without restarts", pod.Name)
	}
	want := []string{"1150:5432", "1151:5432", "1152:9187", "1153:5432"}
	if !reflect.DeepEqual(ports, want) {
		t.Errorf("ports = %v, want %v", ports, want)
	}
}

func TestResolveServiceErrors(t *testing.T) {
	noSelector := dbService()
	noSelector.Name = "external"
	noSelector.Spec.Selector = nil

	cs := fakeAPI(dbService(), noSelector, testPod("p", true, 0, time.Hour, corev1.PodRunning))
	tests := map[string]string{
		"kubectl port-forward svc/db 1:1234":      "does not have a service port 1234",
		"kubectl port-forward svc/external 1:80":  "no pod selector",
		"kubectl port-forward svc/missing 1:80":   "not found",
		"kubectl port-forward pod/p 1:nope":       "no container port named",
		"kubectl port-forward deploy/missing 1:2": "not found",
	}
	for command, wantErr := range tests {
		spec, err := ParseCommand(command)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = resolveTarget(context.Background(), cs, "prod", spec)
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("%s: error = %v, want it to mention %q", command, err, wantErr)
		}
	}
}

func TestResolveDeploymentUsesSelectorAndNamedPorts(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "prod"},
		Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"db"}},
			},
		}},
	}
	other := testPod("other-app", true, 0, time.Hour*9, corev1.PodRunning)
	other.Labels = map[string]string{"app": "other"}
	cs := fakeAPI(deploy, other, testPod("db-1", true, 0, time.Hour, corev1.PodRunning))

	spec, _ := ParseCommand("kubectl port-forward deploy/db 9000:metrics 6000:6000")
	pod, ports, err := resolveTarget(context.Background(), cs, "prod", spec)
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if pod.Name != "db-1" {
		t.Errorf("picked %q, want db-1", pod.Name)
	}
	if want := []string{"9000:9187", "6000:6000"}; !reflect.DeepEqual(ports, want) {
		t.Errorf("ports = %v, want %v", ports, want)
	}
}

func TestResolveNoRunningPod(t *testing.T) {
	cs := fakeAPI(dbService(), testPod("p", true, 0, time.Hour, corev1.PodPending))
	spec, _ := ParseCommand("kubectl port-forward svc/db 1:5432")
	if _, _, err := resolveTarget(context.Background(), cs, "prod", spec); err == nil ||
		!strings.Contains(err.Error(), "no running pod found for service/db") {
		t.Errorf("error = %v", err)
	}

	spec, _ = ParseCommand("kubectl port-forward pod/p 1:5432")
	if _, _, err := resolveTarget(context.Background(), cs, "prod", spec); err == nil ||
		!strings.Contains(err.Error(), "not running") {
		t.Errorf("error = %v", err)
	}
}

// fakeAPI serves objects the way the API server would for the paths
// resolveTarget reads, including server-side label-selector filtering of pod
// lists. Objects round-trip through JSON, exercising the real decoding.
func fakeAPI(objs ...any) getter {
	store := map[string]any{}
	var pods []corev1.Pod
	for _, o := range objs {
		switch o := o.(type) {
		case *corev1.Pod:
			store["api/v1/namespaces/"+o.Namespace+"/pods/"+o.Name] = o
			pods = append(pods, *o)
		case *corev1.Service:
			store["api/v1/namespaces/"+o.Namespace+"/services/"+o.Name] = o
		case *appsv1.Deployment:
			store["apis/apps/v1/namespaces/"+o.Namespace+"/deployments/"+o.Name] = o
		}
	}

	return func(_ context.Context, path string, query url.Values, out any) error {
		obj, ok := store[path]
		if path == "api/v1/namespaces/prod/pods" {
			sel, err := labels.Parse(query.Get("labelSelector"))
			if err != nil {
				return err
			}
			list := &corev1.PodList{}
			for _, p := range pods {
				if sel.Matches(labels.Set(p.Labels)) {
					list.Items = append(list.Items, p)
				}
			}
			obj, ok = list, true
		}
		if !ok {
			return fmt.Errorf("%s not found", path)
		}
		data, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, out)
	}
}
