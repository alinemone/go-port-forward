package kube

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func runningPod(uid string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "db-0", UID: types.UID(uid)},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func TestPodGoneReason(t *testing.T) {
	want := runningPod("a")
	terminating := runningPod("a")
	now := metav1.Now()
	terminating.DeletionTimestamp = &now
	failed := runningPod("a")
	failed.Status.Phase = corev1.PodFailed
	replaced := runningPod("b")
	healthy := runningPod("a")

	for _, tc := range []struct {
		name string
		got  *corev1.Pod
		err  error
		want string
	}{
		{"healthy", &healthy, nil, ""},
		{"deleted", &corev1.Pod{}, &apiError{code: 404, msg: "not found"}, "was deleted"},
		{"replaced", &replaced, nil, "was replaced"},
		{"terminating", &terminating, nil, "is terminating"},
		{"not running", &failed, nil, "no longer running"},
		{"api unreachable", &corev1.Pod{}, errors.New("dial tcp: i/o timeout"), ""},
		{"forbidden", &corev1.Pod{}, &apiError{code: 403, msg: "forbidden"}, ""},
	} {
		got := podGoneReason(&want, tc.got, tc.err)
		if (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Errorf("%s: reason = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestWatchPodClosesTunnelWhenPodIsReplaced(t *testing.T) {
	orig := podWatchInterval
	podWatchInterval = 10 * time.Millisecond
	defer func() { podWatchInterval = orig }()

	var checks atomic.Int32
	get := func(_ context.Context, path string, _ url.Values, out any) error {
		if checks.Add(1) < 3 {
			*out.(*corev1.Pod) = runningPod("a") // still the same pod
			return nil
		}
		*out.(*corev1.Pod) = runningPod("b") // a rollout replaced it
		return nil
	}

	pod := runningPod("a")
	conn := newFakePod("")
	var logged string
	done := make(chan struct{})
	go func() {
		watchPod(context.Background(), get, "prod", &pod, conn, func(m string, _ bool) { logged = m })
		close(done)
	}()

	select {
	case <-conn.CloseChan():
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not close the tunnel after the pod was replaced")
	}
	<-done
	if !strings.Contains(logged, "replaced") || checks.Load() < 3 {
		t.Errorf("log = %q after %d checks", logged, checks.Load())
	}
}

func TestWatchPodKeepsTunnelWhenAPIIsFlaky(t *testing.T) {
	orig := podWatchInterval
	podWatchInterval = 5 * time.Millisecond
	defer func() { podWatchInterval = orig }()

	get := func(context.Context, string, url.Values, any) error { return errors.New("connection refused") }
	pod := runningPod("a")
	conn := newFakePod("")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	watchPod(ctx, get, "prod", &pod, conn, func(string, bool) {})

	select {
	case <-conn.CloseChan():
		t.Fatal("an unreachable API must not tear down a possibly-healthy tunnel")
	default:
	}
}
