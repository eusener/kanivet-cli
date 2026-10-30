package tui

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/muesli/termenv"
)

func plainThemes() map[string]Theme {
	return map[string]Theme{
		"no_color": NewTheme(map[string]string{"NO_COLOR": "1"}, false),
		"ansi16":   themeWithProfile(termenv.ANSI, nil),
	}
}

func TestBoxFootprint(t *testing.T) {
	out := paperTheme().Box("a\nb", 20, BoxOpts{})
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Fatalf("linhas: %d\n%s", len(lines), out)
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 20 {
			t.Errorf("linha %d: largura %d", i, w)
		}
	}
	if !strings.HasPrefix(stripANSI(lines[4]), "▝▀") {
		t.Errorf("última linha: %q", stripANSI(lines[4]))
	}
	for i := 1; i <= 3; i++ {
		if !strings.HasPrefix(stripANSI(lines[i]), "▐") {
			t.Errorf("linha %d sem sombra: %q", i, stripANSI(lines[i]))
		}
	}
	assertPainted(t, out, 20, 5)
}

func TestBoxTitleAndNoShadow(t *testing.T) {
	out := paperTheme().Box("a", 20, BoxOpts{Title: "Perfil", NoShadow: true})
	lines := strings.Split(out, "\n")
	if len(lines) != 3 || !strings.Contains(stripANSI(lines[0]), "PERFIL") {
		t.Fatalf("got:\n%s", stripANSI(out))
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 20 {
			t.Errorf("linha %d: largura %d", i, w)
		}
	}
}

func TestBoxNoShadowPlain(t *testing.T) {
	for name, th := range plainThemes() {
		out := th.Box("a\nb", 20, BoxOpts{})
		if strings.ContainsAny(out, "▐▀▝") {
			t.Errorf("%s: sombra no modo plain:\n%s", name, out)
		}
		if n := len(strings.Split(out, "\n")); n != 4 {
			t.Errorf("%s: linhas %d, esperado 4", name, n)
		}
		if strings.ContainsAny(out, "┏┃") || !strings.Contains(out, "┌") {
			t.Errorf("%s: borda deveria ser normal:\n%s", name, out)
		}
		if name == "no_color" && strings.Contains(out, "\x1b[") {
			t.Errorf("%s: plain com escapes: %q", name, out)
		}
	}
}

func TestTagUpper(t *testing.T) {
	if got := paperTheme().Tag("mise", colPeach); !strings.Contains(got, "MISE") {
		t.Errorf("got %q", got)
	}
	for name, th := range plainThemes() {
		if got := th.Tag("mise", colPeach); got != "[MISE]" {
			t.Errorf("%s: got %q", name, got)
		}
	}
}

func TestCanvasPainted(t *testing.T) {
	th := paperTheme()
	assertPainted(t, th.Canvas([]string{"x", th.Tag("a", colSun)}, 80, 24), 80, 24)
}

func TestCanvasRepaintsAfterReset(t *testing.T) {
	th := paperTheme()
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	seg := r.NewStyle().Foreground(colInk).Bold(true).Render("styled")
	if !strings.Contains(seg, "\x1b[0m") {
		t.Fatalf("segmento sem reset: %q", seg)
	}
	lines := []string{seg + " plain " + th.Tag("t", colSun) + " after", "\x1b[mraw \x1b[49mnobg"}
	assertPainted(t, th.Canvas(lines, 60, 3), 60, 3)
}

func TestCanvasCropsToWidth(t *testing.T) {
	th := paperTheme()
	assertPainted(t, th.Canvas([]string{strings.Repeat("x", 100), "a\nb\nc\nd"}, 10, 3), 10, 3)
}

func TestCanvasPlain(t *testing.T) {
	out := NewTheme(map[string]string{"NO_COLOR": "1"}, false).Canvas([]string{"ab"}, 5, 2)
	if out != "ab   \n     " {
		t.Errorf("got %q", out)
	}
}

func TestTrail(t *testing.T) {
	i18n.SetLang(i18n.PtBR)
	defer i18n.SetLang(i18n.En)
	th := paperTheme()
	got := th.Trail(2, false)
	if !strings.Contains(got, "ITENS") || strings.Count(got, "✓") != 2 {
		t.Errorf("Trail(2,false) = %q", stripANSI(got))
	}
	if got := th.Trail(4, true); strings.Count(got, "✓") != 5 {
		t.Errorf("Trail(4,true) = %q", stripANSI(got))
	}
}

func TestButtonAndTabs(t *testing.T) {
	th := paperTheme()
	if got := stripANSI(th.Button("enter", "instalar")); !strings.Contains(got, "ENTER") || !strings.Contains(got, "INSTALAR") {
		t.Errorf("button: %q", got)
	}
	tabs := th.Tabs([]string{"Shell", "Tools", "Editors"}, 1, 40)
	if lipgloss.Width(tabs) != 40 || !strings.Contains(stripANSI(tabs), "TOOLS") {
		t.Errorf("tabs: %q (%d)", stripANSI(tabs), lipgloss.Width(tabs))
	}
}

func TestCheckPaperGlyphs(t *testing.T) {
	th := paperTheme()
	for _, c := range []struct {
		m, r bool
		want string
	}{{true, false, "✓"}, {false, false, "○"}, {true, true, "●"}, {false, true, "○"}} {
		if got := th.Check(c.m, c.r); got != c.want {
			t.Errorf("Check(%v,%v)=%q want %q", c.m, c.r, got, c.want)
		}
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
