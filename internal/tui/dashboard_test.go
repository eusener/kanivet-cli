package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/state"
)

func st(id string, s detect.Status) detect.ItemStatus {
	return detect.ItemStatus{ID: id, Status: s, Version: "1.0"}
}

// dashModel abre o dashboard 100x30 com os status dados.
func dashModel(t *testing.T, statuses map[string]detect.ItemStatus) Model {
	t.Helper()
	i18n.SetLang(i18n.En)
	t.Cleanup(func() { i18n.SetLang(i18n.En) })
	m := New(newSession(t, linuxSystemd), map[string]string{"NO_COLOR": "1"}, ScreenDashboard)
	return send(m, size(100, 30), detectMsg{statuses: statuses})
}

// gotoCategory troca de aba (tab) até a categoria id.
func gotoCategory(t *testing.T, m Model, id string) Model {
	t.Helper()
	for i, c := range m.s.Cat.Categories {
		if c.ID == id {
			for j := 0; j < i; j++ {
				m = send(m, tea.KeyMsg{Type: tea.KeyTab})
			}
			return m
		}
	}
	t.Fatalf("categoria %s não existe", id)
	return m
}

func TestDashboardGolden(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{
		"bat":     st("bat", detect.StatusInstalled),
		"eza":     st("eza", detect.StatusExternal),
		"ripgrep": st("ripgrep", detect.StatusOutdated),
		"btop":    {ID: "btop", Status: detect.StatusUnsupported, Reason: "no package"},
	})
	m = gotoCategory(t, m, "replacements")
	golden(t, "dashboard.golden", trimView(m.View()))
}

func TestDashboardPendingBar(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	m = gotoCategory(t, m, "replacements")
	for m.dashList().Current() != "bat" {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = send(m, keyRune(" ")) // desmarca bat (nosso) -> a remover
	if m.sel.Marked("bat") {
		t.Fatal("bat deveria estar desmarcado")
	}
	m = send(m, keyRune("/"), keyRune("j"), keyRune("q"), enter()) // busca global por "jq"
	if m.dashList().Current() != "jq" {
		t.Fatalf("a busca global deve achar jq, achou %q", m.dashList().Current())
	}
	m = send(m, keyRune(" "))
	if v := m.View(); !hasAll(v, "1 to install", "0 to update", "1 to remove") {
		t.Fatalf("barra errada:\n%s", v)
	}
}

func TestDashboardUpdateAllMarksOutdated(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{
		"bat": st("bat", detect.StatusOutdated), "fd": st("fd", detect.StatusOutdated), "sd": st("sd", detect.StatusInstalled),
	})
	m = send(m, keyRune("U"))
	if v := m.View(); !hasAll(v, "0 to install", "2 to update", "0 to remove") {
		t.Fatalf("U deve marcar os 2 desatualizados:\n%s", v)
	}
	m = send(m, keyRune("U"))
	if v := m.View(); !hasAll(v, "0 to update") {
		t.Fatalf("U de novo desfaz:\n%s", v)
	}
}

func TestDashboardUpdateKeyMarksCurrentOutdated(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusOutdated)})
	m = gotoCategory(t, m, "replacements")
	for m.dashList().Current() != "bat" {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = send(m, keyRune("u"))
	if !hasAll(m.View(), "0 to install", "1 to update") {
		t.Fatalf("u deve marcar bat:\n%s", m.View())
	}
}

func TestDashboardApplyOpensReview(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	nm, cmd := m.Update(keyRune("a"))
	m = nm.(Model)
	if m.screen != ScreenReview || m.review.caller != ScreenDashboard || cmd == nil {
		t.Fatalf("a deve abrir a revisão vinda do dashboard: screen=%v caller=%v cmd=%v", m.screen, m.review.caller, cmd != nil)
	}
	m = send(m, esc())
	if m.screen != ScreenDashboard {
		t.Fatalf("esc volta ao dashboard, está em %v", m.screen)
	}
}

func TestDashboardProfileModalIsAdditive(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{
		"bat": st("bat", detect.StatusInstalled), "eza": st("eza", detect.StatusExternal),
	})
	before := map[string]bool{}
	for id := range m.s.Cat.Items {
		before[id] = m.sel.Marked(id)
	}
	m = send(m, keyRune("p"))
	if !strings.Contains(m.View(), "WEB (JS/TS)") {
		t.Fatalf("o modal deve listar os perfis:\n%s", m.View())
	}
	for i, p := range m.profiles() {
		if p.ID == "web" {
			m.profCursor = i
		}
	}
	m = send(m, enter())
	for id, was := range before {
		if was && !m.sel.Marked(id) {
			t.Errorf("aplicar web desmarcou %s", id)
		}
	}
	if !m.sel.Marked("node") || m.dash.profile != "web" || m.screen != ScreenDashboard {
		t.Errorf("web deve marcar node e guardar o perfil (profile=%q)", m.dash.profile)
	}
	if n, _, _ := m.sel.Pending(); n == 0 {
		t.Error("o perfil deve deixar itens a instalar")
	}
}

func TestDashboardExternalUntouched(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"eza": st("eza", detect.StatusExternal)})
	m = gotoCategory(t, m, "replacements")
	for m.dashList().Current() != "eza" {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = send(m, keyRune(" "))
	// a nota é quebrada na caixa de detalhe: junta a coluna do detalhe e compara a frase inteira
	explained := strings.Contains(detailText(m.View()), i18n.T("tui.external_untouched"))
	if !m.sel.Marked("eza") || !explained {
		t.Fatalf("externo não desmarca e explica:\n%s", m.View())
	}
}

func TestDashboardFreshSelectionAfterSummary(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	m.screen = ScreenSummary
	m.sel = NewSelection(m.s.Cat, m.s.P, nil, ModeWizard)
	nm, cmd := m.Update(enter())
	m = nm.(Model)
	if m.screen != ScreenDashboard || !m.detecting || m.sel != nil || cmd == nil {
		t.Fatalf("enter no resumo re-detecta: screen=%v detecting=%v sel=%v", m.screen, m.detecting, m.sel)
	}
	m = send(m, detectMsg{statuses: map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)}})
	if m.sel.mode != ModeDashboard || !m.sel.Marked("bat") {
		t.Fatal("a seleção do dashboard deve ser nova, em ModeDashboard")
	}
}

func TestDashboardUpdatesMsgAppliesStatuses(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	out := map[string]detect.ItemStatus{"bat": st("bat", detect.StatusOutdated)}
	m = send(m, updatesMsg{statuses: out, gen: m.dash.gen})
	m = send(m, keyRune("U"))
	if !hasAll(m.View(), "1 to update") {
		t.Fatalf("o resultado da verificação deve chegar à seleção:\n%s", m.View())
	}
	stale := map[string]detect.ItemStatus{}
	m2 := send(m, updatesMsg{statuses: stale, gen: m.dash.gen - 1})
	if len(m2.statuses) == 0 {
		t.Fatal("resultado de uma geração antiga deve ser ignorado")
	}
}

// hasAll diz se v tem cada texto como tag de contador (as tags saem em maiúsculas).
func hasAll(v string, texts ...string) bool {
	for _, t := range texts {
		if !strings.Contains(v, strings.ToUpper(t)) {
			return false
		}
	}
	return true
}

// detailText junta o texto da última caixa de cada linha (o detalhe), com os espaços
// e as quebras de linha reduzidos a um espaço.
func detailText(v string) string {
	var parts []string
	for _, ln := range strings.Split(ansi.Strip(v), "\n") {
		rs := []rune(ln)
		var at []int
		for i, r := range rs {
			if r == '│' || r == '┃' {
				at = append(at, i)
			}
		}
		if len(at) >= 2 {
			parts = append(parts, string(rs[at[len(at)-2]+1:at[len(at)-1]]))
		}
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func tabKey() tea.Msg      { return tea.KeyMsg{Type: tea.KeyTab} }
func shiftTabKey() tea.Msg { return tea.KeyMsg{Type: tea.KeyShiftTab} }

func TestDashTabsSwitch(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	n := len(m.s.Cat.Categories)
	first := m.dashList().ids[0]
	m = send(m, tabKey())
	if m.dash.cat != 1 || m.dashList().ids[0] == first {
		t.Fatalf("tab avança a aba e troca a lista: cat=%d", m.dash.cat)
	}
	m = send(m, shiftTabKey())
	if m.dash.cat != 0 || m.dashList().ids[0] != first {
		t.Fatalf("shift+tab volta: cat=%d", m.dash.cat)
	}
	for i := 0; i < n-1; i++ {
		m = send(m, tabKey())
	}
	if m.dash.cat != n-1 {
		t.Fatalf("última aba: cat=%d", m.dash.cat)
	}
	if m = send(m, tabKey()); m.dash.cat != 0 {
		t.Fatalf("tab na última volta à primeira: cat=%d", m.dash.cat)
	}
	if m = send(m, shiftTabKey()); m.dash.cat != n-1 {
		t.Fatalf("shift+tab na primeira vai à última: cat=%d", m.dash.cat)
	}
	if m.dashList().panel {
		t.Fatal("tab não deve focar o painel de configs no painel")
	}
}

func TestDashTabsScrollKeepsActive(t *testing.T) {
	th := paperTheme()
	var names []string
	for i := 0; i < 14; i++ {
		names = append(names, "categoria"+string(rune('a'+i)))
	}
	for active := range names {
		line := stripANSI(th.Tabs(names, active, 80))
		if w := lipgloss.Width(line); w != 80 {
			t.Errorf("aba %d: largura %d", active, w)
		}
		if !strings.Contains(line, strings.ToUpper(names[active])) {
			t.Errorf("aba %d fora da vista: %q", active, line)
		}
		if active > 3 && !strings.HasPrefix(strings.TrimSpace(line), "…") {
			t.Errorf("aba %d: falta … à esquerda: %q", active, line)
		}
		if active < 10 && !strings.HasSuffix(strings.TrimSpace(line), "…") {
			t.Errorf("aba %d: falta … à direita: %q", active, line)
		}
	}
}

func TestDashShowsVersionAndLatest(t *testing.T) {
	i18n.SetLang(i18n.PtBR)
	m := dashModel(t, map[string]detect.ItemStatus{
		"ripgrep": {ID: "ripgrep", Status: detect.StatusOutdated, Version: "0.9.2", Latest: "0.10.0"},
		"bat":     st("bat", detect.StatusInstalled),
	})
	i18n.SetLang(i18n.PtBR)
	m.th = paperTheme()
	m = gotoCategory(t, m, "replacements")
	v := ansi.Strip(m.View())
	for _, want := range []string{"0.9.2", "⇡ 0.10.0", "EM DIA"} {
		if !strings.Contains(v, want) {
			t.Errorf("view sem %q:\n%s", want, v)
		}
	}
}

func TestDashHeaderCounts(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{
		"bat": st("bat", detect.StatusInstalled), "fd": st("fd", detect.StatusOutdated),
		"eza": st("eza", detect.StatusExternal),
	})
	v := ansi.Strip(m.View())
	for _, want := range []string{"KANIVET", "2 INSTALLED", "1 UPDATES", "1 EXTERNAL"} {
		if !strings.Contains(v, want) {
			t.Errorf("cabeçalho sem %q:\n%s", want, v)
		}
	}
}

func TestDashInstalledDate(t *testing.T) {
	i18n.SetLang(i18n.PtBR)
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	i18n.SetLang(i18n.PtBR)
	m.s.St.SetItem("bat", state.InstalledItem{At: time.Date(2026, 10, 29, 12, 0, 0, 0, time.Local)})
	m = gotoCategory(t, m, "replacements")
	for m.dashList().Current() != "bat" {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if d := detailText(m.View()); !strings.Contains(d, "PELO KANIVET · 29/10") {
		t.Fatalf("detalhe sem a data: %q", d)
	}
}

func TestDashPaintedProfileModal(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	m.th = paperTheme()
	for _, sz := range [][2]int{{100, 30}, {80, 24}} {
		mm := send(m, size(sz[0], sz[1]))
		assertPainted(t, mm.View(), sz[0], sz[1])
		mm = send(mm, keyRune("p"))
		assertPainted(t, mm.View(), sz[0], sz[1])
	}
}

func TestWizardTabTogglesConfigPanel(t *testing.T) {
	m := send(detected(t, newModel(t, linuxSystemd)), enter(), enter())
	if m.screen != ScreenCategories {
		t.Fatalf("esperava as categorias, está em %v", m.screen)
	}
	for i := 0; len(m.list.configs()) == 0; i++ {
		if i > 100 {
			t.Fatal("nenhum item com configs na primeira página")
		}
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.list.panel {
		t.Fatal("o painel começa sem foco")
	}
	if m = send(m, tabKey()); !m.list.panel {
		t.Fatal("o primeiro tab deve focar o painel de configs")
	}
	if m = send(m, tabKey()); m.list.panel {
		t.Fatal("o segundo tab deve devolver o foco à lista")
	}
}

func TestDashTabGoesToGlobalSearch(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	m = send(m, keyRune("/"))
	if !m.dashList().Filtering() {
		t.Fatal("a busca global deve estar digitando")
	}
	cat := m.dash.cat
	m = send(m, tabKey())
	if m.dash.cat != cat || m.dash.global == nil || !m.dashList().Filtering() {
		t.Fatalf("tab na busca não troca de aba: cat=%d->%d", cat, m.dash.cat)
	}
}

func TestDashTabIgnoredInProfileModal(t *testing.T) {
	m := dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)})
	m = send(m, keyRune("p"))
	cat := m.dash.cat
	for _, k := range []tea.Msg{tabKey(), shiftTabKey()} {
		m = send(m, k)
		if m.dash.cat != cat || !m.dash.modal {
			t.Fatalf("tab com o modal aberto não faz nada: cat=%d modal=%v", m.dash.cat, m.dash.modal)
		}
	}
}
