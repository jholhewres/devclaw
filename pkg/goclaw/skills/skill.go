// Package skills define o sistema de extensibilidade do AgentGo Copilot.
// Skills são módulos que adicionam capacidades ao assistente, como integração
// com calendário, email, GitHub, etc. Podem ser built-in ou instaladas pela comunidade.
package skills

import (
	"context"
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

	// Shutdown libera recursos da skill de forma graciosa.
	Shutdown() error
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
