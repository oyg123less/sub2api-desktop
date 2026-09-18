package openai

import (
	"strings"
	"time"
)

const (
	// The 5.6 rollout identifiers are explicit targets requested for Amber.
	// Existing saved choices are preserved; these defaults apply to new setups.
	DefaultGatewayModel = "gpt-5.6-sol"
	DefaultCodexModel   = "gpt-5.6-sol"
	DefaultTestModel    = "gpt-5.6-sol"
)

type catalogEntry struct {
	model       Model
	testOptions []string
}

// modelCatalog is the single source for advertised gateway models and model
// selectors. A zero Created value means the public creation timestamp is not
// known and avoids inventing metadata for rollout identifiers.
var modelCatalog = []catalogEntry{
	{model: catalogModel("gpt-6", "GPT-6 (Astra)", 1788480000), testOptions: []string{"gpt-6"}},
	{model: catalogModel("gpt-6-astra", "GPT-6 Astra", 1788480000), testOptions: []string{"gpt-6-astra"}},
	{model: catalogModel("gpt-5.6-sol", "GPT-5.6 Sol", 0), testOptions: []string{"gpt-5.6-sol"}},
	{model: catalogModel("gpt-5.6-terra", "GPT-5.6 Terra", 0), testOptions: []string{"gpt-5.6-terra"}},
	{model: catalogModel("gpt-5.6-luna", "GPT-5.6 Luna", 0), testOptions: []string{"gpt-5.6-luna"}},
	{model: catalogModel("gpt-5.5", "GPT-5.5", 1776873600), testOptions: []string{"gpt-5.5"}},
	{model: catalogModel("gpt-5.4", "GPT-5.4", 1738368000), testOptions: []string{
		"gpt-5.4", "gpt-5.4-low", "gpt-5.4-medium", "gpt-5.4-high", "gpt-5.4-xhigh",
	}},
	{model: catalogModel("gpt-5.4-mini", "GPT-5.4 Mini", 1738368000), testOptions: []string{"gpt-5.4-mini"}},
	{model: catalogModel("gpt-5.3-codex-spark", "GPT-5.3 Codex Spark", 1735689600), testOptions: []string{
		"gpt-5.3-codex-low", "gpt-5.3-codex-medium", "gpt-5.3-codex-high", "gpt-5.3-codex-xhigh", "gpt-5.3-codex-spark",
	}},
	{model: catalogModel("codex-auto-review", "Codex Auto Review", 1776902400), testOptions: []string{"codex-auto-review"}},
	{model: catalogModel("gpt-5.2", "GPT-5.2", 1733875200), testOptions: []string{
		"gpt-5.2", "gpt-5.2-medium", "gpt-5.2-high",
	}},
	{testOptions: []string{"gpt-5", "gpt-5-codex"}},
	{model: catalogModel("gpt-image-1", "GPT Image 1", 1733875200)},
	{model: catalogModel("gpt-image-1.5", "GPT Image 1.5", 1735689600)},
	{model: catalogModel("gpt-image-2", "GPT Image 2", 1738368000)},
}

func catalogModel(id, displayName string, created int64) Model {
	return Model{
		ID: id, Object: "model", Created: created, OwnedBy: "openai", Type: "model", DisplayName: displayName,
	}
}

// ParseModelList splits a comma, newline, or whitespace separated string into unique model IDs.
func ParseModelList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ';'
	})
	seen := make(map[string]struct{})
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		id := strings.TrimSpace(f)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result
}

// GatewayModels returns a copy of the models advertised by /v1/models,
// optionally merging extra dynamic or custom models.
func GatewayModels(extra ...string) []Model {
	seen := make(map[string]struct{})
	models := make([]Model, 0, len(modelCatalog)+len(extra))
	for _, entry := range modelCatalog {
		if entry.model.ID != "" {
			seen[entry.model.ID] = struct{}{}
			models = append(models, entry.model)
		}
	}
	for _, id := range extra {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; !exists {
			seen[trimmed] = struct{}{}
			models = append(models, catalogModel(trimmed, trimmed, 0))
		}
	}
	return models
}

// ModelOptions returns a deduplicated copy used by every model selector,
// optionally merging extra dynamic or custom models.
func ModelOptions(extra ...string) []string {
	seen := make(map[string]struct{})
	options := make([]string, 0, len(modelCatalog)+len(extra))
	for _, entry := range modelCatalog {
		for _, option := range entry.testOptions {
			if _, exists := seen[option]; exists {
				continue
			}
			seen[option] = struct{}{}
			options = append(options, option)
		}
	}
	for _, id := range extra {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; !exists {
			seen[trimmed] = struct{}{}
			options = append(options, trimmed)
		}
	}
	return options
}

// DefaultModelIDs returns the advertised model IDs.
func DefaultModelIDs() []string {
	models := GatewayModels()
	ids := make([]string, len(models))
	for index, model := range models {
		ids[index] = model.ID
	}
	return ids
}

// DynamicModel creates a Model struct for non-catalog models with sensible defaults.
func DynamicModel(id string) Model {
	idLower := strings.ToLower(strings.TrimSpace(id))
	ownedBy := "custom"
	displayName := id
	switch {
	case strings.HasPrefix(idLower, "deepseek"):
		ownedBy = "deepseek"
		if strings.Contains(idLower, "reasoner") {
			displayName = "DeepSeek Reasoner (R1)"
		} else if strings.Contains(idLower, "chat") {
			displayName = "DeepSeek Chat (V3)"
		}
	case strings.HasPrefix(idLower, "moonshot") || strings.HasPrefix(idLower, "kimi"):
		ownedBy = "moonshot"
	case strings.HasPrefix(idLower, "glm"):
		ownedBy = "zhipu"
	case strings.HasPrefix(idLower, "abab"):
		ownedBy = "minimax"
	case strings.HasPrefix(idLower, "gpt-") || strings.HasPrefix(idLower, "o1") || strings.HasPrefix(idLower, "o3") || strings.HasPrefix(idLower, "o4"):
		ownedBy = "openai"
	}
	return Model{
		ID:          id,
		Object:      "model",
		Created:     time.Now().Unix(),
		OwnedBy:     ownedBy,
		Type:        "model",
		DisplayName: displayName,
	}
}

// ActiveGatewayModels returns the model list for the gateway.
// If activeIDs is non-empty, only models from active accounts (and custom settings)
// are returned so that Codex Desktop dropdown directly reflects available pool accounts.
// If activeIDs is empty (e.g. initial setup), it falls back to the default GatewayModels().
func ActiveGatewayModels(activeIDs []string) []Model {
	if len(activeIDs) == 0 {
		return GatewayModels()
	}
	catalogMap := make(map[string]Model)
	for _, entry := range modelCatalog {
		if entry.model.ID != "" {
			catalogMap[entry.model.ID] = entry.model
		}
	}
	seen := make(map[string]struct{})
	models := make([]Model, 0, len(activeIDs))
	for _, id := range activeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		if entry, ok := catalogMap[id]; ok {
			models = append(models, entry)
		} else {
			models = append(models, DynamicModel(id))
		}
	}
	return models
}

// ActiveModelOptions returns the deduplicated options for UI model pickers.
// When activeIDs is non-empty, active account models are prioritized and merged.
func ActiveModelOptions(activeIDs []string) []string {
	if len(activeIDs) == 0 {
		return ModelOptions()
	}
	seen := make(map[string]struct{})
	var options []string
	add := func(m string) {
		m = strings.TrimSpace(m)
		if m == "" {
			return
		}
		if _, exists := seen[m]; !exists {
			seen[m] = struct{}{}
			options = append(options, m)
		}
	}
	for _, id := range activeIDs {
		add(id)
	}
	for _, id := range ModelOptions() {
		add(id)
	}
	return options
}
