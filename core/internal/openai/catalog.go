package openai

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

// GatewayModels returns a copy of the models advertised by /v1/models.
func GatewayModels() []Model {
	models := make([]Model, 0, len(modelCatalog))
	for _, entry := range modelCatalog {
		if entry.model.ID != "" {
			models = append(models, entry.model)
		}
	}
	return models
}

// ModelOptions returns a deduplicated copy used by every model selector.
func ModelOptions() []string {
	seen := make(map[string]struct{})
	options := make([]string, 0, len(modelCatalog))
	for _, entry := range modelCatalog {
		for _, option := range entry.testOptions {
			if _, exists := seen[option]; exists {
				continue
			}
			seen[option] = struct{}{}
			options = append(options, option)
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
