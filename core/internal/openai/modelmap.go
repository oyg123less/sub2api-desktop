package openai

import "strings"

// codexModelMap normalizes user-facing model names (including reasoning-effort
// suffixed variants such as gpt-5.4-high) to the canonical model names accepted
// by the ChatGPT Codex backend. Mirrors upstream sub2api's codexModelMap.
var codexModelMap = map[string]string{
	"gpt-5.6-sol":          "gpt-5.6-sol",
	"gpt-5.6-terra":        "gpt-5.6-terra",
	"gpt-5.6-luna":         "gpt-5.6-luna",
	"gpt-5.5":              "gpt-5.5",
	"gpt-5.5-pro":          "gpt-5.5-pro",
	"codex-auto-review":    "codex-auto-review",
	"gpt-5.4":              "gpt-5.4",
	"gpt-5.4-mini":         "gpt-5.4-mini",
	"gpt-5.4-none":         "gpt-5.4",
	"gpt-5.4-low":          "gpt-5.4",
	"gpt-5.4-medium":       "gpt-5.4",
	"gpt-5.4-high":         "gpt-5.4",
	"gpt-5.4-xhigh":        "gpt-5.4",
	"gpt-5.4-chat-latest":  "gpt-5.4",
	"gpt-5.3":              "gpt-5.3-codex",
	"gpt-5.3-none":         "gpt-5.3-codex",
	"gpt-5.3-low":          "gpt-5.3-codex",
	"gpt-5.3-medium":       "gpt-5.3-codex",
	"gpt-5.3-high":         "gpt-5.3-codex",
	"gpt-5.3-xhigh":        "gpt-5.3-codex",
	"gpt-5.3-codex":        "gpt-5.3-codex",
	"gpt-5.3-codex-spark":  "gpt-5.3-codex-spark",
	"gpt-5.3-codex-low":    "gpt-5.3-codex",
	"gpt-5.3-codex-medium": "gpt-5.3-codex",
	"gpt-5.3-codex-high":   "gpt-5.3-codex",
	"gpt-5.3-codex-xhigh":  "gpt-5.3-codex",
	"gpt-5.2":              "gpt-5.2",
	"gpt-5.2-none":         "gpt-5.2",
	"gpt-5.2-low":          "gpt-5.2",
	"gpt-5.2-medium":       "gpt-5.2",
	"gpt-5.2-high":         "gpt-5.2",
	"gpt-5.2-xhigh":        "gpt-5.2",
	"gpt-5":                "gpt-5.4",
	"gpt-5-mini":           "gpt-5.4",
	"gpt-5-nano":           "gpt-5.4",
	"gpt-5.1":              "gpt-5.4",
	"gpt-5.1-codex":        "gpt-5.3-codex",
	"gpt-5.1-codex-max":    "gpt-5.3-codex",
	"gpt-5.1-codex-mini":   "gpt-5.3-codex",
	"gpt-5.2-codex":        "gpt-5.2",
	"codex-mini-latest":    "gpt-5.3-codex",
	"gpt-5-codex":          "gpt-5.3-codex",
}

// codexVersionModelPrefixes resolves unknown variants (e.g. dated snapshots)
// by longest-prefix match.
var codexVersionModelPrefixes = []struct {
	prefix string
	target string
}{
	{prefix: "gpt-5.6-sol", target: "gpt-5.6-sol"},
	{prefix: "gpt-5.6-terra", target: "gpt-5.6-terra"},
	{prefix: "gpt-5.6-luna", target: "gpt-5.6-luna"},
	{prefix: "gpt-5.3-codex-spark", target: "gpt-5.3-codex-spark"},
	{prefix: "gpt-5.3-codex", target: "gpt-5.3-codex"},
	{prefix: "gpt-5.4-mini", target: "gpt-5.4-mini"},
	{prefix: "gpt-5.5-pro", target: "gpt-5.5-pro"},
	{prefix: "gpt-5.5", target: "gpt-5.5"},
	{prefix: "gpt-5.4", target: "gpt-5.4"},
	{prefix: "gpt-5.2", target: "gpt-5.2"},
}

// NormalizeReasoningEffort maps a raw effort token to the canonical set
// ("low" | "medium" | "high" | "xhigh"); anything else yields "".
func NormalizeReasoningEffort(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)
	switch value {
	case "none", "minimal", "low", "medium", "high":
		return value
	case "xhigh", "extrahigh", "max":
		return "xhigh"
	default:
		return ""
	}
}

func isKnownEffortSuffix(suffix string) bool {
	switch suffix {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	}
	return false
}

// ReasoningEffortFromModel extracts the reasoning effort encoded as the last
// dash/underscore-separated segment of a model name (e.g. gpt-5.4-high → high).
func ReasoningEffortFromModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return ""
	}
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	parts := strings.FieldsFunc(m, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	if len(parts) == 0 {
		return ""
	}
	return NormalizeReasoningEffort(parts[len(parts)-1])
}

// MapCodexModel resolves the user-requested model to the upstream model name
// accepted by the ChatGPT Codex backend and the reasoning effort implied by
// its suffix (empty when none).
func MapCodexModel(model string) (upstream string, effort string) {
	effort = ReasoningEffortFromModel(model)
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	if m == "" {
		return model, effort
	}
	if target, ok := codexModelMap[m]; ok {
		return target, effort
	}
	if i := strings.LastIndex(m, "-"); i >= 0 && isKnownEffortSuffix(m[i+1:]) {
		if target, ok := codexModelMap[m[:i]]; ok {
			return target, effort
		}
	}
	for _, p := range codexVersionModelPrefixes {
		if strings.HasPrefix(m, p.prefix) {
			return p.target, effort
		}
	}
	return model, effort
}
