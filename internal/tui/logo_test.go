package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestLogoOnlyInPaper(t *testing.T) {
	logo := paperTheme().Logo()
	if len(logo) != 8 {
		t.Fatalf("símbolo com %d linhas, want 8", len(logo))
	}
	for i, l := range logo {
		if w := lipgloss.Width(l); w != logoWidth {
			t.Errorf("linha %d com largura %d, want %d: %q", i, w, logoWidth, ansi.Strip(l))
		}
	}
	if got := themeWithProfile(termenv.ANSI, nil).Logo(); got != nil {
		t.Fatalf("16 cores não deveria desenhar o símbolo: %q", got)
	}
}

func TestWelcomeShowsLogo(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 40}} {
		m := withTheme(detected(t, newModel(t, linuxSystemd)), paperTheme())
		m = send(m, size(sz[0], sz[1]))
		v := m.View()
		assertPainted(t, v, sz[0], sz[1])
		plain := ansi.Strip(v)
		if !strings.Contains(plain, "■ ■ ■") || !strings.Contains(plain, "YOUR DEV TERMINAL,") || !strings.Contains(plain, "SYSTEM") {
			t.Fatalf("%dx%d sem símbolo, manchete ou sistema:\n%s", sz[0], sz[1], plain)
		}
	}
}
