package copilot

import "testing"

func TestCategorize_Event(t *testing.T) {
	cases := []string{
		"reunião às 15h com o time",
		"meeting with the team at 3pm",
		"lembrete: pagar aluguel",
		"reminder to check deploy",
		"deploy da versão 2.1 hoje",
		"amanhã tem standup",
		"pagou fatura do Inter R$ 3.483,68",
		"segunda-feira tem daily",
	}
	for _, c := range cases {
		if got := categorizeMemory(c); got != "event" {
			t.Errorf("categorizeMemory(%q) = %q, want 'event'", c, got)
		}
	}
}

func TestCategorize_Event_Time(t *testing.T) {
	cases := []string{
		"lembrete 14:30",
		"call às 6h30",
		"agenda 09:00 review",
	}
	for _, c := range cases {
		if got := categorizeMemory(c); got != "event" {
			t.Errorf("categorizeMemory(%q) = %q, want 'event'", c, got)
		}
	}
}

func TestCategorize_Summary(t *testing.T) {
	cases := []string{
		"resumo do dia de trabalho",
		"daily summary of tasks completed",
		"compacted session overview",
		"relatório semanal de atividades",
		"consolidated from 5 entries",
	}
	for _, c := range cases {
		if got := categorizeMemory(c); got != "summary" {
			t.Errorf("categorizeMemory(%q) = %q, want 'summary'", c, got)
		}
	}
}

func TestCategorize_Preference(t *testing.T) {
	cases := []string{
		"prefere dark mode",
		"user prefers concise responses",
		"gosta de café sem açúcar",
		"likes using vim keybindings",
		"nunca usa tabs, sempre espaços",
		"favorito: VS Code",
	}
	for _, c := range cases {
		if got := categorizeMemory(c); got != "preference" {
			t.Errorf("categorizeMemory(%q) = %q, want 'preference'", c, got)
		}
	}
}

func TestCategorize_Fact(t *testing.T) {
	cases := []string{
		"user works at HostGator",
		"Jhol mora em Maceió/AL",
		"SSH key stored in vault as server_key",
		"IntegraClaw uses DevClaw as base",
		"nome completo: Jhol Hewres",
	}
	for _, c := range cases {
		if got := categorizeMemory(c); got != "fact" {
			t.Errorf("categorizeMemory(%q) = %q, want 'fact'", c, got)
		}
	}
}

func TestCategorize_Default(t *testing.T) {
	if got := categorizeMemory("random text with no patterns"); got != "fact" {
		t.Errorf("default should be 'fact', got %q", got)
	}
}
