// Package skills – commands.go implements slash command routing for skills.
// When a user sends a message starting with /command, it is routed to the
// matching skill's Execute() method instead of the general LLM pipeline.
//
// Usage:
//
//	/weather São Paulo  →  routes to skill "weather" with args "São Paulo"
//	/deploy staging     →  routes to skill "deploy" with args "staging"
//
// Reserved command names (cannot be overridden by skills):
//
//	/help, /remember, /forget, /skills, /status, /reset, /model, /stop
package skills

import (
	"context"
	"fmt"
	"strings"
)

// reservedCommands are command names that cannot be overridden by skills.
// They are handled by the core assistant.
var reservedCommands = map[string]bool{
	"help": true, "remember": true, "forget": true,
	"skills": true, "status": true, "reset": true,
	"model": true, "stop": true, "admin": true,
	"config": true, "clear": true,
}

// SlashCommandFlags controls how a skill slash command behaves.
type SlashCommandFlags struct {
	// UserInvocable means the command can only be triggered by the user
	// typing /command, not by the LLM calling it autonomously.
	UserInvocable bool `yaml:"user_invocable"`

	// DisableModelInvocation prevents the LLM from calling this skill's
	// Execute() method directly. Only explicit /command invocation works.
	DisableModelInvocation bool `yaml:"disable_model_invocation"`
}

// ParseSlashCommand checks if the input starts with /command and returns
// the command name and arguments. Returns empty strings if not a command.
func ParseSlashCommand(input string) (command, args string) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") || len(input) < 2 {
		return "", ""
	}

	// Remove the leading slash.
	rest := input[1:]

	// Split into command and args.
	parts := strings.SplitN(rest, " ", 2)
	command = strings.ToLower(parts[0])
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	return command, args
}

// IsReservedCommand returns true if the command name is reserved.
func IsReservedCommand(name string) bool {
	return reservedCommands[strings.ToLower(name)]
}

// CommandRouter routes slash commands to matching skills.
type CommandRouter struct {
	registry *Registry
}

// NewCommandRouter creates a new slash command router.
func NewCommandRouter(registry *Registry) *CommandRouter {
	return &CommandRouter{registry: registry}
}

// Route attempts to route a slash command to a matching skill.
// Returns the skill and the args if found, nil otherwise.
func (cr *CommandRouter) Route(command string) (Skill, bool) {
	// Check if the command matches a registered skill name.
	skill, ok := cr.registry.Get(command)
	if !ok {
		return nil, false
	}

	// Check if it's a reserved command (handled elsewhere).
	if IsReservedCommand(command) {
		return nil, false
	}

	return skill, true
}

// Execute routes a slash command to the matching skill and executes it.
// Returns the result or an error.
func (cr *CommandRouter) Execute(ctx context.Context, command, args string) (string, error) {
	skill, ok := cr.Route(command)
	if !ok {
		return "", fmt.Errorf("unknown command /%s", command)
	}

	result, err := skill.Execute(ctx, args)
	if err != nil {
		return "", fmt.Errorf("command /%s failed: %w", command, err)
	}
	return result, nil
}

// ListCommands returns a list of available slash commands from registered skills.
func (cr *CommandRouter) ListCommands() []string {
	skills := cr.registry.List()
	var commands []string
	for _, meta := range skills {
		if !IsReservedCommand(meta.Name) {
			commands = append(commands, "/"+meta.Name)
		}
	}
	return commands
}
