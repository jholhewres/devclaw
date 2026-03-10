// Package skills define o sistema de extensibilidade do DevClaw.
// Skills são módulos que adicionam capacidades ao assistente, como integração
// com calendário, email, GitHub, etc. Podem ser built-in ou instaladas pela comunidade.
package skills

import (
	"context"
	"os"
	"os/exec"
	"runtime"
)

// Skill define a interface que toda skill deve implementar.
// Uma skill encapsula uma capacidade específica do assistente.
type Skill interface {
	// Metadata retorna os metadados da skill (nome, versão, autor, etc).
	Metadata() Metadata

	// Tools retorna as funções/ferramentas que a skill expõe ao agente LLM.
	Tools() []Tool

	// SystemPrompt retorna instruções adicionais a serem injetadas no prompt do sistema
	// quando a skill está ativa. Pode retornar string vazia se não necessário.
	SystemPrompt() string

	// Triggers retorna padrões de linguagem natural que ativam esta skill.
	// Usado para ativação automática baseada no conteúdo da mensagem.
	Triggers() []string

	// Init inicializa a skill com a configuração fornecida.
	// Chamado uma vez durante o startup.
	Init(ctx context.Context, config map[string]any) error

	// Execute executa a skill com o input fornecido e retorna o resultado.
	Execute(ctx context.Context, input string) (string, error)

	// Location retorna o caminho absoluto do SKILL.md para skills baseadas em arquivo.
	// Retorna "" para skills built-in (que não possuem arquivo SKILL.md).
	// Usado pelo modelo de referência: a LLM lê o SKILL.md sob demanda via read_file.
	Location() string

	// Shutdown libera recursos da skill de forma graciosa.
	Shutdown() error
}

// ConfigRequirement descreve uma configuração/credencial obrigatória para uma skill.
// Skills podem declarar requisitos que serão verificados automaticamente antes da execução.
type ConfigRequirement struct {
	// Key é a chave usada para armazenar no vault (ex: "SLACK_BOT_TOKEN").
	Key string `yaml:"key"`

	// Name é o nome amigável para mostrar ao usuário (ex: "Slack Bot Token").
	Name string `yaml:"name"`

	// Description explica o que é e como obter essa configuração.
	Description string `yaml:"description"`

	// Pattern é um padrão opcional para validação (ex: "xoxb-*" para tokens Slack).
	Pattern string `yaml:"pattern,omitempty"`

	// Example mostra um exemplo de valor válido (ex: "xoxb-1234567890-1234567890-AbCdEf").
	Example string `yaml:"example,omitempty"`

	// Secret indica se o valor deve ser tratado como segredo (ocultar em logs).
	Secret bool `yaml:"secret"`

	// Required indica se é obrigatório. Se false, a skill pode funcionar sem ele.
	Required bool `yaml:"required"`

	// EnvVar é a variável de ambiente alternativa (ex: "SLACK_BOT_TOKEN").
	EnvVar string `yaml:"env_var,omitempty"`
}

// SetupStatus indica o estado de configuração de uma skill.
type SetupStatus struct {
	// IsComplete indica se todas as configurações obrigatórias estão presentes.
	IsComplete bool

	// MissingRequirements são as configurações que faltam.
	MissingRequirements []ConfigRequirement

	// OptionalMissing são configurações opcionais que não foram definidas.
	OptionalMissing []ConfigRequirement

	// Message é uma mensagem amigável sobre o estado do setup.
	Message string
}

// SkillSetupChecker é implementado por skills que precisam de configuração.
// O sistema verifica automaticamente antes de executar a skill.
type SkillSetupChecker interface {
	// RequiredConfig retorna as configurações obrigatórias e opcionais.
	RequiredConfig() []ConfigRequirement

	// CheckSetup verifica se a skill está corretamente configurada.
	// Recebe um VaultReader para verificar credenciais armazenadas.
	CheckSetup(vault VaultReader) SetupStatus
}

// VaultReader é uma interface minimalista para ler valores do vault.
// Skills usam isso para verificar se credenciais existem.
type VaultReader interface {
	// Get retorna o valor armazenado para a chave, ou erro se não existir.
	Get(key string) (string, error)
	// Has retorna true se a chave existe no vault.
	Has(key string) bool
}

// SourceTier indicates where a skill was loaded from.
// Higher tiers override lower tiers when skills have the same name.
type SourceTier int

const (
	// TierBundled is for skills bundled with DevClaw (lowest priority).
	TierBundled SourceTier = 0
	// TierManaged is for skills installed via the managed skill system.
	TierManaged SourceTier = 1
	// TierWorkspace is for skills defined in the workspace directory (highest priority).
	TierWorkspace SourceTier = 2
)

// String returns a human-readable tier name.
func (t SourceTier) String() string {
	switch t {
	case TierWorkspace:
		return "workspace"
	case TierManaged:
		return "managed"
	default:
		return "bundled"
	}
}

// SkillsLimitsConfig configures resource limits for skill loading.
type SkillsLimitsConfig struct {
	// MaxCandidatesPerRoot is the maximum skills scanned per workspace root (default: 300).
	MaxCandidatesPerRoot int `yaml:"max_candidates_per_root"`
	// MaxSkillsInPrompt is the maximum skills included in the system prompt (default: 150).
	MaxSkillsInPrompt int `yaml:"max_skills_in_prompt"`
	// MaxSkillsPromptChars is the total character budget for skills in the prompt (default: 30000).
	MaxSkillsPromptChars int `yaml:"max_skills_prompt_chars"`
	// MaxSkillFileBytes is the maximum size of a single skill file (default: 256000).
	MaxSkillFileBytes int `yaml:"max_skill_file_bytes"`
}

// DefaultSkillsLimits returns sensible defaults for skill loading limits.
func DefaultSkillsLimits() SkillsLimitsConfig {
	return SkillsLimitsConfig{
		MaxCandidatesPerRoot: 300,
		MaxSkillsInPrompt:    150,
		MaxSkillsPromptChars: 30000,
		MaxSkillFileBytes:    256000,
	}
}

// Effective returns a copy with default values filled in for zero fields.
func (c SkillsLimitsConfig) Effective() SkillsLimitsConfig {
	out := c
	if out.MaxCandidatesPerRoot <= 0 {
		out.MaxCandidatesPerRoot = 300
	}
	if out.MaxSkillsInPrompt <= 0 {
		out.MaxSkillsInPrompt = 150
	}
	if out.MaxSkillsPromptChars <= 0 {
		out.MaxSkillsPromptChars = 30000
	}
	if out.MaxSkillFileBytes <= 0 {
		out.MaxSkillFileBytes = 256000
	}
	return out
}

// Metadata contém os metadados de uma skill.
type Metadata struct {
	// Name é o identificador único da skill (ex: "calendar", "github").
	Name string `yaml:"name"`

	// Version é a versão semântica da skill (ex: "1.0.0").
	Version string `yaml:"version"`

	// Author é o autor ou organização que criou a skill.
	Author string `yaml:"author"`

	// Description é uma breve descrição do que a skill faz.
	Description string `yaml:"description"`

	// Category é a categoria da skill (ex: "productivity", "development").
	Category string `yaml:"category"`

	// Tags são palavras-chave para busca e indexação.
	Tags []string `yaml:"tags"`

	// Requires declares runtime eligibility requirements. Skills that don't
	// meet these requirements are excluded from the prompt to avoid confusing
	// the LLM with unavailable capabilities.
	Requires SkillRequirements `yaml:"requires" json:"requires,omitempty"`

	// SourceTier indicates the load priority tier of this skill.
	// Higher tiers override lower tiers with the same name.
	SourceTier SourceTier `yaml:"-" json:"-"`
}

// SkillRequirements declares runtime eligibility for a skill.
type SkillRequirements struct {
	// Bins lists binaries that must ALL exist in PATH.
	Bins []string `yaml:"bins" json:"bins,omitempty"`

	// AnyBins lists binaries where at least one must exist in PATH.
	AnyBins []string `yaml:"any_bins" json:"anyBins,omitempty"`

	// Env lists environment variables that must all be set (non-empty).
	Env []string `yaml:"env" json:"env,omitempty"`

	// OS lists compatible operating systems (runtime.GOOS values).
	// Empty means all OSes are supported.
	OS []string `yaml:"os" json:"os,omitempty"`
}

// osAliases maps non-Go OS names to runtime.GOOS values. ClawHub/OpenClaw
// skills use "win32" while Go uses "windows".
var osAliases = map[string]string{
	"win32": "windows",
}

// IsEligible checks whether the current runtime satisfies all requirements.
// Returns true when no requirements are declared or all checks pass.
func (r SkillRequirements) IsEligible() bool {
	if len(r.OS) > 0 {
		found := false
		goos := runtime.GOOS
		for _, o := range r.OS {
			normalized := o
			if alias, ok := osAliases[o]; ok {
				normalized = alias
			}
			if normalized == goos {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, bin := range r.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			return false
		}
	}
	if len(r.AnyBins) > 0 {
		anyFound := false
		for _, bin := range r.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				anyFound = true
				break
			}
		}
		if !anyFound {
			return false
		}
	}
	for _, env := range r.Env {
		if os.Getenv(env) == "" {
			return false
		}
	}
	return true
}

// Tool representa uma função/ferramenta exposta por uma skill ao agente LLM.
type Tool struct {
	// Name é o identificador da ferramenta (ex: "list_events").
	Name string `json:"name"`

	// Description descreve o que a ferramenta faz (usado no prompt do LLM).
	Description string `json:"description"`

	// Parameters define os parâmetros aceitos pela ferramenta.
	Parameters []ToolParameter `json:"parameters"`

	// Handler é a função que executa a ferramenta.
	Handler ToolHandler `json:"-"`
}

// ToolParameter define um parâmetro de uma ferramenta.
type ToolParameter struct {
	// Name é o nome do parâmetro.
	Name string `json:"name"`

	// Type é o tipo do parâmetro (string, integer, boolean, etc).
	Type string `json:"type"`

	// Description descreve o parâmetro.
	Description string `json:"description"`

	// Required indica se o parâmetro é obrigatório.
	Required bool `json:"required"`

	// Default é o valor padrão, se houver.
	Default any `json:"default,omitempty"`
}

// ToolHandler é a assinatura da função que processa a chamada de uma ferramenta.
type ToolHandler func(ctx context.Context, args map[string]any) (any, error)

// SkillLoader define a interface para carregar skills de diferentes fontes
// (embedded, filesystem, registry remoto, etc).
type SkillLoader interface {
	// Load carrega e retorna skills a partir da fonte configurada.
	Load(ctx context.Context) ([]Skill, error)

	// Source retorna o identificador da fonte (ex: "builtin", "filesystem", "registry").
	Source() string
}
