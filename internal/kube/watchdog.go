package kube

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
)

// podWatchInterval is how often a tunnel checks that its pod is still the one
// to talk to. A var so tests can shrink it.
var podWatchInterval = 20 * time.Second

// podWatchTimeout bounds each check so a slow API server can't stall it.
const podWatchTimeout = 10 * time.Second

// watchPod keeps a tunnel honest while it sits idle. The connection to the API
// server outlives its pod: after a rollout or failover the tunnel still looks
// live, and only the next client connection fails. So it periodically checks
// the pod, and when it was deleted, replaced, is terminating or no longer
// running, it closes the connection — the caller then reconnects and resolves
// a healthy pod before anyone hits the dead one.
//
// A failed check (API unreachable, forbidden) closes nothing: the tunnel may be
// fine, and a dead network is caught by the connection's own keepalives.
func watchPod(ctx context.Context, get getter, ns string, pod *corev1.Pod, conn httpstream.Connection, log func(string, bool)) {
	ticker := time.NewTicker(podWatchInterval)
	defer ticker.Stop()
	path := "api/v1/namespaces/" + ns + "/pods/" + pod.Name

	for {
		select {
		case <-ctx.Done():
			return
		case <-conn.CloseChan():
			return
		case <-ticker.C:
		}

		checkCtx, cancel := context.WithTimeout(ctx, podWatchTimeout)
		var current corev1.Pod
		err := get(checkCtx, path, nil, &current)
		cancel()
		if ctx.Err() != nil {
			return
		}

		reason := podGoneReason(pod, &current, err)
		if reason == "" {
			continue
		}
		log(reason+" — reconnecting to a healthy pod", true)
		conn.Close()
		return
	}
}

// podGoneReason says why the tunnel's pod can no longer be used, or "" when it
// still can (or the check was inconclusive).
func podGoneReason(want, got *corev1.Pod, err error) string {
	switch {
	case err != nil && isNotFound(err):
		return fmt.Sprintf("pod %s was deleted", want.Name)
	case err != nil:
		return ""
	case want.UID != "" && got.UID != want.UID:
		return fmt.Sprintf("pod %s was replaced", want.Name)
	case got.DeletionTimestamp != nil:
		return fmt.Sprintf("pod %s is terminating", want.Name)
	case got.Status.Phase != corev1.PodRunning:
		return fmt.Sprintf("pod %s is no longer running (%s)", want.Name, got.Status.Phase)
	}
	return ""
}
