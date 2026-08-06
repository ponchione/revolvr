package audit

import (
	"path"
	"slices"
	"sort"
	"strings"
)

type RouteReason struct {
	Kind    Kind     `json:"kind"`
	Signals []string `json:"signals"`
}

// RouteSpecialists is a pure host decision over admitted task risk and actual
// changed paths/symbols. Model output cannot add or remove audit kinds.
func RouteSpecialists(task TaskEvidence, changed []ChangedFile, blast []BlastRadiusEdge) []RouteReason {
	signals := map[Kind]map[string]struct{}{}
	add := func(kind Kind, signal string) {
		if signals[kind] == nil {
			signals[kind] = map[string]struct{}{}
		}
		signals[kind][signal] = struct{}{}
	}
	risk := strings.ToLower(task.RiskClass)
	mutation := strings.ToLower(task.MutationClass)
	if risk == "high" || risk == "critical" {
		add(KindSecurity, "task-risk:"+risk)
		add(KindIntegration, "task-risk:"+risk)
	}
	if strings.Contains(mutation, "api") {
		add(KindAPICompatibility, "mutation-class:"+mutation)
	}
	if strings.Contains(mutation, "database") || strings.Contains(mutation, "schema") || strings.Contains(mutation, "migration") {
		add(KindMigration, "mutation-class:"+mutation)
	}
	components := map[string]struct{}{}
	for _, file := range changed {
		lower := strings.ToLower(file.Path)
		base := strings.ToLower(path.Base(file.Path))
		component := strings.SplitN(lower, "/", 2)[0]
		if component != "" {
			components[component] = struct{}{}
		}
		switch {
		case strings.HasPrefix(lower, "db/migrations/") || strings.Contains(base, "migration") || strings.HasSuffix(lower, ".sql"):
			add(KindMigration, "changed-path:"+file.Path)
		case strings.HasPrefix(lower, "docs/") || base == "readme.md" || strings.HasSuffix(lower, ".md"):
			add(KindDocumentation, "changed-path:"+file.Path)
		}
		if containsAny(lower, "auth", "crypto", "secret", "permission", "sandbox", "policy", "security") || containsAnyJoined(file.Symbols, "auth", "permission", "encrypt", "token", "secret") {
			add(KindSecurity, "security-surface:"+file.Path)
		}
		if containsAny(lower, "api/", "handler", "protocol", "schema", "contract") || containsAnyJoined(file.Symbols, "api", "handler", "request", "response", "public") {
			add(KindAPICompatibility, "api-surface:"+file.Path)
		}
		if containsAny(lower, "benchmark", "performance", "cache", "index", "scheduler", "queue") || containsAnyJoined(file.Symbols, "benchmark", "hotpath", "cache", "batch") {
			add(KindPerformance, "performance-surface:"+file.Path)
		}
		if containsAny(lower, "integration", "compose/", "external", "client", "server") {
			add(KindIntegration, "integration-surface:"+file.Path)
		}
	}
	if len(components) > 1 || len(blast) > len(changed) {
		add(KindIntegration, "cross-component-change")
	}
	ordered := []Kind{KindSecurity, KindPerformance, KindIntegration, KindMigration, KindDocumentation, KindAPICompatibility}
	result := make([]RouteReason, 0, len(signals))
	for _, kind := range ordered {
		set := signals[kind]
		if len(set) == 0 {
			continue
		}
		values := make([]string, 0, len(set))
		for signal := range set {
			values = append(values, signal)
		}
		sort.Strings(values)
		result = append(result, RouteReason{Kind: kind, Signals: values})
	}
	return result
}

func containsAny(value string, needles ...string) bool {
	return slices.ContainsFunc(needles, func(needle string) bool { return strings.Contains(value, needle) })
}

func containsAnyJoined(values []string, needles ...string) bool {
	return containsAny(strings.ToLower(strings.Join(values, "\x00")), needles...)
}
