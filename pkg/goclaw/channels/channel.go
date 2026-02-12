// Package channels define as interfaces e tipos para canais de comunicação
// do AgentGo Copilot. Cada canal (WhatsApp, Discord, Telegram) implementa
// a interface Channel para receber e enviar mensagens de forma unificada.
package channels

import (
	"context"
	"fmt"
	"time"
)

// Channel define a interface genérica que todo canal de comunicação deve implementar.
// Canais são responsáveis por conectar, enviar e receber mensagens de plataformas externas.
type Channel interface {
	// Name retorna o identificador do canal (ex: "whatsapp", "discord", "telegram").
	Name() string

	// Connect estabelece a conexão com a plataforma de mensagens.
	// Deve ser chamado antes de Send/Receive. Retorna erro se a conexão falhar.
	Connect(ctx context.Context) error

	// Disconnect encerra a conexão de forma graciosa, finalizando goroutines e liberando recursos.
	Disconnect() error

	// Send envia uma mensagem para o destinatário especificado.
	Send(ctx context.Context, to string, message *OutgoingMessage) error

	// Receive retorna um channel Go que emite mensagens recebidas.
	// O channel é fechado quando Disconnect é chamado.
	Receive() <-chan *IncomingMessage

	// IsConnected retorna true se o canal está conectado e funcional.
	IsConnected() bool

	// Health retorna o estado de saúde do canal para monitoramento.
	Health() HealthStatus
}

// IncomingMessage representa uma mensagem recebida de qualquer canal.
type IncomingMessage struct {
	// ID é o identificador único da mensagem no canal de origem.
	ID string

	// Channel identifica o canal de origem (ex: "whatsapp", "discord").
	Channel string

	// From é o identificador do remetente na plataforma.
	From string

	// ChatID é o identificador do grupo ou DM onde a mensagem foi enviada.
	ChatID string

	// Content é o conteúdo textual da mensagem.
	Content string

	// Timestamp é o momento em que a mensagem foi enviada.
	Timestamp time.Time

	// ReplyTo contém o ID da mensagem sendo respondida, se aplicável.
	ReplyTo string

	// Metadata contém dados adicionais específicos do canal.
	Metadata map[string]any
}

// OutgoingMessage representa uma mensagem a ser enviada por um canal.
type OutgoingMessage struct {
	// Content é o conteúdo textual da mensagem.
	Content string

	// ReplyTo contém o ID da mensagem a ser respondida, se aplicável.
	ReplyTo string

	// Metadata contém dados adicionais específicos do canal.
	Metadata map[string]any
}

// HealthStatus representa o estado de saúde de um canal.
type HealthStatus struct {
	// Connected indica se o canal está conectado.
	Connected bool

	// LastMessageAt é o timestamp da última mensagem processada.
	LastMessageAt time.Time

	// ErrorCount é o número de erros acumulados desde a última reconexão.
	ErrorCount int

	// Latency é a latência média de envio em milissegundos.
	LatencyMs int64

	// Details contém informações adicionais de diagnóstico.
	Details map[string]any
}

// ChannelConfig contém configurações comuns a todos os canais.
type ChannelConfig struct {
	// Enabled indica se o canal está habilitado.
	Enabled bool `mapstructure:"enabled"`

	// Trigger é a palavra-chave que ativa o copilot (ex: "@copilot").
	Trigger string `mapstructure:"trigger"`

	// MaxRetries é o número máximo de tentativas de reconexão.
	MaxRetries int `mapstructure:"max_retries"`

	// RetryBackoffMs é o intervalo base entre tentativas em milissegundos.
	RetryBackoffMs int `mapstructure:"retry_backoff_ms"`
}

// ErrChannelDisconnected indica que o canal não está conectado.
var ErrChannelDisconnected = fmt.Errorf("channel is not connected")

// ErrSendFailed indica falha no envio da mensagem.
var ErrSendFailed = fmt.Errorf("failed to send message")

// ErrConnectionFailed indica falha na conexão com o canal.
var ErrConnectionFailed = fmt.Errorf("failed to connect to channel")
