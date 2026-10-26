package kube

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// workloadPaths maps the workload kinds pf forwards to their apps/v1 resource.
var workloadPaths = map[string]string{
	"deployment":  "deployments",
	"statefulset": "statefulsets",
	"replicaset":  "replicasets",
	"daemonset":   "daemonsets",
}

// resolveTarget picks the pod a port-forward target lands on and translates the
// requested remote ports into that pod's container ports, mirroring kubectl:
// a Service port maps through its targetPort, a named port is looked up on the
// pod's containers. It returns the resolved local → container port pairs.
func resolveTarget(ctx context.Context, get getter, ns string, spec Spec) (*corev1.Pod, []forwardedPort, error) {
	switch spec.Kind {
	case "pod":
		var pod corev1.Pod
		if err := get(ctx, "api/v1/namespaces/"+ns+"/pods/"+spec.Name, nil, &pod); err != nil {
			return nil, nil, err
		}
		if pod.Status.Phase != corev1.PodRunning {
			return nil, nil, fmt.Errorf("pod %s is not running (phase %s)", pod.Name, pod.Status.Phase)
		}
		ports, err := containerPorts(&pod, spec.Ports)
		return &pod, ports, err

	case "service":
		var svc corev1.Service
		if err := get(ctx, "api/v1/namespaces/"+ns+"/services/"+spec.Name, nil, &svc); err != nil {
			return nil, nil, err
		}
		if len(svc.Spec.Selector) == 0 {
			return nil, nil, fmt.Errorf("service %s has no pod selector, so there is no pod to forward to", svc.Name)
		}
		pod, err := pickPod(ctx, get, ns, labels.SelectorFromSet(svc.Spec.Selector), spec.Target())
		if err != nil {
			return nil, nil, err
		}
		ports, err := servicePorts(&svc, pod, spec.Ports)
		return pod, ports, err

	default:
		selector, err := workloadSelector(ctx, get, ns, spec.Kind, spec.Name)
		if err != nil {
			return nil, nil, err
		}
		pod, err := pickPod(ctx, get, ns, selector, spec.Target())
		if err != nil {
			return nil, nil, err
		}
		ports, err := containerPorts(pod, spec.Ports)
		return pod, ports, err
	}
}

func workloadSelector(ctx context.Context, get getter, ns, kind, name string) (labels.Selector, error) {
	resource, ok := workloadPaths[kind]
	if !ok {
		return nil, fmt.Errorf("unsupported target kind %q", kind)
	}
	// Every apps/v1 workload keeps its pod selector at spec.selector.
	var obj struct {
		Spec struct {
			Selector *metav1.LabelSelector `json:"selector"`
		} `json:"spec"`
	}
	if err := get(ctx, "apis/apps/v1/namespaces/"+ns+"/"+resource+"/"+name, nil, &obj); err != nil {
		return nil, err
	}

	sel := obj.Spec.Selector
	if sel == nil {
		return nil, fmt.Errorf("%s/%s has no pod selector", kind, name)
	}
	selector, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return nil, err
	}
	if selector.Empty() {
		return nil, fmt.Errorf("%s/%s has an empty pod selector", kind, name)
	}
	return selector, nil
}

// pickPod returns the most suitable running pod behind a selector: ready pods
// first, then the fewest restarts, then the oldest — the same preference order
// kubectl uses, so pf lands on the pod kubectl would.
func pickPod(ctx context.Context, get getter, ns string, selector labels.Selector, target string) (*corev1.Pod, error) {
	var list corev1.PodList
	query := url.Values{"labelSelector": {selector.String()}}
	if err := get(ctx, "api/v1/namespaces/"+ns+"/pods", query, &list); err != nil {
		return nil, err
	}

	candidates := make([]*corev1.Pod, 0, len(list.Items))
	for i := range list.Items {
		pod := &list.Items[i]
		if pod.DeletionTimestamp == nil && pod.Status.Phase == corev1.PodRunning {
			candidates = append(candidates, pod)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no running pod found for %s", target)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if ra, rb := podReady(a), podReady(b); ra != rb {
			return ra
		}
		if ra, rb := podRestarts(a), podRestarts(b); ra != rb {
			return ra < rb
		}
		return a.CreationTimestamp.Before(&b.CreationTimestamp)
	})
	return candidates[0], nil
}

func podReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podRestarts(pod *corev1.Pod) int32 {
	var total int32
	for _, s := range pod.Status.ContainerStatuses {
		total += s.RestartCount
	}
	return total
}

// servicePorts maps Service ports to the pod's container ports via each
// Service port's targetPort. A remote port the Service doesn't declare is an
// error, exactly as in kubectl.
func servicePorts(svc *corev1.Service, pod *corev1.Pod, mappings []PortMapping) ([]forwardedPort, error) {
	out := make([]forwardedPort, 0, len(mappings))
	for _, m := range mappings {
		svcPort, err := findServicePort(svc, m.Remote)
		if err != nil {
			return nil, err
		}

		var containerPort int32
		switch {
		case svcPort.TargetPort.Type == intstr.String:
			if containerPort, err = lookupContainerPort(pod, svcPort.TargetPort.StrVal); err != nil {
				return nil, err
			}
		case svcPort.TargetPort.IntValue() == 0:
			containerPort = svcPort.Port
		default:
			containerPort = int32(svcPort.TargetPort.IntValue())
		}
		out = append(out, forwardedPort{Local: m.Local, Remote: int(containerPort)})
	}
	return out, nil
}

func findServicePort(svc *corev1.Service, remote string) (*corev1.ServicePort, error) {
	num, numErr := strconv.Atoi(remote)
	for i := range svc.Spec.Ports {
		p := &svc.Spec.Ports[i]
		if (numErr == nil && int(p.Port) == num) || (numErr != nil && p.Name == remote) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("service %s does not have a service port %s", svc.Name, remote)
}

func containerPorts(pod *corev1.Pod, mappings []PortMapping) ([]forwardedPort, error) {
	out := make([]forwardedPort, 0, len(mappings))
	for _, m := range mappings {
		if n, err := strconv.Atoi(m.Remote); err == nil {
			out = append(out, forwardedPort{Local: m.Local, Remote: n})
			continue
		}
		port, err := lookupContainerPort(pod, m.Remote)
		if err != nil {
			return nil, err
		}
		out = append(out, forwardedPort{Local: m.Local, Remote: int(port)})
	}
	return out, nil
}

func lookupContainerPort(pod *corev1.Pod, name string) (int32, error) {
	for _, c := range pod.Spec.Containers {
		for _, p := range c.Ports {
			if p.Name == name {
				return p.ContainerPort, nil
			}
		}
	}
	return 0, fmt.Errorf("pod %s has no container port named %q", pod.Name, name)
}
