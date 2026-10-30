package tui

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

func TestViewPaintedEveryScreen(t *testing.T) {
	exec, _ := executing(t)
	sum, _ := summaryModel(t)
	welcome := detected(t, newModel(t, linuxSystemd))
	profile := send(welcome, enter())
	cats := send(profile, enter())
	screens := map[string]Model{
		"welcome": welcome, "profile": profile, "categories": cats,
		"review": reviewFor(t, linuxSystemd), "execute": exec, "summary": sum,
		"dashboard": dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)}),
	}
	for name, m := range screens {
		m.th = paperTheme()
		for _, sz := range [][2]int{{100, 30}, {80, 24}} {
			mm := send(m, size(sz[0], sz[1]))
			t.Run(name, func(t *testing.T) { assertPainted(t, mm.View(), sz[0], sz[1]) })
		}
	}
}

func TestHeaderShowsTrail(t *testing.T) {
	m := send(detected(t, newModel(t, linuxSystemd)), enter(), enter())
	i18n.SetLang(i18n.PtBR)
	t.Cleanup(func() { i18n.SetLang(i18n.En) })
	m.th = paperTheme()
	m.w, m.h = 100, 30
	v := m.View()
	for _, want := range []string{"KANIVET", "ITENS", "3/5"} {
		if !strings.Contains(v, want) {
			t.Errorf("view sem %q:\n%s", want, v)
		}
	}
}

func TestTooSmallPainted(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m.th = paperTheme()
	m.w, m.h = 60, 30
	assertPainted(t, m.View(), 60, m.h)
}

func TestPrimaryAction(t *testing.T) {
	m := newModel(t, linuxSystemd)
	m.detecting = false
	if k, _, ok := m.primaryAction(); !ok || k != "enter" {
		t.Fatalf("welcome: key=%q ok=%v", k, ok)
	}
	m.detecting = true
	if _, _, ok := m.primaryAction(); ok {
		t.Fatal("welcome detectando não tem ação principal")
	}
	m.screen = ScreenCategories
	if _, _, ok := m.primaryAction(); ok {
		t.Fatal("categorias sem ação principal")
	}
}

// No modo plain sem Nerd Font a trilha é a mais larga; o cabeçalho de 80 colunas
// não pode cortar nem o último passo nem o FIM.
func TestHeaderFitsWidest80(t *testing.T) {
	i18n.SetLang(i18n.PtBR)
	t.Cleanup(func() { i18n.SetLang(i18n.En) })
	sum, _ := summaryModel(t)
	i18n.SetLang(i18n.PtBR)
	sum = send(sum, size(80, 24))
	v := sum.View()
	for _, want := range []string{"INSTALAR", "FIM"} {
		if !strings.Contains(v, want) {
			t.Errorf("cabeçalho sem %q:\n%s", want, v)
		}
	}
}

func TestCategoriesPaintedSearchAndPanel(t *testing.T) {
	base := send(detected(t, newModel(t, linuxSystemd)), enter(), enter())
	base.th = paperTheme()
	base = send(base, size(80, 24))
	cases := map[string][]tea.Msg{
		"search": {keyRune("/"), keyRune("e")},
		"panel":  {keyRune("c")},
	}
	for name, msgs := range cases {
		t.Run(name, func(t *testing.T) {
			m := send(base, msgs...)
			assertPainted(t, m.View(), 80, 24)
		})
	}
}

// headerText devolve o texto da linha do cabeçalho, entre as bordas da caixa.
func headerText(v string) string {
	row := strings.Split(ansi.Strip(v), "\n")[1]
	if i := strings.Index(row, "┃"); i >= 0 {
		row = row[i+len("┃"):]
	}
	if i := strings.LastIndex(row, "┃"); i >= 0 {
		row = row[:i]
	}
	return strings.TrimSpace(row)
}

// leftover tira do texto as partes inteiras dadas e os separadores; o que sobra é
// pedaço de palavra cortada.
func leftover(s string, parts ...string) string {
	sort.Slice(parts, func(i, j int) bool { return len(parts[i]) > len(parts[j]) })
	for _, p := range parts {
		if p != "" {
			s = strings.ReplaceAll(s, p, " ")
		}
	}
	return strings.Join(strings.Fields(strings.NewReplacer("✓", " ", "─", " ").Replace(s)), " ")
}

// Em pt-BR, com Nerd Font e 80 colunas, a trilha degrada (passos feitos viram ✓,
// separador curto) em vez de cortar o passo ativo ou colar no N/5.
func TestHeaderTrailFitsPtBRNerd80(t *testing.T) {
	screens := allScreens(t)
	i18n.SetLang(i18n.PtBR)
	t.Cleanup(func() { i18n.SetLang(i18n.En) })
	labels := []string{}
	for _, k := range []string{"welcome", "profile", "items", "review", "install"} {
		labels = append(labels, strings.ToUpper(i18n.T("tui.trail."+k)))
	}
	for name, step := range map[string]int{"welcome": 0, "profile": 1, "categories": 2, "review": 3, "execute": 4, "summary": 5} {
		m := withTheme(screens[name], paperTheme())
		m = send(m, size(80, 24))
		t.Run(name, func(t *testing.T) {
			h := headerText(m.View())
			right := fmt.Sprintf("%d/5", step+1)
			if step == 5 {
				right = strings.ToUpper(i18n.T("tui.trail.end"))
			} else if !strings.Contains(h, labels[step]) {
				t.Errorf("passo ativo %q cortado: %q", labels[step], h)
			}
			if !strings.HasSuffix(h, " "+right) {
				t.Errorf("%q deveria vir separado por espaço no fim: %q", right, h)
			}
			if rest := leftover(h, append([]string{"KANIVET", right}, labels...)...); rest != "" {
				t.Errorf("pedaço de passo cortado %q em %q", rest, h)
			}
		})
	}
}

// O cabeçalho do painel em pt-BR com externos cabe em 80 colunas largando tags
// inteiras, nunca cortando uma no meio.
func TestDashHeaderDropsWholeTags80(t *testing.T) {
	statuses := map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled), "fd": st("fd", detect.StatusOutdated)}
	for _, id := range []string{"eza", "ripgrep", "jq", "zoxide", "btop", "lazygit", "delta", "tldr", "httpie", "yq"} {
		statuses[id] = st(id, detect.StatusExternal)
	}
	m := dashModel(t, statuses)
	m.s.St.Profile = "laravel"
	i18n.SetLang(i18n.PtBR)
	m = withTheme(m, paperTheme())
	m = send(m, size(80, 24))
	m.dash.checking = false
	h := headerText(m.View())
	var inst, upd, ext int
	for id := range m.s.Cat.Items {
		switch x := m.sel.status(id).Status; {
		case owned(x):
			inst++
			if x == detect.StatusOutdated {
				upd++
			}
		case external(x):
			ext++
		}
	}
	if ext == 0 || upd == 0 {
		t.Fatalf("o teste precisa de externos e atualizações: ext=%d upd=%d", ext, upd)
	}
	tags := []string{
		strings.ToUpper(i18n.T("tui.dash.profile", "laravel")),
		strings.ToUpper(i18n.N("tui.dash.hd_installed", inst)),
		strings.ToUpper(i18n.N("tui.dash.hd_updates", upd)),
		strings.ToUpper(i18n.N("tui.dash.hd_external", ext)),
	}
	if !strings.Contains(h, tags[0]) {
		t.Errorf("o perfil deve ficar: %q", h)
	}
	status := ansi.Strip(m.dashStatus())
	if rest := leftover(h, append([]string{"KANIVET", status}, tags...)...); rest != "" {
		t.Errorf("tag cortada no meio (%q) em %q", rest, h)
	}
}

// No painel a 80 colunas o rodapé não gasta espaço com contadores zerados e mantém os
// atalhos principais (tab, u, p, /, ?, q).
func TestDashFooterKeepsMainKeys80(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	m = send(m, size(80, 24))
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	foot := lines[len(lines)-1]
	for _, want := range []string{"TAB ", "U ", "P ", "/ ", "? ", "Q "} {
		if !strings.Contains(foot, want) {
			t.Errorf("rodapé sem %q: %q", want, foot)
		}
	}
	if strings.Contains(foot, "0 TO") {
		t.Errorf("contador zerado no rodapé: %q", foot)
	}
	// com pendências o contador aparece
	m = gotoCategory(t, m, "replacements")
	for m.dashList().Current() != "bat" {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = send(m, keyRune(" "))
	lines = strings.Split(ansi.Strip(m.View()), "\n")
	// aos 80 colunas o contador cede lugar a aplicar e aos atalhos principais
	if foot = lines[len(lines)-1]; !strings.Contains(foot, "A apply") || !strings.Contains(foot, "TAB ") {
		t.Errorf("rodapé com pendência: %q", foot)
	}
}

// A boas-vindas não repete o nome do passo acima da manchete: a trilha já o mostra.
func TestWelcomeNoRepeatedTitle(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.En, i18n.PtBR} {
		i18n.SetLang(lang)
		t.Cleanup(func() { i18n.SetLang(i18n.En) })
		m := detected(t, newModel(t, linuxSystemd))
		for _, sz := range [][2]int{{80, 24}, {120, 40}} {
			m = send(m, size(sz[0], sz[1]))
			v := ansi.Strip(m.View())
			title := strings.ToUpper(i18n.T("tui.step.welcome"))
			if n := strings.Count(v, title); n != 1 {
				t.Errorf("[%v %v] %q aparece %d vezes (só a trilha):\n%s", lang, sz, title, n, v)
			}
		}
	}
}

// pendingDash abre o dashboard em tema Paper com i itens a instalar, u a atualizar e
// r a remover, no tamanho w x h.
func pendingDash(t *testing.T, lang i18n.Lang, i, u, r, w, h int) Model {
	t.Helper()
	m := dashModel(t, nil)
	i18n.SetLang(lang)
	var ids []string
	excl := map[string]bool{}
	for _, c := range m.s.Cat.Categories {
		excl[c.ID] = c.Exclusive
	}
	for id, it := range m.s.Cat.Items {
		if !excl[it.Category] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) < i+u+r {
		t.Fatalf("catálogo com poucos itens: %d", len(ids))
	}
	sts := map[string]detect.ItemStatus{}
	for n, id := range ids[:i+u+r] {
		switch {
		case n < i:
			sts[id] = st(id, detect.StatusAbsent)
		case n < i+u:
			sts[id] = st(id, detect.StatusOutdated)
		default:
			sts[id] = st(id, detect.StatusInstalled)
		}
	}
	m = send(m, detectMsg{statuses: sts})
	for n, id := range ids[:i+u+r] {
		switch {
		case n < i:
			m.sel.Toggle(id)
		case n < i+u:
			m.sel.MarkUpdate(id)
		default:
			m.sel.Toggle(id)
		}
	}
	m.th = paperTheme()
	m = send(m, size(w, h))
	if gi, gu, gr := m.sel.Pending(); gi != i || gu != u || gr != r {
		t.Fatalf("pendências %d/%d/%d, esperado %d/%d/%d", gi, gu, gr, i, u, r)
	}
	return m
}

// Com mudanças pendentes, aplicar nunca cai do rodapé: os contadores encolhem e, se
// ainda faltar espaço, saem antes dos atalhos.
func TestDashFooterKeepsApplyWithPending(t *testing.T) {
	cases := []struct{ i, u, r, w, h int }{
		{1, 1, 1, 80, 24}, {1, 1, 1, 100, 30}, {12, 3, 7, 80, 24}, {12, 3, 7, 100, 30},
	}
	for _, lang := range []i18n.Lang{i18n.En, i18n.PtBR} {
		for _, c := range cases {
			name := fmt.Sprintf("%v/%d-%d-%d@%dx%d", lang, c.i, c.u, c.r, c.w, c.h)
			t.Run(name, func(t *testing.T) {
				m := pendingDash(t, lang, c.i, c.u, c.r, c.w, c.h)
				v := m.View()
				assertPainted(t, v, c.w, c.h)
				foot := footOf(v)
				if got := lipgloss.Width(foot); got != c.w {
					t.Errorf("rodapé com %d colunas, esperado %d: %q", got, c.w, foot)
				}
				keys := []string{"A ", "? ", "Q "}
				if c.w >= 80 {
					keys = append(keys, "TAB ")
				}
				for _, want := range keys {
					if !strings.Contains(foot, want) {
						t.Errorf("rodapé sem %q: %q", want, foot)
					}
				}
			})
		}
	}
}
