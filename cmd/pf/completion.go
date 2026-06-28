package main

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/theme"
)

// The functions below are Cobra ValidArgsFunctions: given the words typed so
// far they return candidate completions. The shell (bash/zsh/fish/powershell)
// filters by the typed prefix, so we return the full set and never touch files.

// serviceNames returns saved service names (sorted, best-effort).
func serviceNames() []string {
	names, err := storage.NewStorage().ListServiceNames()
	if err != nil {
		return nil
	}
	return names
}

// groupNames returns saved group names (sorted, best-effort).
func groupNames() []string {
	groups, err := storage.NewStorage().ListGroups()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// multiComplete completes a multi-value list of names. Multiple values work
// both space-separated (`pf run a b`) and comma-separated (`pf run a,b`) — the
// runner accepts either separator.
//
// We always return BARE names (the part the shell will substitute), never a
// `prefix+name`: the shell replaces only the segment after the last separator,
// so returning a prefixed candidate would duplicate the prefix (`a,a,b`). When
// the word so far is `a,b,c…`, we filter by the partial after the last comma and
// drop names already in the list, so a name is never offered twice. NoSpace is
// added inside a comma list so the user can keep typing `,next`.
func multiComplete(all, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	used := make(map[string]bool)
	mark := func(s string) {
		for _, w := range strings.Split(s, ",") {
			if w = strings.TrimSpace(w); w != "" {
				used[w] = true
			}
		}
	}
	for _, a := range args {
		mark(a)
	}

	last := toComplete
	inCommaList := false
	if i := strings.LastIndex(toComplete, ","); i >= 0 {
		mark(toComplete[:i])
		last = toComplete[i+1:]
		inCommaList = true
	}

	out := make([]string, 0, len(all))
	for _, name := range all {
		if !used[name] && strings.HasPrefix(name, last) {
			out = append(out, name)
		}
	}

	dir := cobra.ShellCompDirectiveNoFileComp
	if inCommaList {
		dir |= cobra.ShellCompDirectiveNoSpace
	}
	return out, dir
}

// completeServicesAndGroups completes a multi-value list of services/groups
// (used by `run` and the bare-name shortcut).
func completeServicesAndGroups(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return multiComplete(append(serviceNames(), groupNames()...), args, toComplete)
}

// completeServiceList completes a multi-value list of services (group add).
func completeServiceList(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return multiComplete(serviceNames(), args, toComplete)
}

// completeServices completes a single service name (delete).
func completeServices(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return serviceNames(), cobra.ShellCompDirectiveNoFileComp
}

// completeGroups completes a single group name (group delete/rename).
func completeGroups(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return groupNames(), cobra.ShellCompDirectiveNoFileComp
}

// completeGroupThenServices completes the group name as the first argument and a
// comma-separated list of services for the rest (group add-service/remove-service).
func completeGroupThenServices(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return groupNames(), cobra.ShellCompDirectiveNoFileComp
	}
	return multiComplete(serviceNames(), args[1:], toComplete)
}

func completeThemes(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	// Best-effort so user-defined palettes complete alongside the built-ins.
	_ = storage.NewStorage().RegisterCustomThemes()
	return append(theme.Names(), "list"), cobra.ShellCompDirectiveNoFileComp
}

func completeIconArgs(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"on", "off", "status"}, cobra.ShellCompDirectiveNoFileComp
}
