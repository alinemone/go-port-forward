package kube

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/httpstream"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/klog/v2"
)

// apiTimeout bounds the API lookups (kubeconfig → service → pod) that precede
// each tunnel, so an unreachable cluster fails fast and the caller can retry.
const apiTimeout = 30 * time.Second

func init() {
	// client-go logs through klog and reports per-connection failures through
	// utilruntime.ErrorHandlers, both of which write to stderr by default and
	// would corrupt the TUI. Silence the global logger and route every reported
	// error to the logger carried in its context — the per-service sink Forward
	// installs — so each error lands in the right service's log pane.
	klog.SetLogger(logr.Discard())
	utilruntime.ErrorHandlers = []utilruntime.ErrorHandler{
		func(ctx context.Context, err error, msg string, keysAndValues ...interface{}) {
			klog.FromContext(ctx).Error(err, msg, keysAndValues...)
		},
	}
}

// Options carries settings that apply to every forward, outside the command.
type Options struct {
	// ClientCert/ClientKey are PEM file paths of pf's configured certificate.
	// They are used unless the command sets --client-certificate/--client-key.
	ClientCert string
	ClientKey  string
}

// Hooks receives a forward's progress.
type Hooks struct {
	Log   func(message string, isError bool)
	Ready func() // called once every local listener is up
}

// Forward resolves spec to a pod and forwards its ports until ctx is cancelled
// (returning nil) or the tunnel breaks (returning the error). It never retries;
// reconnecting — and re-resolving the pod, which may have moved — is the
// caller's job.
func Forward(ctx context.Context, spec Spec, opts Options, hooks Hooks) error {
	if hooks.Log == nil {
		hooks.Log = func(string, bool) {}
	}

	cfg, ns, err := restConfig(spec, opts)
	if err != nil {
		return err
	}
	api, err := newAPIClient(cfg)
	if err != nil {
		return err
	}

	apiCtx, cancel := context.WithTimeout(ctx, apiTimeout)
	pod, ports, err := resolveTarget(apiCtx, api.get, ns, spec)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	hooks.Log(fmt.Sprintf("%s → pod %s (namespace %s)", spec.Target(), pod.Name, ns), false)

	logger := logr.New(&logSink{log: hooks.Log})
	reqURL := api.url("api/v1/namespaces/" + ns + "/pods/" + pod.Name + "/portforward")
	dialer, err := newDialer(cfg, reqURL, logger)
	if err != nil {
		return err
	}

	addresses := spec.Addresses
	if len(addresses) == 0 {
		addresses = []string{"localhost"}
	}

	fwCtx, stop := context.WithCancel(klog.NewContext(ctx, logger))
	defer stop()

	ready := make(chan struct{})
	fw, err := portforward.NewOnAddressesWithContext(fwCtx, dialer, addresses, ports, ready,
		lineWriter{log: hooks.Log}, lineWriter{log: hooks.Log, isError: true})
	if err != nil {
		return err
	}

	go func() {
		select {
		case <-ready:
			if hooks.Ready != nil {
				hooks.Ready()
			}
		case <-fwCtx.Done():
		}
	}()

	err = fw.ForwardPorts()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// restConfig builds the client config the way kubectl does for the same flags:
// the default kubeconfig loading rules (KUBECONFIG, ~/.kube/config) plus
// command-line overrides.
func restConfig(spec Spec, opts Options) (*rest.Config, string, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = spec.Kubeconfig

	overrides := &clientcmd.ConfigOverrides{CurrentContext: spec.Context}
	overrides.Context.Cluster = spec.Cluster
	overrides.Context.AuthInfo = spec.User
	overrides.ClusterInfo.Server = spec.Server
	overrides.ClusterInfo.InsecureSkipTLSVerify = spec.InsecureSkipTLSVerify
	overrides.AuthInfo.Token = spec.Token

	certFile, keyFile := spec.ClientCert, spec.ClientKey
	if certFile == "" && keyFile == "" {
		certFile, keyFile = opts.ClientCert, opts.ClientKey
	}
	overrides.AuthInfo.ClientCertificate = certFile
	overrides.AuthInfo.ClientKey = keyFile

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, "", err
	}
	// Server-side deprecation warnings would otherwise print to stderr.
	cfg.WarningHandler = rest.NoWarnings{}

	ns := spec.Namespace
	if ns == "" {
		if ns, _, err = clientConfig.Namespace(); err != nil {
			return nil, "", err
		}
	}
	return cfg, ns, nil
}

// newDialer prefers the WebSocket tunnel (the default since kubectl 1.31) and
// falls back to SPDY when the API server or a proxy can't upgrade to it.
func newDialer(cfg *rest.Config, u *url.URL, logger klog.Logger) (httpstream.Dialer, error) {
	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return nil, err
	}
	spdyDialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, u)

	wsDialer, err := portforward.NewSPDYOverWebsocketDialerWithLogger(logger, u, cfg)
	if err != nil {
		return nil, err
	}
	return portforward.NewFallbackDialer(wsDialer, spdyDialer, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	}), nil
}

// lineWriter turns client-go's "Forwarding from…" / "Handling connection for…"
// output into one log entry per line.
type lineWriter struct {
	log     func(string, bool)
	isError bool
}

func (w lineWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(string(p), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			w.log(line, w.isError)
		}
	}
	return len(p), nil
}

// logSink is a logr sink that forwards client-go's errors for one tunnel into
// that service's log. Verbose (V>0) info is dropped.
type logSink struct {
	log func(string, bool)
}

func (s *logSink) Init(logr.RuntimeInfo)                    {}
func (s *logSink) Enabled(level int) bool                   { return level <= 0 }
func (s *logSink) WithValues(...interface{}) logr.LogSink   { return s }
func (s *logSink) WithName(string) logr.LogSink             { return s }
func (s *logSink) Info(_ int, msg string, _ ...interface{}) { s.log(msg, false) }

func (s *logSink) Error(err error, msg string, _ ...interface{}) {
	if err != nil {
		msg += ": " + err.Error()
	}
	s.log(msg, true)
}
