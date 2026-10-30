package tui

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// paperTheme devolve um tema com cor verdadeira e Nerd Font, independente do terminal.
func paperTheme() Theme {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	return NewThemeWith(r, nil, true)
}

// themeWithProfile devolve um tema (com cor, sem Nerd Font) para o perfil dado.
func themeWithProfile(p termenv.Profile, env map[string]string) Theme {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(p)
	return NewThemeWith(r, env, false)
}

// assertPainted exige h linhas de largura w em que toda célula impressa tem fundo ativo.
func assertPainted(t *testing.T, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != h {
		t.Fatalf("linhas: got %d want %d", len(lines), h)
	}
	for i, ln := range lines {
		if got := lipgloss.Width(ln); got != w {
			t.Errorf("linha %d: largura %d, esperado %d: %q", i, got, w, ln)
		}
		if col, ok := firstUnpainted(ln); !ok {
			t.Errorf("linha %d: célula sem fundo (byte %d): %q", i, col, ln)
		}
	}
}

// firstUnpainted acompanha o fundo ativo pelas sequências SGR e devolve o byte
// da primeira célula impressa sem fundo; ok=true quando não há nenhuma.
func firstUnpainted(s string) (int, bool) {
	bg := false
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j >= len(s) {
				return i, false
			}
			if s[j] == 'm' {
				bg = applySGR(s[i+2:j], bg)
			}
			i = j + 1
			continue
		}
		if !bg {
			return i, false
		}
		i++
	}
	return 0, true
}

func applySGR(params string, bg bool) bool {
	if params == "" {
		return false
	}
	p := strings.Split(params, ";")
	for k := 0; k < len(p); k++ {
		n, _ := strconv.Atoi(p[k])
		switch {
		case n == 0 || n == 49:
			bg = false
		case n == 48:
			bg = true
			if k+1 < len(p) && p[k+1] == "2" {
				k += 4
			} else if k+1 < len(p) && p[k+1] == "5" {
				k += 2
			}
		case n == 38:
			if k+1 < len(p) && p[k+1] == "2" {
				k += 4
			} else if k+1 < len(p) && p[k+1] == "5" {
				k += 2
			}
		case n >= 40 && n <= 47, n >= 100 && n <= 107:
			bg = true
		}
	}
	return bg
}
