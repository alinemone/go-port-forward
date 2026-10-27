package ui

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/alinemone/go-port-forward/internal/cert"
	"github.com/alinemone/go-port-forward/internal/kube"
	"github.com/alinemone/go-port-forward/internal/manager"
)

// Discover: pick a namespace, choose which of its services to forward (data
// stores are suggested), review names/ports/commands, and save them all.

type discoverStep int

const (
	stepTarget discoverStep = iota
	stepSelect
	stepReview
)

const discoverTimeout = 20 * time.Second

// The cluster calls the wizard makes; vars so tests can stand in a cluster.
var (
	kubeContexts     = kube.Contexts
	listNamespaces   = kube.ListNamespaces
	discoverServices = kube.DiscoverServices
)

type discoverState struct {
	active        bool
	standalone    bool // `pf discover`: finishing or cancelling ends the program
	fromDashboard bool // opened with d on the dashboard: cancelling returns there
	step          discoverStep

	// target
	contexts       []string
	ctxIndex       int
	currentContext string
	nsInput        textinput.Model
	namespaces     []string // for autocomplete; nil when unavailable
	nsError        string
	nsFromSaved    bool // namespaces came from saved services, not the cluster
	suggestion     int  // highlighted autocomplete row, -1 = none

	// select
	namespace  string
	loading    bool
	scanError  string
	candidates []kube.Candidate
	selected   map[int]bool
	saved      map[string]string // "ns/service" → name it is already saved as
	cursor     int

	// review
	items    []reviewItem
	focus    int // field index across items: item*3 + (0 name, 1 port, 2 command)
	errorMsg string

	added []string // names saved by the last successful add
}

type reviewItem struct {
	cand      kube.Candidate
	name      textinput.Model
	port      textinput.Model
	command   textinput.Model
	cmdEdited bool // the command was edited by hand: stop regenerating it
}

type namespacesMsg struct {
	context string
	names   []string
	err     error
}

type scanMsg struct {
	namespace  string
	candidates []kube.Candidate
	err        error
}

func discoverOptions() kube.Options {
	mgr, err := cert.NewManager()
	if err != nil {
		return kube.Options{}
	}
	if c, ok := mgr.GetCertificate(); ok {
		return kube.Options{ClientCert: c.CertPath, ClientKey: c.KeyPath}
	}
	return kube.Options{}
}

func (u *UI) kubeContext() string {
	if d := &u.discover; d.ctxIndex >= 0 && d.ctxIndex < len(d.contexts) {
		return d.contexts[d.ctxIndex]
	}
	return ""
}

// openDiscover starts the wizard. A namespace skips straight to scanning it.
func (u *UI) openDiscover(namespace string, standalone bool) tea.Cmd {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "e.g. prod-salam-gate"
	in.CharLimit = 63
	in.SetWidth(40)

	u.discover = discoverState{active: true, standalone: standalone, nsInput: in, suggestion: -1}
	d := &u.discover
	d.contexts, d.currentContext, _ = kubeContexts()
	for i, c := range d.contexts {
		if c == d.currentContext {
			d.ctxIndex = i
		}
	}

	if namespace != "" {
		d.nsInput.SetValue(namespace)
		return tea.Batch(u.loadNamespaces(), u.startScan(namespace))
	}
	return tea.Batch(u.loadNamespaces(), d.nsInput.Focus())
}

func (u *UI) closeDiscover() tea.Cmd {
	standalone, fromDashboard := u.discover.standalone, u.discover.fromDashboard
	added := u.discover.added
	u.discover = discoverState{added: added}
	switch {
	case standalone:
		return tea.Quit
	case fromDashboard && len(added) == 0:
		u.exitManageMode()
	}
	return nil
}

func (u *UI) loadNamespaces() tea.Cmd {
	kctx := u.kubeContext()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), discoverTimeout)
		defer cancel()
		names, err := listNamespaces(ctx, kctx, discoverOptions())
		return namespacesMsg{context: kctx, names: names, err: err}
	}
}

func (u *UI) startScan(namespace string) tea.Cmd {
	d := &u.discover
	d.step, d.namespace, d.loading, d.scanError = stepSelect, namespace, true, ""
	d.candidates, d.selected, d.cursor = nil, map[int]bool{}, 0
	d.nsInput.Blur()
	kctx := u.kubeContext()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), discoverTimeout)
		defer cancel()
		cands, err := discoverServices(ctx, kctx, namespace, discoverOptions())
		return scanMsg{namespace: namespace, candidates: cands, err: err}
	}
}

// nsSuggestions are the namespaces matching what's typed: prefix matches
// first, then substring matches.
func (u *UI) nsSuggestions(limit int) []string {
	query := strings.TrimSpace(u.discover.nsInput.Value())
	var prefix, contains []string
	for _, ns := range u.discover.namespaces {
		switch {
		case strings.HasPrefix(ns, query):
			prefix = append(prefix, ns)
		case strings.Contains(ns, query):
			contains = append(contains, ns)
		}
	}
	out := append(prefix, contains...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

const maxNSSuggestions = 6

func (u *UI) handleDiscoverMsg(msg tea.Msg) (bool, tea.Cmd) {
	d := &u.discover
	switch msg := msg.(type) {
	case namespacesMsg:
		if !d.active || msg.context != u.kubeContext() {
			return true, nil
		}
		d.namespaces, d.nsError, d.nsFromSaved = msg.names, "", false
		if msg.err != nil {
			// Listing namespaces is often forbidden; fall back to the ones the
			// saved services already use.
			d.nsError = msg.err.Error()
			if saved := u.savedNamespaces(); len(saved) > 0 {
				d.namespaces, d.nsFromSaved = saved, true
			}
		}
		return true, nil
	case scanMsg:
		if !d.active || msg.namespace != d.namespace {
			return true, nil
		}
		d.loading = false
		if msg.err != nil {
			d.scanError = msg.err.Error()
			return true, nil
		}
		d.candidates = msg.candidates
		d.saved = u.savedTargets()
		for i, c := range d.candidates {
			if c.Tier == kube.TierSuggested && d.saved[d.namespace+"/"+c.Service] == "" {
				d.selected[i] = true
			}
		}
		return true, nil
	}
	return false, nil
}

// savedNamespaces lists the namespaces the saved services forward from.
func (u *UI) savedNamespaces() []string {
	seen := map[string]bool{}
	var out []string
	services, _ := u.store.LoadServices()
	for _, cmd := range services {
		if spec, err := kube.ParseCommand(cmd); err == nil && spec.Namespace != "" && !seen[spec.Namespace] {
			seen[spec.Namespace] = true
			out = append(out, spec.Namespace)
		}
	}
	sort.Strings(out)
	return out
}

// savedTargets maps "namespace/service" of every saved svc/ forward to its
// saved name, so already-added services are shown as such.
func (u *UI) savedTargets() map[string]string {
	out := map[string]string{}
	services, _ := u.store.LoadServices()
	for name, cmd := range services {
		if spec, err := kube.ParseCommand(cmd); err == nil && spec.Kind == "service" {
			out[spec.Namespace+"/"+spec.Name] = name
		}
	}
	return out
}

func (u *UI) updateDiscover(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyPressMsg)
	if !isKey {
		return u, u.forwardDiscoverInput(msg)
	}
	key := normalizeKeyToken(keyMsg)
	d := &u.discover

	switch d.step {
	case stepTarget:
		suggestions := u.nsSuggestions(maxNSSuggestions)
		switch key {
		case "esc":
			return u, u.closeDiscover()
		case "ctrl+t":
			if len(d.contexts) > 1 {
				d.ctxIndex = (d.ctxIndex + 1) % len(d.contexts)
				d.namespaces, d.nsError, d.suggestion = nil, "", -1
				return u, u.loadNamespaces()
			}
			return u, nil
		case "down":
			if d.suggestion < len(suggestions)-1 {
				d.suggestion++
			}
			return u, nil
		case "up":
			if d.suggestion >= 0 {
				d.suggestion--
			}
			return u, nil
		case "tab":
			if len(suggestions) > 0 {
				pick := max(0, d.suggestion)
				d.nsInput.SetValue(suggestions[min(pick, len(suggestions)-1)])
				d.nsInput.CursorEnd()
				d.suggestion = -1
			}
			return u, nil
		case "enter":
			ns := strings.TrimSpace(d.nsInput.Value())
			if d.suggestion >= 0 && d.suggestion < len(suggestions) {
				ns = suggestions[d.suggestion]
				d.nsInput.SetValue(ns)
			}
			if ns == "" {
				return u, nil
			}
			return u, u.startScan(ns)
		}
		before := d.nsInput.Value()
		cmd := u.forwardDiscoverInput(msg)
		if d.nsInput.Value() != before {
			d.suggestion = -1
		}
		return u, cmd

	case stepSelect:
		switch key {
		case "esc":
			d.step = stepTarget
			return u, d.nsInput.Focus()
		case "up", "k":
			if d.cursor > 0 {
				d.cursor--
			}
		case "down", "j":
			if d.cursor < len(d.candidates)-1 {
				d.cursor++
			}
		case "space":
			if d.cursor < len(d.candidates) && d.saved[d.namespace+"/"+d.candidates[d.cursor].Service] == "" {
				d.selected[d.cursor] = !d.selected[d.cursor]
			}
		case "enter":
			if !d.loading && d.scanError == "" && u.discoverSelectionCount() > 0 {
				return u, u.startReview()
			}
		}
		return u, nil

	case stepReview:
		fields := len(d.items) * 3
		switch key {
		case "esc":
			d.step, d.errorMsg = stepSelect, ""
			return u, nil
		case "tab", "down":
			return u, u.focusReview((d.focus + 1) % fields)
		case "shift+tab", "up":
			return u, u.focusReview((d.focus - 1 + fields) % fields)
		case "enter":
			return u, u.addDiscovered()
		}
		item := &d.items[d.focus/3]
		before := item.command.Value()
		cmd := u.forwardDiscoverInput(msg)
		switch d.focus % 3 {
		case 2:
			if item.command.Value() != before {
				item.cmdEdited = true
			}
		default:
			u.regenerateCommand(item)
		}
		d.errorMsg = ""
		return u, cmd
	}
	return u, nil
}

func (u *UI) forwardDiscoverInput(msg tea.Msg) tea.Cmd {
	d := &u.discover
	var cmd tea.Cmd
	switch {
	case d.step == stepTarget:
		d.nsInput, cmd = d.nsInput.Update(msg)
	case d.step == stepReview && len(d.items) > 0:
		item := &d.items[d.focus/3]
		switch d.focus % 3 {
		case 0:
			item.name, cmd = item.name.Update(msg)
		case 1:
			item.port, cmd = item.port.Update(msg)
		case 2:
			item.command, cmd = item.command.Update(msg)
		}
	}
	return cmd
}

func (u *UI) discoverSelectionCount() int {
	n := 0
	for _, on := range u.discover.selected {
		if on {
			n++
		}
	}
	return n
}

// startReview builds one editable item per selected service with a proposed
// name, a free local port and the generated command.
func (u *UI) startReview() tea.Cmd {
	d := &u.discover
	d.step, d.focus, d.errorMsg, d.items = stepReview, 0, "", nil

	services, _ := u.store.LoadServices()
	usedNames := map[string]bool{}
	usedPorts := map[int]bool{}
	for name, cmd := range services {
		usedNames[name] = true
		if local, _ := portsOf(cmd); local > 0 {
			usedPorts[local] = true
		}
	}

	for i, c := range d.candidates {
		if !d.selected[i] {
			continue
		}
		name := uniqueName(proposeName(d.namespace, c), usedNames)
		usedNames[name] = true
		port := freeLocalPort(c.Port, usedPorts)
		usedPorts[port] = true

		item := reviewItem{
			cand:    c,
			name:    newServiceTextInput("service name", name, 26),
			port:    newServiceTextInput("port", strconv.Itoa(port), 7),
			command: newServiceTextInput("command", "", max(20, u.width-22)),
		}
		item.port.CharLimit = 5
		u.regenerateCommand(&item)
		d.items = append(d.items, item)
	}
	return u.focusReview(0)
}

func (u *UI) focusReview(field int) tea.Cmd {
	d := &u.discover
	for i := range d.items {
		d.items[i].name.Blur()
		d.items[i].port.Blur()
		d.items[i].command.Blur()
	}
	d.focus = field
	item := &d.items[field/3]
	switch field % 3 {
	case 0:
		return item.name.Focus()
	case 1:
		return item.port.Focus()
	default:
		return item.command.Focus()
	}
}

// regenerateCommand keeps the command in step with the port — until the user
// edits the command by hand.
func (u *UI) regenerateCommand(item *reviewItem) {
	if item.cmdEdited {
		return
	}
	port, err := strconv.Atoi(strings.TrimSpace(item.port.Value()))
	if err != nil {
		port = 0
	}
	d := &u.discover
	item.command.SetValue(kube.BuildCommand(u.kubeContext(), d.currentContext, d.namespace, item.cand.Service, port, item.cand.Port))
}

// reviewProblem validates one item against saved services and the other items.
func (u *UI) reviewProblem(i int, saved map[string]string) string {
	d := &u.discover
	item := &d.items[i]
	name := strings.TrimSpace(item.name.Value())
	if err := manager.ValidateServiceName(name); err != nil {
		return err.Error()
	}
	if _, exists := saved[name]; exists {
		return fmt.Sprintf("a service named %q already exists", name)
	}
	spec, err := kube.ParseCommand(strings.TrimSpace(item.command.Value()))
	if err != nil {
		return err.Error()
	}
	for j := range d.items {
		if j == i {
			continue
		}
		if strings.TrimSpace(d.items[j].name.Value()) == name {
			return fmt.Sprintf("name %q is used twice", name)
		}
		if other, err := kube.ParseCommand(d.items[j].command.Value()); err == nil && other.Ports[0].Local == spec.Ports[0].Local {
			return fmt.Sprintf("local port %d is used twice", spec.Ports[0].Local)
		}
	}
	return ""
}

// reviewWarning flags a local port another saved service already uses — fine
// as long as they never run together, so it doesn't block saving.
func (u *UI) reviewWarning(i int, saved map[string]string) string {
	spec, err := kube.ParseCommand(strings.TrimSpace(u.discover.items[i].command.Value()))
	if err != nil {
		return ""
	}
	for name, cmd := range saved {
		if local, _ := portsOf(cmd); local == spec.Ports[0].Local {
			return fmt.Sprintf("port %d is also used by %q", local, name)
		}
	}
	return ""
}

func (u *UI) addDiscovered() tea.Cmd {
	d := &u.discover
	saved, _ := u.store.LoadServices()
	for i := range d.items {
		if p := u.reviewProblem(i, saved); p != "" {
			d.errorMsg = fmt.Sprintf("%s: %s", strings.TrimSpace(d.items[i].name.Value()), p)
			return u.focusReview(i * 3)
		}
	}

	var added []string
	for i := range d.items {
		name := strings.TrimSpace(d.items[i].name.Value())
		if err := u.store.AddService(name, strings.TrimSpace(d.items[i].command.Value())); err != nil {
			d.errorMsg = err.Error()
			return nil
		}
		added = append(added, name)
	}
	d.added = added

	if d.standalone {
		return u.closeDiscover()
	}
	u.closeDiscover()
	// Back in the Hangar with the new services selected: Enter launches them.
	u.manage.searchQuery = ""
	u.reloadManageRowsFromStorage()
	for _, name := range added {
		u.manage.selectedServices[name] = true
	}
	u.focusManage(rowService, added[0])
	u.manage.infoMsg = fmt.Sprintf("✓ added %s — Enter to launch", strings.Join(added, ", "))
	return nil
}

func portsOf(command string) (local int, remote string) {
	spec, err := kube.ParseCommand(command)
	if err != nil {
		return 0, ""
	}
	return spec.Ports[0].Local, spec.Ports[0].Remote
}

var envPrefixes = []string{"prod-", "production-", "stage-", "staging-", "dev-", "test-"}

// proposeName suggests a saved-service name from the namespace and store:
// prod-salam-gate + postgres → salam-gate-pg; replicas get a "-replica".
func proposeName(namespace string, c kube.Candidate) string {
	base := namespace
	for _, p := range envPrefixes {
		base = strings.TrimPrefix(base, p)
	}
	base = strings.TrimSuffix(strings.TrimPrefix(base, "api-"), "-api")
	suffix := c.Short
	if suffix == "" {
		suffix = strings.TrimPrefix(strings.TrimPrefix(c.Service, namespace), "-")
		if suffix == "" {
			suffix = "svc"
		}
	}
	name := base + "-" + suffix
	if c.Note == "replica" {
		name += "-replica"
	}
	if len(name) > 50 {
		name = name[:50]
	}
	return strings.Trim(name, "-")
}

func uniqueName(name string, used map[string]bool) string {
	if !used[name] {
		return name
	}
	for i := 2; ; i++ {
		if candidate := fmt.Sprintf("%s-%d", name, i); !used[candidate] {
			return candidate
		}
	}
}

// freeLocalPort proposes a local port: the remote port itself when nothing
// claims it, else the first free one from remote+10000 up.
func freeLocalPort(remote int, used map[int]bool) int {
	ok := func(p int) bool { return p > 0 && p <= 65535 && !used[p] && portFree(p) }
	if ok(remote) {
		return remote
	}
	start := remote + 10000
	if start > 65535 {
		start = 20000
	}
	for p := start; p <= 65535; p++ {
		if ok(p) {
			return p
		}
	}
	return remote
}

func portFree(p int) bool {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
