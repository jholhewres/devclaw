package commands

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
	"github.com/spf13/cobra"
)

// newChatCmd creates the `copilot chat` command for interactive CLI conversations.
func newChatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat [message]",
		Short: "Chat with the assistant via terminal",
		Long: `Start a conversation with the assistant directly in the terminal.
Pass a message as argument for a single response, or run without arguments
for an interactive REPL session.

The CLI chat uses the same agent loop, tools, and skills as WhatsApp.

Interactive features:
  ↑/↓ arrows  — navigate command history
  Ctrl+A/E    — jump to start/end of line
  Ctrl+W      — delete word backward
  Ctrl+U      — clear line
  Ctrl+R      — reverse history search
  Tab         — autocomplete commands

Examples:
  copilot chat "What time is it?"
  copilot chat                      # interactive mode`,
		Args: cobra.MaximumNArgs(1),
		RunE: runChat,
	}

	cmd.Flags().StringP("model", "m", "", "override the LLM model")
	return cmd
}

func runChat(cmd *cobra.Command, args []string) error {
	// ── Load config ──
	cfg, _, err := resolveConfig(cmd)
	if err != nil {
		return err
	}

	// Override model if flag is set.
	if model, _ := cmd.Flags().GetString("model"); model != "" {
		cfg.Model = model
	}

	// ── Configure logger (quiet for chat mode) ──
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	logLevel := slog.LevelWarn
	if verbose {
		logLevel = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	logger := slog.New(handler)

	// ── Resolve secrets ──
	copilot.AuditSecrets(cfg, logger)
	vault := copilot.ResolveAPIKey(cfg, logger)

	if cfg.API.APIKey == "" || copilot.IsEnvReference(cfg.API.APIKey) {
		return fmt.Errorf("no API key configured. Run: copilot config vault-set")
	}

	// ── Create and start assistant ──
	assistant := copilot.New(cfg, logger)
	if vault != nil {
		assistant.SetVault(vault)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := assistant.Start(ctx); err != nil {
		return fmt.Errorf("failed to start assistant: %w", err)
	}
	defer assistant.Stop()

	// ── Single message mode ──
	if len(args) > 0 {
		response := executeChat(assistant, args[0])
		fmt.Println(response)
		return nil
	}

	// ── Interactive REPL mode ──
	return runInteractiveChat(assistant, cfg)
}

// executeChat sends a message through the assistant and returns the response.
func executeChat(assistant *copilot.Assistant, message string) string {
	session := assistant.SessionStore().GetOrCreate("cli", "terminal")
	prompt := assistant.ComposePrompt(session, message)
	response := assistant.ExecuteAgent(context.Background(), prompt, session, message)
	session.AddMessage(message, response)
	return response
}

// chatCommands lists all available CLI commands for autocomplete.
var chatCommands = []string{
	"/quit", "/exit", "/q",
	"/clear", "/reset", "/new",
	"/tools", "/model", "/help",
	"/usage", "/compact", "/stop",
	"/think", "/history", "/export",
}

// chatCompleter provides tab-completion for commands and arguments.
func chatCompleter() *readline.PrefixCompleter {
	return readline.NewPrefixCompleter(
		readline.PcItem("/quit"),
		readline.PcItem("/exit"),
		readline.PcItem("/clear"),
		readline.PcItem("/reset"),
		readline.PcItem("/new"),
		readline.PcItem("/tools"),
		readline.PcItem("/model"),
		readline.PcItem("/help"),
		readline.PcItem("/usage",
			readline.PcItem("reset"),
			readline.PcItem("global"),
		),
		readline.PcItem("/compact"),
		readline.PcItem("/stop"),
		readline.PcItem("/think",
			readline.PcItem("off"),
			readline.PcItem("low"),
			readline.PcItem("medium"),
			readline.PcItem("high"),
		),
		readline.PcItem("/history"),
		readline.PcItem("/export"),
	)
}

// historyFile returns the path to the readline history file.
func historyFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".devclaw")
	_ = os.MkdirAll(dir, 0o700)
	return filepath.Join(dir, "chat_history")
}

// runInteractiveChat runs an interactive REPL chat with readline support.
func runInteractiveChat(assistant *copilot.Assistant, cfg *copilot.Config) error {
	// ── Initialize readline ──
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "\033[36myou>\033[0m ",
		HistoryFile:     historyFile(),
		HistoryLimit:    1000,
		AutoComplete:    chatCompleter(),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold: true,

		// Filter passwords and empty lines from history.
		DisableAutoSaveHistory: false,
	})
	if err != nil {
		// Fallback to basic stdin if readline fails (e.g., non-interactive terminal).
		return runBasicChat(assistant, cfg)
	}
	defer rl.Close()

	// ── Welcome message ──
	fmt.Println()
	fmt.Printf("  \033[1m%s\033[0m — Interactive CLI Chat\n", cfg.Name)
	fmt.Println("  ─────────────────────────────────")
	fmt.Println("  Type your message and press Enter.")
	fmt.Println()
	fmt.Println("  \033[2mKeyboard shortcuts:\033[0m")
	fmt.Println("    ↑/↓        Navigate history")
	fmt.Println("    Ctrl+R     Search history")
	fmt.Println("    Tab        Autocomplete commands")
	fmt.Println("    Ctrl+A/E   Jump to start/end")
	fmt.Println("    Ctrl+C     Cancel current input")
	fmt.Println("    Ctrl+D     Exit")
	fmt.Println()
	fmt.Println("  \033[2mCommands: /help, /quit, /tools, /model, /usage, /think\033[0m")
	fmt.Println()

	session := assistant.SessionStore().GetOrCreate("cli", "terminal")

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				// Ctrl+C: if line is empty, suggest /quit; otherwise clear line.
				continue
			}
			if err == io.EOF {
				// Ctrl+D.
				fmt.Println("\n  Bye!")
				return nil
			}
			return err
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// ── Handle CLI commands ──
		lower := strings.ToLower(input)
		parts := strings.Fields(lower)
		command := parts[0]

		switch command {
		case "/quit", "/exit", "/q":
			fmt.Println("  Bye!")
			return nil

		case "/clear", "/reset", "/new":
			session.ClearHistory()
			fmt.Println("  \033[33m[conversation cleared]\033[0m")
			fmt.Println()
			continue

		case "/tools":
			tools := assistant.ToolExecutor().ToolNames()
			fmt.Printf("  \033[1m%d tools available:\033[0m\n", len(tools))
			for i, t := range tools {
				fmt.Printf("    \033[36m%2d.\033[0m %s\n", i+1, t)
			}
			fmt.Println()
			continue

		case "/model":
			if len(parts) > 1 {
				newModel := parts[1]
				scfg := session.GetConfig()
				scfg.Model = newModel
				session.SetConfig(scfg)
				fmt.Printf("  \033[32mModel changed to: %s\033[0m\n\n", newModel)
			} else {
				effectiveModel := cfg.Model
				scfg := session.GetConfig()
				if scfg.Model != "" {
					effectiveModel = scfg.Model
				}
				fmt.Printf("  Model:    \033[1m%s\033[0m\n", effectiveModel)
				fmt.Printf("  API:      %s\n", cfg.API.BaseURL)
				fmt.Printf("  Fallback: %v\n", cfg.Fallback.Models)
				fmt.Println()
			}
			continue

		case "/usage":
			pu, cu, reqs := session.GetTokenUsage()
			total := pu + cu
			fmt.Printf("  \033[1mSession Usage:\033[0m\n")
			fmt.Printf("    Prompt tokens:     %d\n", pu)
			fmt.Printf("    Completion tokens: %d\n", cu)
			fmt.Printf("    Total tokens:      %d\n", total)
			fmt.Printf("    Requests:          %d\n", reqs)
			fmt.Printf("    History entries:    %d\n", session.HistoryLen())
			fmt.Println()
			continue

		case "/compact":
			before := session.HistoryLen()
			if before < 5 {
				fmt.Println("  \033[33mHistory too short to compact.\033[0m")
				fmt.Println()
				continue
			}
			summary := "Session compacted via CLI."
			session.CompactHistory(summary, before/4)
			fmt.Printf("  \033[32mCompacted: %d → %d entries\033[0m\n\n", before, session.HistoryLen())
			continue

		case "/think":
			if len(parts) > 1 {
				level := parts[1]
				switch level {
				case "off", "low", "medium", "high":
					session.SetThinkingLevel(level)
					fmt.Printf("  \033[32mThinking level: %s\033[0m\n\n", level)
				default:
					fmt.Println("  \033[31mInvalid level. Use: off, low, medium, high\033[0m")
					fmt.Println()
				}
			} else {
				level := session.GetThinkingLevel()
				if level == "" {
					level = "default"
				}
				fmt.Printf("  Thinking level: \033[1m%s\033[0m\n\n", level)
			}
			continue

		case "/history":
			entries := session.RecentHistory(20)
			if len(entries) == 0 {
				fmt.Println("  \033[33mNo history.\033[0m")
				fmt.Println()
				continue
			}
			fmt.Printf("  \033[1mRecent history (%d entries):\033[0m\n", len(entries))
			for i, e := range entries {
				userPreview := e.UserMessage
				if len(userPreview) > 60 {
					userPreview = userPreview[:60] + "..."
				}
				assistPreview := e.AssistantResponse
				if len(assistPreview) > 60 {
					assistPreview = assistPreview[:60] + "..."
				}
				fmt.Printf("    \033[36m%2d.\033[0m \033[2m%s\033[0m\n", i+1, e.Timestamp.Format("15:04:05"))
				fmt.Printf("        you> %s\n", userPreview)
				fmt.Printf("        bot> %s\n", assistPreview)
			}
			fmt.Println()
			continue

		case "/export":
			entries := session.RecentHistory(1000)
			if len(entries) == 0 {
				fmt.Println("  \033[33mNo history to export.\033[0m")
				fmt.Println()
				continue
			}
			exportPath := filepath.Join(".", "chat_export.md")
			f, err := os.Create(exportPath)
			if err != nil {
				fmt.Printf("  \033[31mFailed to create file: %v\033[0m\n\n", err)
				continue
			}
			fmt.Fprintf(f, "# Chat Export\n\n")
			for _, e := range entries {
				fmt.Fprintf(f, "## %s\n\n", e.Timestamp.Format("2006-01-02 15:04:05"))
				fmt.Fprintf(f, "**You:** %s\n\n", e.UserMessage)
				fmt.Fprintf(f, "**Assistant:** %s\n\n---\n\n", e.AssistantResponse)
			}
			f.Close()
			fmt.Printf("  \033[32mExported %d entries to %s\033[0m\n\n", len(entries), exportPath)
			continue

		case "/help":
			printHelp()
			continue
		}

		// ── Spinner / thinking indicator ──
		fmt.Print("  \033[2mthinking...\033[0m")

		// ── Send to the agent ──
		prompt := assistant.ComposePrompt(session, input)
		response := assistant.ExecuteAgent(context.Background(), prompt, session, input)
		session.AddMessage(input, response)

		// Clear the "thinking..." line.
		fmt.Print("\r\033[K")

		// ── Print response with formatting ──
		fmt.Println()
		fmt.Printf("\033[32m%s>\033[0m %s\n", cfg.Name, response)
		fmt.Println()
	}
}

// printHelp displays all available CLI commands.
func printHelp() {
	fmt.Println()
	fmt.Println("  \033[1mAvailable Commands:\033[0m")
	fmt.Println("  ─────────────────")
	fmt.Println("  \033[36m/help\033[0m          Show this help")
	fmt.Println("  \033[36m/quit\033[0m          Exit (/exit, /q)")
	fmt.Println("  \033[36m/clear\033[0m         Clear conversation (/reset, /new)")
	fmt.Println("  \033[36m/tools\033[0m         List available tools")
	fmt.Println("  \033[36m/model\033[0m [name]  Show or change model")
	fmt.Println("  \033[36m/usage\033[0m         Show token usage stats")
	fmt.Println("  \033[36m/compact\033[0m       Compact session history")
	fmt.Println("  \033[36m/think\033[0m [level] Set thinking level (off/low/medium/high)")
	fmt.Println("  \033[36m/history\033[0m       Show recent conversation")
	fmt.Println("  \033[36m/export\033[0m        Export chat to Markdown file")
	fmt.Println()
	fmt.Println("  \033[1mKeyboard Shortcuts:\033[0m")
	fmt.Println("  ─────────────────")
	fmt.Println("  ↑/↓          Navigate input history")
	fmt.Println("  Ctrl+R       Reverse search history")
	fmt.Println("  Tab          Autocomplete commands")
	fmt.Println("  Ctrl+A       Jump to start of line")
	fmt.Println("  Ctrl+E       Jump to end of line")
	fmt.Println("  Ctrl+W       Delete word backward")
	fmt.Println("  Ctrl+U       Clear entire line")
	fmt.Println("  Ctrl+L       Clear screen")
	fmt.Println("  Ctrl+C       Cancel current input")
	fmt.Println("  Ctrl+D       Exit")
	fmt.Println()
}

// runBasicChat is a fallback for non-interactive terminals (no readline).
func runBasicChat(assistant *copilot.Assistant, cfg *copilot.Config) error {
	fmt.Println()
	fmt.Printf("  %s — CLI Chat (basic mode)\n", cfg.Name)
	fmt.Println("  Type /quit to exit, /help for commands.")
	fmt.Println()

	session := assistant.SessionStore().GetOrCreate("cli", "terminal")

	scanner := readline.NewCancelableStdin(os.Stdin)
	defer scanner.Close()

	buf := make([]byte, 4096)
	for {
		fmt.Print("you> ")
		n, err := scanner.Read(buf)
		if err != nil {
			fmt.Println()
			return nil
		}

		input := strings.TrimSpace(string(buf[:n]))
		if input == "" {
			continue
		}

		if strings.ToLower(input) == "/quit" || strings.ToLower(input) == "/exit" {
			fmt.Println("Bye!")
			return nil
		}

		prompt := assistant.ComposePrompt(session, input)
		response := assistant.ExecuteAgent(context.Background(), prompt, session, input)
		session.AddMessage(input, response)

		fmt.Printf("\n%s> %s\n\n", cfg.Name, response)
	}
}
