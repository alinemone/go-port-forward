// Package kube runs `kubectl port-forward`-equivalent tunnels in-process with
// client-go, so pf needs no kubectl binary and spawns no child processes.
package kube

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Spec is a parsed `kubectl port-forward` command line. Services are still
// stored as the familiar kubectl command string; Spec is what pf actually runs.
type Spec struct {
	Kubeconfig            string
	Context               string
	Cluster               string
	User                  string
	Server                string
	Token                 string
	InsecureSkipTLSVerify bool
	ClientCert            string
	ClientKey             string

	Namespace string // empty → the kube context's namespace (or "default")
	Kind      string // pod, service, deployment, statefulset, replicaset, daemonset
	Name      string
	Ports     []PortMapping
	Addresses []string // local bind addresses; empty → localhost
}

// PortMapping is one LOCAL:REMOTE pair. Remote is kept as a string because, like
// kubectl, it may name a port (a Service port name or a container port name).
type PortMapping struct {
	Local  int
	Remote string
}

// Target renders the forward target the way kubectl prints it, e.g. "service/db".
func (s Spec) Target() string {
	return s.Kind + "/" + s.Name
}

var kindAliases = map[string]string{
	"po": "pod", "pod": "pod", "pods": "pod",
	"svc": "service", "service": "service", "services": "service",
	"deploy": "deployment", "deployment": "deployment", "deployments": "deployment",
	"sts": "statefulset", "statefulset": "statefulset", "statefulsets": "statefulset",
	"rs": "replicaset", "replicaset": "replicaset", "replicasets": "replicaset",
	"ds": "daemonset", "daemonset": "daemonset", "daemonsets": "daemonset",
}

// valueFlags are the kubectl flags pf honours that take a value. ignoredValueFlags
// are accepted (so existing commands keep working) but have no effect here.
var (
	valueFlags = map[string]bool{
		"namespace": true, "context": true, "kubeconfig": true, "cluster": true,
		"user": true, "server": true, "token": true, "address": true,
		"client-certificate": true, "client-key": true,
	}
	ignoredValueFlags = map[string]bool{
		"pod-running-timeout": true, "request-timeout": true, "v": true,
	}
	shortFlags = map[string]string{"n": "namespace", "s": "server", "v": "v"}

	portNameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
)

// ParseCommand parses a `kubectl ... port-forward TARGET PORT...` command line.
// Anything pf cannot run faithfully in-process (another kubectl verb, an unknown
// flag, a random local port) is rejected with an error rather than guessed at.
func ParseCommand(command string) (Spec, error) {
	var spec Spec

	args, err := splitArgs(command)
	if err != nil {
		return spec, err
	}
	if len(args) == 0 {
		return spec, fmt.Errorf("command is empty")
	}
	if !isKubectl(args[0]) {
		return spec, fmt.Errorf("only kubectl port-forward commands are supported (got %q)", args[0])
	}

	var positional []string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}

		name, value, hasValue := splitFlag(arg)
		if name == "insecure-skip-tls-verify" {
			spec.InsecureSkipTLSVerify = !hasValue || value == "true"
			continue
		}
		if !valueFlags[name] && !ignoredValueFlags[name] {
			return spec, fmt.Errorf("unsupported flag %q", arg)
		}
		if !hasValue {
			if i+1 >= len(args) {
				return spec, fmt.Errorf("flag %q needs a value", arg)
			}
			i++
			value = args[i]
		}

		switch name {
		case "namespace":
			spec.Namespace = value
		case "context":
			spec.Context = value
		case "kubeconfig":
			spec.Kubeconfig = value
		case "cluster":
			spec.Cluster = value
		case "user":
			spec.User = value
		case "server":
			spec.Server = value
		case "token":
			spec.Token = value
		case "client-certificate":
			spec.ClientCert = value
		case "client-key":
			spec.ClientKey = value
		case "address":
			for _, addr := range strings.Split(value, ",") {
				if addr = strings.TrimSpace(addr); addr != "" {
					spec.Addresses = append(spec.Addresses, addr)
				}
			}
		}
	}

	if len(positional) == 0 || positional[0] != "port-forward" {
		return spec, fmt.Errorf("only kubectl port-forward commands are supported")
	}
	positional = positional[1:]
	if len(positional) < 2 {
		return spec, fmt.Errorf("port-forward needs a target and at least one port, e.g. svc/db 5432:5432")
	}

	if spec.Kind, spec.Name, err = parseTarget(positional[0]); err != nil {
		return spec, err
	}
	for _, p := range positional[1:] {
		mapping, err := parsePortMapping(p)
		if err != nil {
			return spec, err
		}
		spec.Ports = append(spec.Ports, mapping)
	}

	return spec, nil
}

func isKubectl(arg string) bool {
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(arg, `\`, "/")))
	return base == "kubectl" || base == "kubectl.exe"
}

// splitFlag splits "--name=value", "--name", "-n=value", "-nvalue" and "-n" into
// the long flag name and an optional inline value.
func splitFlag(arg string) (name, value string, hasValue bool) {
	if strings.HasPrefix(arg, "--") {
		name = arg[2:]
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			return name[:eq], name[eq+1:], true
		}
		return name, "", false
	}

	short := arg[1:2]
	name = short
	if long, ok := shortFlags[short]; ok {
		name = long
	}
	rest := strings.TrimPrefix(arg[2:], "=")
	if rest != "" {
		return name, rest, true
	}
	return name, "", false
}

func parseTarget(arg string) (kind, name string, err error) {
	typ, name, found := strings.Cut(arg, "/")
	if !found {
		// A bare NAME is a pod, as in kubectl.
		typ, name = "pod", arg
	}
	// Accept group-qualified types such as deployment.apps/NAME.
	typ, _, _ = strings.Cut(strings.ToLower(typ), ".")
	kind, ok := kindAliases[typ]
	if !ok {
		return "", "", fmt.Errorf("unsupported port-forward target type %q", typ)
	}
	if name == "" {
		return "", "", fmt.Errorf("port-forward target %q has no name", arg)
	}
	return kind, name, nil
}

func parsePortMapping(arg string) (PortMapping, error) {
	local, remote, found := strings.Cut(arg, ":")
	if !found {
		local, remote = arg, arg
	}
	if local == "" {
		return PortMapping{}, fmt.Errorf("port %q: a random local port is not supported, set one explicitly (e.g. 5432:5432)", arg)
	}

	localPort, err := strconv.Atoi(local)
	if err != nil || localPort < 1 || localPort > 65535 {
		return PortMapping{}, fmt.Errorf("port %q: invalid local port %q", arg, local)
	}
	if n, err := strconv.Atoi(remote); err == nil {
		if n < 1 || n > 65535 {
			return PortMapping{}, fmt.Errorf("port %q: invalid remote port %q", arg, remote)
		}
	} else if !portNameRegex.MatchString(remote) {
		return PortMapping{}, fmt.Errorf("port %q: invalid remote port %q", arg, remote)
	}

	return PortMapping{Local: localPort, Remote: remote}, nil
}

// splitArgs splits a command line into arguments, honouring single and double
// quotes. Backslashes are literal so Windows paths survive untouched.
func splitArgs(s string) ([]string, error) {
	var (
		args    []string
		current strings.Builder
		inArg   bool
		quote   rune
	)
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			inArg = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inArg {
				args = append(args, current.String())
				current.Reset()
				inArg = false
			}
		default:
			current.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in command")
	}
	if inArg {
		args = append(args, current.String())
	}
	return args, nil
}
