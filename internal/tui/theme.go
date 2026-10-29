package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/eusener/kanivet-cli/internal/detect"
)

// Theme agrupa os estilos e a escolha de ícones da TUI.
type Theme struct {
	Title, Subtle, Accent, Selected, Danger, Warn, OK, Border, Help lipgloss.Style
	Nerd, Color                                                     bool
}

var (
	colAccent = lipgloss.AdaptiveColor{Light: "#5A4FCF", Dark: "#A79BFF"}
	colOK     = lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#7BD88F"}
	colWarn   = lipgloss.AdaptiveColor{Light: "#B26A00", Dark: "#FFC56B"}
	colDanger = lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#FF7B72"}
	colSubtle = lipgloss.AdaptiveColor{Light: "#6B6B76", Dark: "#8B8B99"}
	colBorder = lipgloss.AdaptiveColor{Light: "#D0D0DA", Dark: "#3A3A48"}
)

// NewTheme monta o tema. Com NO_COLOR não vazio em env, os estilos não têm cor.
func NewTheme(env map[string]string, nerd bool) Theme {
	t := Theme{Nerd: nerd, Color: env["NO_COLOR"] == ""}
	fg := func(c lipgloss.TerminalColor) lipgloss.Style {
		if !t.Color {
			return lipgloss.NewStyle()
		}
		return lipgloss.NewStyle().Foreground(c)
	}
	t.Title = lipgloss.NewStyle().Bold(true)
	if t.Color {
		t.Title = t.Title.Foreground(colAccent)
	}
	t.Subtle = fg(colSubtle)
	t.Accent = fg(colAccent)
	t.Selected = fg(colAccent).Bold(true)
	t.Danger = fg(colDanger)
	t.Warn = fg(colWarn)
	t.OK = fg(colOK)
	t.Border = fg(colBorder)
	t.Help = fg(colSubtle)
	return t
}

var (
	asciiIcons = map[detect.Status]string{
		detect.StatusInstalled: "[x]", detect.StatusOutdated: "[^]", detect.StatusExternal: "[e]",
		detect.StatusExternalOutdated: "[!]", detect.StatusAbsent: "[ ]", detect.StatusProvided: "[p]",
		detect.StatusUnsupported: "[-]", detect.StatusUnknown: "[?]",
	}
	nerdIcons = map[detect.Status]string{
		detect.StatusInstalled:        "", // check
		detect.StatusOutdated:         "", // arrow-up
		detect.StatusExternal:         "", // external-link
		detect.StatusExternalOutdated: "", // exclamation-triangle
		detect.StatusAbsent:           "", // circle-o
		detect.StatusProvided:         "", // link
		detect.StatusUnsupported:      "", // ban
		detect.StatusUnknown:          "", // question
	}
)

// StatusIcon devolve o ícone do status (Nerd Font ou ASCII).
func (t Theme) StatusIcon(s detect.Status) string {
	m := asciiIcons
	if t.Nerd {
		m = nerdIcons
	}
	if v, ok := m[s]; ok {
		return v
	}
	return m[detect.StatusUnknown]
}

// Check devolve o marcador de seleção: caixa ou, com radio, rádio.
func (t Theme) Check(marked, radio bool) string {
	switch {
	case t.Nerd && radio && marked:
		return ""
	case t.Nerd && radio:
		return ""
	case t.Nerd && marked:
		return ""
	case t.Nerd:
		return ""
	case radio && marked:
		return "(•)"
	case radio:
		return "( )"
	case marked:
		return "[x]"
	}
	return "[ ]"
}

// HasNerdFont diz se a JetBrainsMono Nerd Font está presente.
func HasNerdFont(st map[string]detect.ItemStatus) bool {
	return st["nerd-font-jetbrains"].Status.Present()
}
