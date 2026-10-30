package tui

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/muesli/termenv"
)

// allScreens monta cada tela com dados representativos: execução em andamento,
// resumo com falhas e painel com atualizações. Cada tela tem a sua própria seleção
// (nenhuma herda as marcações de outra).
func allScreens(t *testing.T) map[string]Model {
	t.Helper()
	exec, _ := executing(t)
	sum, _ := summaryModel(t)
	welcome := detected(t, newModel(t, linuxSystemd))
	profile := send(detected(t, newModel(t, linuxSystemd)), enter())
	cats := send(detected(t, newModel(t, linuxSystemd)), enter(), enter())
	dash := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	dash = send(dash, updatesMsg{statuses: map[string]detect.ItemStatus{"bat": st("bat", detect.StatusOutdated)}, gen: dash.dash.gen})
	return map[string]Model{
		"welcome": welcome, "profile": profile, "categories": cats,
		"review": reviewFor(t, linuxSystemd), "execute": exec, "summary": sum, "dashboard": dash,
	}
}

// withTheme troca o tema do modelo e remonta o que guarda uma cópia dele (as listas do
// wizard e do painel), para que a tela inteira seja desenhada no tema dado.
func withTheme(m Model, th Theme) Model {
	m.th = th
	if m.sel != nil {
		if m.screen == ScreenCategories {
			m.list = m.newList()
		}
		if m.screen == ScreenDashboard {
			m.dash.list = m.catList()
			if m.dash.global != nil {
				g := *m.dash.global
				g.th = th
				m.dash.global = &g
			}
		}
	}
	if m.screen == ScreenReview {
		m = m.layoutReview()
	}
	return m
}

func checkScreens(t *testing.T, w, h int) {
	for name, m := range allScreens(t) {
		m = withTheme(m, paperTheme())
		m = send(m, size(w, h))
		t.Run(name, func(t *testing.T) {
			v := m.View()
			assertPainted(t, v, w, h)
			assertInked(t, v)
			plain := ansi.Strip(v)
			if !strings.Contains(plain, "KANIVET") {
				t.Errorf("sem KANIVET:\n%s", plain)
			}
			if _, label, ok := m.primaryAction(); ok && !strings.Contains(strings.ToUpper(plain), strings.ToUpper(label)) {
				t.Errorf("sem o botão %q:\n%s", label, plain)
			}
			// I7: as listas (categorias, painel) também são desenhadas no tema Paper
			if strings.ContainsAny(plain, "┌└") {
				t.Errorf("caixa com borda fina (tema fora do Paper):\n%s", plain)
			}
			if name == "categories" || name == "dashboard" {
				if n := strings.Count(plain, "┏"); n < 3 {
					t.Errorf("esperava cabeçalho + 2 caixas Paper, achei %d:\n%s", n, plain)
				}
			}
			lines := strings.Split(plain, "\n")
			if name == "welcome" && !strings.Contains(lines[len(lines)-1], "0 TO INSTALL") {
				t.Errorf("a boas-vindas não deve herdar a seleção das outras telas: %q", lines[len(lines)-1])
			}
			last := ""
			for _, l := range lines {
				if strings.TrimSpace(l) != "" {
					last = l
				}
			}
			if last != lines[len(lines)-1] || strings.TrimSpace(last) == "" {
				t.Errorf("a última linha deve ser o rodapé, não vazia:\n%s", plain)
			}
		})
	}
}

func TestAllScreensAt80x24(t *testing.T)  { checkScreens(t, 80, 24) }
func TestAllScreensAt120x40(t *testing.T) { checkScreens(t, 120, 40) }

var sgrSeq = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// hasBackground diz se alguma SGR da string liga fundo (40-47, 100-107 ou 48),
// lendo a lista de parâmetros e pulando os argumentos de 38/48.
func hasBackground(s string) bool {
	for _, mt := range sgrSeq.FindAllStringSubmatch(s, -1) {
		p := strings.Split(mt[1], ";")
		for k := 0; k < len(p); k++ {
			n, _ := strconv.Atoi(p[k])
			switch {
			case n == 48, n >= 40 && n <= 47, n >= 100 && n <= 107:
				return true
			case n == 38 || n == 58:
				if k+1 < len(p) && p[k+1] == "2" {
					k += 4
				} else if k+1 < len(p) && p[k+1] == "5" {
					k += 2
				}
			}
		}
	}
	return false
}

func TestHasBackgroundDetector(t *testing.T) {
	for _, s := range []string{"\x1b[44m", "\x1b[1;44m", "\x1b[7;44m", "\x1b[100m", "\x1b[48;5;7m", "\x1b[48;2;1;2;3m", "\x1b[38;5;1;48;5;2m", "\x1b[1;38;2;1;2;3;48;2;4;5;6m"} {
		if !hasBackground(s) {
			t.Errorf("não detectou fundo em %q", s)
		}
	}
	for _, s := range []string{"\x1b[1m", "\x1b[7;31m", "\x1b[38;5;44m", "\x1b[38;2;40;44;47m", "\x1b[0m", "texto"} {
		if hasBackground(s) {
			t.Errorf("falso positivo em %q", s)
		}
	}
}

func TestPlainModeHasNoBackground(t *testing.T) {
	themes := map[string]Theme{
		"no_color": themeWithProfile(termenv.TrueColor, map[string]string{"NO_COLOR": "1"}),
		"ansi16":   themeWithProfile(termenv.ANSI, nil),
	}
	for tn, th := range themes {
		for name, m := range allScreens(t) {
			m = withTheme(m, th)
			m = send(m, size(80, 24))
			for _, extra := range []string{"", "?"} {
				if extra == "?" {
					if name == "execute" {
						continue
					}
					m = send(m, keyRune("?"))
				}
				t.Run(tn+"/"+name+extra, func(t *testing.T) {
					v := m.View()
					if hasBackground(v) || strings.ContainsAny(v, "▐▀▝") {
						t.Errorf("modo plain com fundo ou sombra:\n%q", v)
					}
				})
			}
		}
	}
}

func TestHelpOverlayPainted(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 40}} {
		for name, m := range allScreens(t) {
			if name == "execute" { // q e ? não valem no meio do run
				continue
			}
			m = withTheme(m, paperTheme())
			m = send(m, size(sz[0], sz[1]), keyRune("?"))
			t.Run(name, func(t *testing.T) {
				v := m.View()
				assertPainted(t, v, sz[0], sz[1])
				assertInked(t, v)
				plain := ansi.Strip(v)
				if !strings.Contains(strings.ToUpper(plain), "SHORTCUTS") || !strings.Contains(plain, "KANIVET") {
					t.Errorf("ajuda sem título:\n%s", plain)
				}
				if strings.Count(plain, "┏") < 2 { // cabeçalho + caixa da ajuda
					t.Errorf("a ajuda deve ser uma caixa:\n%s", plain)
				}
			})
		}
	}
}

func TestQuitConfirmOverlayPainted(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.En, i18n.PtBR} {
		i18n.SetLang(lang)
		t.Cleanup(func() { i18n.SetLang(i18n.En) })
		m := send(detected(t, newModel(t, linuxSystemd)), enter(), enter(), size(80, 24), keyRune("q"))
		m.th = paperTheme()
		if !m.confirmQuit {
			t.Fatal("q com pendências deveria pedir confirmação")
		}
		v := m.View()
		assertPainted(t, v, 80, 24)
		plain := ansi.Strip(v)
		if strings.Count(plain, "┏") < 2 || !strings.Contains(plain, "KANIVET") {
			t.Errorf("diálogo de saída incompleto:\n%s", plain)
		}
		flat := strings.NewReplacer("┃", " ", "┏", " ", "┓", " ", "┗", " ", "┛", " ", "━", " ", "▐", " ", "▝", " ", "▀", " ").Replace(plain)
		flat = strings.Join(strings.Fields(flat), " ")
		if want := strings.Join(strings.Fields(i18n.T("tui.quit_confirm")), " "); !strings.Contains(flat, want) {
			t.Errorf("[%v] mensagem cortada, falta %q em:\n%s", lang, want, flat)
		}
		foot := strings.Split(plain, "\n")[23]
		if strings.Contains(strings.ToUpper(foot), "ENTER") || !strings.Contains(foot, "Q ") {
			t.Errorf("rodapé do diálogo deveria mostrar só q/outra tecla: %q", foot)
		}
	}
}

// fgCodes devolve os códigos de cor de texto de 16 cores (30-37, 90-97) que as SGR da
// string ligam, pulando os argumentos de 38/48/58.
func fgCodes(s string) []int {
	var out []int
	for _, mt := range sgrSeq.FindAllStringSubmatch(s, -1) {
		p := strings.Split(mt[1], ";")
		for k := 0; k < len(p); k++ {
			n, _ := strconv.Atoi(p[k])
			switch {
			case n == 38 || n == 48 || n == 58:
				if k+1 < len(p) && p[k+1] == "2" {
					k += 4
				} else if k+1 < len(p) && p[k+1] == "5" {
					k += 2
				}
			case n >= 30 && n <= 37, n >= 90 && n <= 97:
				out = append(out, n)
			}
		}
	}
	return out
}

// Com 16 cores as tintas neutras não viram preto (30) nem cinza-escuro (90): num terminal
// escuro ficariam ilegíveis. A linha selecionada usa só negrito e vídeo invertido.
func TestANSI16NoBlackInk(t *testing.T) {
	th := themeWithProfile(termenv.ANSI, nil)
	if c := fgCodes(th.Selected.Render("x")); len(c) > 0 {
		t.Errorf("Selected com cor de texto %v: %q", c, th.Selected.Render("x"))
	}
	for name, m := range allScreens(t) {
		m = withTheme(m, th)
		m = send(m, size(80, 24))
		for _, extra := range []string{"", "?"} {
			if extra == "?" {
				if name == "execute" {
					continue
				}
				m = send(m, keyRune("?"))
			}
			t.Run(name+extra, func(t *testing.T) {
				v := m.View()
				for _, c := range fgCodes(v) {
					if c == 30 || c == 90 {
						t.Fatalf("cor de texto %d (preto) no modo de 16 cores:\n%q", c, v)
					}
				}
			})
		}
	}
}
