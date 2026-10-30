package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/muesli/termenv"
)

// allScreens monta cada tela com dados representativos: execução em andamento,
// resumo com falhas e painel com atualizações.
func allScreens(t *testing.T) map[string]Model {
	t.Helper()
	exec, _ := executing(t)
	sum, _ := summaryModel(t)
	welcome := detected(t, newModel(t, linuxSystemd))
	profile := send(welcome, enter())
	cats := send(profile, enter())
	dash := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	dash = send(dash, updatesMsg{statuses: map[string]detect.ItemStatus{"bat": st("bat", detect.StatusOutdated)}, gen: dash.dash.gen})
	return map[string]Model{
		"welcome": welcome, "profile": profile, "categories": cats,
		"review": reviewFor(t, linuxSystemd), "execute": exec, "summary": sum, "dashboard": dash,
	}
}

func checkScreens(t *testing.T, w, h int) {
	for name, m := range allScreens(t) {
		m.th = paperTheme()
		m = send(m, size(w, h))
		t.Run(name, func(t *testing.T) {
			v := m.View()
			assertPainted(t, v, w, h)
			plain := ansi.Strip(v)
			if !strings.Contains(plain, "KANIVET") {
				t.Errorf("sem KANIVET:\n%s", plain)
			}
			if _, label, ok := m.primaryAction(); ok && !strings.Contains(strings.ToUpper(plain), strings.ToUpper(label)) {
				t.Errorf("sem o botão %q:\n%s", label, plain)
			}
			lines := strings.Split(plain, "\n")
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

var bgSeq = regexp.MustCompile(`\x1b\[(48;|4[0-7]m|10[0-7]m)`)

func TestPlainModeHasNoBackground(t *testing.T) {
	themes := map[string]Theme{
		"no_color": themeWithProfile(termenv.TrueColor, map[string]string{"NO_COLOR": "1"}),
		"ansi16":   themeWithProfile(termenv.ANSI, nil),
	}
	for tn, th := range themes {
		for name, m := range allScreens(t) {
			m.th = th
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
					if bgSeq.MatchString(v) || strings.ContainsAny(v, "▐▀▝") {
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
			m.th = paperTheme()
			m = send(m, size(sz[0], sz[1]), keyRune("?"))
			t.Run(name, func(t *testing.T) {
				v := m.View()
				assertPainted(t, v, sz[0], sz[1])
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
	m := send(detected(t, newModel(t, linuxSystemd)), enter(), enter(), size(80, 24), keyRune("q"))
	m.th = paperTheme()
	if !m.confirmQuit {
		t.Fatal("q com pendências deveria pedir confirmação")
	}
	assertPainted(t, m.View(), 80, 24)
	plain := ansi.Strip(m.View())
	if !strings.Contains(plain, "Quit with pending changes") || !strings.Contains(plain, "KANIVET") || strings.Count(plain, "┏") < 2 {
		t.Errorf("diálogo de saída incompleto:\n%s", plain)
	}
}
