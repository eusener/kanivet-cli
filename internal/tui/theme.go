package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/state"
	"github.com/muesli/termenv"
)

// Paleta "papel e tinta".
const (
	colPaper     = lipgloss.Color("#f4efea")
	colCard      = lipgloss.Color("#ffffff")
	colInk       = lipgloss.Color("#383838")
	colInkStrong = lipgloss.Color("#111110")
	colInkMuted  = lipgloss.Color("#666666")
	colSky       = lipgloss.Color("#97d4ff")
	colSun       = lipgloss.Color("#ffde00")
	colPeach     = lipgloss.Color("#ffdcbd")
	colMint      = lipgloss.Color("#b0fff4")
	colCoral     = lipgloss.Color("#ffcbc8")
	colOK        = lipgloss.Color("#068475")
	colErr       = lipgloss.Color("#e23f35")
	colLogBg     = lipgloss.Color("#2b2a29")
	colLogFg     = lipgloss.Color("#e8e2dc")
)

// Theme agrupa os estilos e a escolha de ícones da TUI.
type Theme struct {
	Title, Subtle, Accent, Selected, Danger, Warn, OK, Border, Help lipgloss.Style
	Nerd, Color                                                     bool
	// Paper é true quando há cor e o terminal tem 256 cores ou mais: só então
	// a tela é pintada de creme, com sombras e bordas grossas. Caso contrário
	// (16 cores, NO_COLOR) o tema é "plain".
	Paper bool
	R     *lipgloss.Renderer
}

// NewTheme monta o tema com o renderer padrão do lipgloss.
func NewTheme(env map[string]string, nerd bool) Theme {
	return NewThemeWith(lipgloss.DefaultRenderer(), env, nerd)
}

// NewThemeWith monta o tema para o renderer dado. Com NO_COLOR não vazio em
// env, os estilos não têm cor.
func NewThemeWith(r *lipgloss.Renderer, env map[string]string, nerd bool) Theme {
	t := Theme{Nerd: nerd, Color: env["NO_COLOR"] == "", R: r}
	p := r.ColorProfile()
	t.Paper = t.Color && (p == termenv.TrueColor || p == termenv.ANSI256)
	fg := func(c lipgloss.TerminalColor) lipgloss.Style {
		if !t.Color {
			return t.S()
		}
		return t.S().Foreground(c)
	}
	t.Title = fg(colInkStrong).Bold(true)
	t.Subtle = fg(colInkMuted)
	t.Accent = fg(colInkStrong)
	t.Selected = fg(colInkStrong).Bold(true)
	if t.Paper {
		t.Selected = t.Selected.Background(colSky)
	} else {
		t.Selected = t.Selected.Reverse(true)
	}
	t.Danger = fg(colErr)
	t.Warn = fg(colInk).Bold(true)
	t.OK = fg(colOK)
	t.Border = fg(colInk)
	t.Help = fg(colInkMuted)
	return t
}

// S devolve um estilo novo do renderer, com o fundo creme quando Paper.
func (t Theme) S() lipgloss.Style {
	s := t.R.NewStyle()
	if t.Paper {
		s = s.Background(colPaper)
	}
	return s
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

// StepIcon devolve o ícone do passo na execução: ✓/✗/↷ ou, sem Nerd Font, [ok]/[x]/[skip].
func (t Theme) StepIcon(s state.StepStatus) string {
	switch s {
	case state.StepDone:
		return pick(t.Nerd, "✓", "[ok]")
	case state.StepFailed:
		return pick(t.Nerd, "✗", "[x]")
	case state.StepSkipped:
		return pick(t.Nerd, "↷", "[skip]")
	case state.StepRunning:
		return pick(t.Nerd, "…", "[..]")
	}
	return pick(t.Nerd, "·", "[ ]")
}

func pick(nerd bool, a, b string) string {
	if nerd {
		return a
	}
	return b
}

// Check devolve o marcador de seleção: ✓/○ (ou ●/○ com radio) ou, sem Nerd
// Font, [x]/[ ] (ou (•)/( )).
func (t Theme) Check(marked, radio bool) string {
	switch {
	case t.Nerd && radio && marked:
		return "●"
	case t.Nerd && marked:
		return "✓"
	case t.Nerd:
		return "○"
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
