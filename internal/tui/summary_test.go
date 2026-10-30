package tui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/configs"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/executor"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/state"
)

func keyRune(r string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)} }

// summaryModel monta o resumo com 1 sucesso, 1 falha (com log) e 1 conflito de config.
func summaryModel(t *testing.T) (Model, string) {
	t.Helper()
	i18n.SetLang(i18n.En)
	s := okSession(t)
	m := New(s, nil, ScreenSummary)
	m = send(m, size(100, 30))
	j, err := state.NewJournal(s.StateDir, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := j.LogWriter("node")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 1; i <= 250; i++ {
		fmt.Fprintf(&sb, "log line %d\n", i)
	}
	w.Write([]byte(sb.String()))
	w.Close()
	target := s.ExpandTarget("{{.XDG_CONFIG}}/fzf/fzfrc")
	m.plan = &plan.Plan{Steps: []plan.Step{
		{ID: "install:fzf", ItemID: "fzf", Kind: plan.KindInstall},
		{ID: "install:node", ItemID: "node", Kind: plan.KindInstall},
		{ID: "config:fzf-defaults", ItemID: "fzf", Kind: plan.KindConfig},
		{ID: plan.ShellStepID, Kind: plan.KindShell},
	}}
	m.runJ = j
	m.runRep = executor.Report{
		Done:      []string{"install:fzf", "config:fzf-defaults", plan.ShellStepID},
		Failed:    []string{"install:node"},
		Conflicts: []configs.Conflict{{Target: target, Diff: "-old line\n+new line\n"}},
	}
	return m, target
}

func TestSummaryGolden(t *testing.T) {
	m, _ := summaryModel(t)
	m.s.Cat.Items["fzf"].Notes = i18n.Text{i18n.En: "Run fzf --help once."}
	m.runJ = &state.Journal{Dir: "/state/runs/R1"}
	m.runRep.Conflicts[0].Target = "/home/u/.config/fzf/fzfrc"
	m = send(m, size(80, 24))
	m.th = NewTheme(map[string]string{"NO_COLOR": "1"}, false)
	golden(t, "summary.golden", trimView(m.View()))
}

func TestSummaryOpensLog(t *testing.T) {
	m, _ := summaryModel(t)
	m = send(m, keyRune("l")) // I4: o log abre com l (enter vai ao painel)
	v := m.View()
	if !strings.Contains(v, "log line 250") {
		t.Fatalf("o log deve abrir no fim:\n%s", v)
	}
	top := m
	for i := 0; i < 30; i++ {
		top = send(top, tea.KeyMsg{Type: tea.KeyPgUp})
	}
	tv := top.View()
	if !strings.Contains(tv, "log line 51 ") || strings.Contains(tv, "log line 50 ") {
		t.Fatalf("o topo deve ser a linha 51 (200 linhas):\n%s", tv)
	}
	m = send(m, esc())
	if strings.Contains(m.View(), "log line 250") {
		t.Fatal("esc não fechou o log")
	}
}

func TestSummaryDiffOfConflict(t *testing.T) {
	m, _ := summaryModel(t)
	m = send(m, keyRune("j"), keyRune("d"))
	if v := m.View(); !strings.Contains(v, "+new line") {
		t.Fatalf("sem diff:\n%s", v)
	}
	m = send(m, esc())
	if strings.Contains(m.View(), "+new line") {
		t.Fatal("esc não fechou o diff")
	}
}

func TestSummaryForceConflictAsksConfirmation(t *testing.T) {
	m, target := summaryModel(t)
	if err := os.MkdirAll(target[:strings.LastIndex(target, "/")], 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(target, []byte("mine\n"), 0o644)

	m = send(m, keyRune("j")) // cursor no conflito
	nm, cmd := m.Update(keyRune("F"))
	m = nm.(Model)
	if cmd != nil || !strings.Contains(m.View(), "[y/N]") {
		t.Fatalf("F deve só pedir confirmação:\n%s", m.View())
	}
	nm, cmd = m.Update(keyRune("n"))
	m = nm.(Model)
	if cmd != nil || strings.Contains(m.View(), "[y/N]") {
		t.Fatal("n deve cancelar")
	}
	if b, _ := os.ReadFile(target); string(b) != "mine\n" {
		t.Fatal("arquivo mexido sem confirmação")
	}

	m = send(m, keyRune("F"))
	nm, cmd = m.Update(keyRune("y"))
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("y deve produzir um cmd")
	}
	msg := cmd()
	if b, _ := os.ReadFile(target); string(b) == "mine\n" {
		t.Fatal("config não foi reaplicada com força")
	}
	rendered, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	cfg := m.s.St.Configs[target]
	if cfg.SHA256 != shaHex(rendered) {
		t.Fatalf("St.Configs[target].SHA256 = %q, want sha do conteúdo reaplicado", cfg.SHA256)
	}
	if b, err := os.ReadFile(cfg.Backup); err != nil || string(b) != "mine\n" {
		t.Fatalf("backup = %q, %v", b, err)
	}
	m = send(m, msg)
	if v := m.View(); !strings.Contains(v, "reapplied") {
		t.Fatalf("sem resultado:\n%s", v)
	}
	// só o passo daquela config rodou: o journal novo tem um passo config:fzf-defaults
	if _, ok := msg.(forceDoneMsg); !ok {
		t.Fatalf("msg = %T", msg)
	}
}

func TestSummaryShowsItemNotes(t *testing.T) {
	m, _ := summaryModel(t)
	m.s.Cat.Items["fzf"].Notes = i18n.Text{i18n.En: "Run fzf --help once."}
	v := m.View()
	for _, want := range []string{"Run fzf --help once.", i18n.T("summary.new_terminal")} {
		if !strings.Contains(v, want) {
			t.Errorf("sem %q:\n%s", want, v)
		}
	}
}

func TestSummaryKeys(t *testing.T) {
	m, _ := summaryModel(t)
	m.sel = NewSelection(m.s.Cat, m.s.P, nil, ModeWizard)
	_, cmd := m.Update(keyRune("q"))
	if !isQuit(cmd) {
		t.Fatal("q deve sair direto")
	}
	m.runRep.Failed = nil
	m = send(m, enter())
	if m.screen != ScreenDashboard {
		t.Fatalf("enter sem falha: screen = %v", m.screen)
	}
}

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Forçar A não pode reescrever B, outro arquivo da mesma config que o usuário também editou.
func TestSummaryForceOnlyTouchesConfirmedFile(t *testing.T) {
	m, _ := summaryModel(t)
	s := m.s
	tpl := "configs/fzf/fzfrc"
	s.Cat.Items["fzf"].Configs = []catalog.ConfigSpec{{ID: "two", Files: []catalog.ConfigFile{
		{Template: tpl, Target: "{{.XDG_CONFIG}}/fzf/a"},
		{Template: tpl, Target: "{{.XDG_CONFIG}}/fzf/b"},
	}}}
	a, b := s.ExpandTarget("{{.XDG_CONFIG}}/fzf/a"), s.ExpandTarget("{{.XDG_CONFIG}}/fzf/b")
	if err := os.MkdirAll(a[:strings.LastIndex(a, "/")], 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(a, []byte("orig A\n"), 0o644)
	first := &plan.Plan{Steps: []plan.Step{{ID: "config:two", ItemID: "fzf", Kind: plan.KindConfig, Files: []plan.FileChange{
		{Target: "{{.XDG_CONFIG}}/fzf/a", Template: tpl, ItemID: "fzf"},
		{Target: "{{.XDG_CONFIG}}/fzf/b", Template: tpl, ItemID: "fzf"},
	}}}}
	if rep, _, err := s.Run(context.Background(), first, nil, app.RunOptions{}); err != nil || len(rep.Failed) > 0 {
		t.Fatalf("primeira aplicação: %+v %v", rep, err)
	}
	prevB := s.St.Configs[b]
	os.WriteFile(a, []byte("edited A\n"), 0o644)
	os.WriteFile(b, []byte("edited B\n"), 0o644)
	m.runRep.Conflicts = []configs.Conflict{{Target: a, Diff: "x"}, {Target: b, Diff: "y"}}
	m.sum = summaryState{cursor: 1} // falha em 0, conflito A em 1

	m = send(m, keyRune("F"))
	nm, cmd := m.Update(keyRune("y"))
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("y deve produzir um cmd")
	}
	msg := cmd()
	if got, _ := os.ReadFile(b); string(got) != "edited B\n" {
		t.Fatalf("B foi sobrescrito sem confirmação: %q", got)
	}
	if got, _ := os.ReadFile(a); string(got) == "edited A\n" {
		t.Fatal("A não foi reaplicado")
	}
	if s.St.Configs[b] != prevB {
		t.Fatalf("estado de B mudou: %+v -> %+v", prevB, s.St.Configs[b])
	}
	got, _ := os.ReadFile(a)
	if s.St.Configs[a].SHA256 != shaHex(got) {
		t.Fatal("estado de A não reflete o conteúdo reaplicado")
	}
	if bk, err := os.ReadFile(s.St.Configs[a].Backup); err != nil || string(bk) != "orig A\n" {
		t.Fatalf("backup de A = %q, %v", bk, err)
	}
	_ = msg
}

// I5: encolher a janela abaixo de 4 linhas, rolar e voltar não pode estourar o viewport.
func TestSummaryPanelSurvivesTinyResize(t *testing.T) {
	m, _ := summaryModel(t)
	m = m.openLog("install:node")
	m = send(m, size(80, 2))
	for i := 0; i < 40; i++ {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = send(m, size(100, 30))
	viewNoPanic(t, m)
	if m.sum.vp.Height < 1 || m.sum.vp.YOffset > m.sum.vp.TotalLineCount() {
		t.Fatalf("viewport fora dos limites: height=%d yoffset=%d total=%d", m.sum.vp.Height, m.sum.vp.YOffset, m.sum.vp.TotalLineCount())
	}
}

func viewNoPanic(t *testing.T, m Model) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View entrou em pânico: %v", r)
		}
	}()
	_ = m.View()
}

// retryModel monta um resumo com 2 falhas (node, lerd) e 3 sucessos (fzf, flock, mise).
func retryModel(t *testing.T) Model {
	t.Helper()
	m, _ := summaryModel(t)
	m.sel = NewSelection(m.s.Cat, m.s.P, nil, ModeWizard)
	m.sel.ApplyProfile("laravel")
	steps := []plan.Step{}
	for _, id := range []string{"fzf", "flock", "mise", "node", "lerd"} {
		steps = append(steps, plan.Step{ID: "install:" + id, ItemID: id, Kind: plan.KindInstall})
	}
	m.plan = &plan.Plan{Steps: steps}
	m.runRep = executor.Report{
		Done:   []string{"install:fzf", "install:flock", "install:mise"},
		Failed: []string{"install:node", "install:lerd"},
	}
	m.sum = summaryState{}
	return m
}

func TestSummaryLogKey(t *testing.T) {
	m, _ := summaryModel(t)
	// I4: numa falha, enter vai ao painel (o botão principal continua visível) e l abre o log
	if _, label, ok := m.primaryAction(); !ok || !strings.Contains(strings.ToUpper(footOf(m.View())), strings.ToUpper(label)) {
		t.Fatalf("o botão ENTER do painel deve aparecer mesmo com o cursor numa falha: %q", footOf(m.View()))
	}
	for _, b := range m.summaryBindings() {
		if b.Help().Key == "enter" && b.Help().Desc != i18n.T("tui.summary.key_dashboard") {
			t.Errorf("enter na falha: %q, quer o painel", b.Help().Desc)
		}
	}
	if d := send(m, enter()); d.screen != ScreenDashboard || d.sum.panel != panelNone {
		t.Fatalf("enter numa falha deve ir ao painel: screen=%v panel=%v", d.screen, d.sum.panel)
	}
	m = send(m, keyRune("l"))
	if m.sum.panel != panelLog || !strings.Contains(m.View(), "log line 250") {
		t.Fatalf("l numa falha deve abrir o log: panel=%v", m.sum.panel)
	}
	m = send(m, esc(), keyRune("j"), keyRune("l")) // no conflito, l não faz nada
	if m.sum.panel != panelNone {
		t.Fatal("l fora de uma falha não abre o log")
	}
}

func TestRetryBuildsSelectionOfFailed(t *testing.T) {
	m := retryModel(t)
	sel, ok := m.retrySelection()
	if !ok {
		t.Fatal("retrySelection ok = false")
	}
	if got := strings.Join(sel.Add, ","); got != "node,lerd" {
		t.Fatalf("Add = %v, quer só os 2 que falharam", sel.Add)
	}
	if len(sel.Remove)+len(sel.Uninstall) != 0 {
		t.Fatalf("sem remoções: %+v", sel)
	}
	if sel.Profile != "" {
		t.Fatalf("Profile = %q: o perfil expandiria itens que não falharam", sel.Profile)
	}
}

func TestRetryKeepsVersions(t *testing.T) {
	m := retryModel(t)
	m.sel.versions["node"] = "24"
	want := m.sel.ToPlan().Versions
	sel, _ := m.retrySelection()
	if len(want) == 0 || fmt.Sprint(sel.Versions) != fmt.Sprint(want) {
		t.Fatalf("Versions = %v, quer %v", sel.Versions, want)
	}
}

func TestRetryEntersReview(t *testing.T) {
	m := retryModel(t)
	m = send(m, keyRune("r"))
	if m.screen != ScreenReview || m.review.caller != ScreenSummary {
		t.Fatalf("screen=%v caller=%v", m.screen, m.review.caller)
	}
	if !m.review.loading {
		t.Fatal("a revisão deve calcular o plano")
	}
	m = send(m, esc()) // esc volta ao resumo
	if m.screen != ScreenSummary || len(m.runRep.Failed) != 2 {
		t.Fatalf("esc deve voltar ao resumo intacto: screen=%v", m.screen)
	}
}

func TestRetryDisabledWithoutFailures(t *testing.T) {
	enabled := func(m Model) bool {
		for _, b := range m.summaryBindings() {
			if b.Help().Key == "r" {
				return b.Enabled()
			}
		}
		return false
	}
	m := retryModel(t)
	if !enabled(m) {
		t.Fatal("r deve estar habilitado com falhas")
	}
	m.runRep.Failed = nil
	if enabled(m) {
		t.Fatal("r habilitado sem falhas")
	}
	if m = send(m, keyRune("r")); m.screen != ScreenSummary {
		t.Fatalf("r sem falhas mudou a tela: %v", m.screen)
	}

	m = retryModel(t)
	m.sel.SetStatuses(map[string]detect.ItemStatus{
		"node": {ID: "node", Status: detect.StatusUnsupported},
		"lerd": {ID: "lerd", Status: detect.StatusUnsupported},
	})
	if enabled(m) {
		t.Fatal("r habilitado com falhas todas inselecionáveis")
	}
	if m = send(m, keyRune("r")); m.screen != ScreenSummary {
		t.Fatalf("r mudou a tela: %v", m.screen)
	}
	// uma só inselecionável: o retry leva só a outra
	m = retryModel(t)
	m.sel.SetStatuses(map[string]detect.ItemStatus{"node": {ID: "node", Status: detect.StatusUnsupported}})
	if sel, ok := m.retrySelection(); !ok || strings.Join(sel.Add, ",") != "lerd" {
		t.Fatalf("Add = %v", sel.Add)
	}
}

func TestRetryResetsRunState(t *testing.T) {
	m := retryModel(t)
	m.runErr = fmt.Errorf("boom")
	m.sum.cursor = 1
	m.review = reviewState{caller: ScreenSummary, pl: &plan.Plan{Steps: []plan.Step{{ID: "install:node", ItemID: "node", Kind: plan.KindInstall}}}}
	m.screen, m.detecting = ScreenReview, false
	m = send(m, enter())
	t.Cleanup(m.exec.cancel)
	if m.screen != ScreenExecute {
		t.Fatalf("screen = %v", m.screen)
	}
	if len(m.runRep.Failed)+len(m.runRep.Done) != 0 || m.runErr != nil || m.runJ != nil || m.sum.cursor != 0 {
		t.Fatalf("o run novo deve zerar o resultado: %+v %v", m.runRep, m.runErr)
	}
	if len(m.exec.steps) != 1 || m.exec.took == nil {
		t.Fatalf("exec não refeito: %+v", m.exec.steps)
	}
}

func TestSummaryTitles(t *testing.T) {
	m, _ := summaryModel(t)
	if v := m.View(); !strings.Contains(v, "FINISHED WITH FAILURES") {
		t.Fatalf("sem título de falha:\n%s", v)
	}
	m.runRep.Failed, m.runRep.Conflicts = nil, nil
	m.exec.elapsed = 3*time.Minute + 42*time.Second
	if v := m.View(); !strings.Contains(v, "DONE IN 3M42S") {
		t.Fatalf("sem título de sucesso:\n%s", v)
	}
	m.exec.interrupted = true
	if v := m.View(); !strings.Contains(v, "INTERRUPTED") {
		t.Fatalf("sem título de interrupção:\n%s", v)
	}
}

func TestSummaryPtBRLongFits(t *testing.T) {
	m := retryModel(t)
	i18n.SetLang(i18n.PtBR)
	defer i18n.SetLang(i18n.En)
	m.keys = DefaultKeys() // os rótulos das teclas nascem na língua ativa
	m.exec.notes = map[string]string{"install:node": strings.Repeat("sem binário para linux/arm64 ", 8)[:200]}
	m.runRep.Conflicts = []configs.Conflict{{Target: "/home/u/" + strings.Repeat("diretorio/", 20) + "arquivo", Diff: "x"}}
	m.th = paperTheme()
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}} {
		m = send(m, size(sz[0], sz[1]))
		v := m.View()
		assertPainted(t, v, sz[0], sz[1])
		for _, want := range []string{"TERMINOU COM FALHAS", "VER LOG", "TENTAR DE NOVO", "…"} {
			if !strings.Contains(ansi.Strip(v), want) {
				t.Errorf("%dx%d sem %q:\n%s", sz[0], sz[1], want, ansi.Strip(v))
			}
		}
	}
	plain := retryModel(t)
	plain.th = NewTheme(map[string]string{"NO_COLOR": "1"}, false)
	plain = send(plain, size(80, 24))
	for i, ln := range strings.Split(plain.View(), "\n") {
		if lipgloss.Width(ln) != 80 {
			t.Errorf("plain linha %d largura %d", i, lipgloss.Width(ln))
		}
	}
}

func TestSummaryPainted(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.En, i18n.PtBR} {
		i18n.SetLang(lang)
		m, _ := summaryModel(t)
		i18n.SetLang(lang)
		m.th = paperTheme()
		for _, sz := range [][2]int{{80, 24}, {100, 30}, {140, 50}} {
			m = send(m, size(sz[0], sz[1]))
			assertPainted(t, m.View(), sz[0], sz[1])
		}
	}
	i18n.SetLang(i18n.En)
}

// O plano do retry, montado como a revisão faz, só tem os itens que falharam (e o shell compartilhado).
func TestRetryPlanOnlyFailedItems(t *testing.T) {
	m := retryModel(t)
	m.s = testSession(t) // detecção diz "ausente": os itens entram no plano
	m.sel = NewSelection(m.s.Cat, m.s.P, nil, ModeWizard)
	m.sel.ApplyProfile("laravel")
	m.sel.Toggle("fzf") // desmarca fzf: o perfil o traz, o usuário o tirou
	if m.sel.Marked("fzf") {
		t.Fatal("fzf deveria estar desmarcado")
	}
	sel, ok := m.retrySelection()
	if !ok {
		t.Fatal("sem retry")
	}
	pl, _, err := m.s.Plan(context.Background(), sel)
	if err != nil {
		t.Fatal(err)
	}
	failed := map[string]bool{"node": true, "lerd": true, "mise": true} // mise: dependência de node
	for _, st := range pl.Steps {
		if st.ID == "config:fzf-defaults" {
			t.Fatalf("config do fzf desmarcado no plano do retry: %+v", st)
		}
		if st.ItemID != "" && !failed[st.ItemID] {
			t.Errorf("passo %s de item fora dos que falharam (e suas dependências)", st.ID)
		}
		if st.ItemID == "" && st.ID != plan.ShellStepID {
			t.Errorf("passo inesperado %s", st.ID)
		}
	}
	if len(pl.Steps) == 0 {
		t.Fatalf("plano vazio: sel=%+v", sel)
	}
	// o perfil continua registrado na revisão
	nm := send(m, keyRune("r"))
	if nm.review.profile != m.sel.ToPlan().Profile || nm.review.profile == "" {
		t.Fatalf("review.profile = %q", nm.review.profile)
	}
}

// I3: um chsh:zsh que falhou volta no retry como troca de shell (SwitchToZsh), não como
// instalação do zsh, e o plano refeito tem o passo do chsh.
func TestRetryChshSwitchesShell(t *testing.T) {
	p := linuxSystemd
	p.Shell = "bash"
	s := newSession(t, p)
	m := send(New(s, nil, ScreenSummary), size(100, 30))
	m.sel = NewSelection(s.Cat, s.P, nil, ModeWizard)
	m.plan = &plan.Plan{Steps: []plan.Step{
		{ID: "install:zsh", ItemID: "zsh", Kind: plan.KindInstall},
		{ID: "chsh:zsh", ItemID: "zsh", Kind: plan.KindChsh},
	}}
	m.runRep = executor.Report{Done: []string{"install:zsh"}, Failed: []string{"chsh:zsh"}}
	sel, ok := m.retrySelection()
	if !ok || !sel.SwitchToZsh {
		t.Fatalf("retry do chsh: ok=%v sel=%+v", ok, sel)
	}
	for _, id := range sel.Add {
		if id == "zsh" {
			t.Errorf("o chsh não deve virar Add:[zsh]: %+v", sel)
		}
	}
	pl, _, err := s.Plan(context.Background(), sel)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pl.Step("chsh:zsh"); !ok {
		t.Fatalf("o plano do retry não tem o chsh: %+v", pl.Steps)
	}
}

// I5: a revisão mostra um único total (o do título); o rodapé não repete os contadores
// da seleção, que podem divergir do plano.
func TestReviewFooterHasNoCounters(t *testing.T) {
	m := reviewFor(t, linuxSystemd)
	if tags := m.counterTags(); tags != nil {
		t.Fatalf("counterTags na revisão = %q", tags)
	}
	m = retryModel(t)
	m = send(m, keyRune("r"))
	foot := footOf(m.View())
	for _, bad := range []string{"TO INSTALL", "CONFIGS"} {
		if strings.Contains(foot, bad) {
			t.Errorf("rodapé da revisão com contador %q: %q", bad, foot)
		}
	}
}

// I5: na revisão do "tentar de novo", q não pergunta pelas pendências da seleção do
// wizard (o run já aconteceu): sai direto, como no resumo.
func TestRetryReviewQuitIgnoresWizardPending(t *testing.T) {
	m := retryModel(t)
	if n, c := m.pending(); n+c == 0 {
		t.Fatal("o teste precisa de uma seleção com pendências")
	}
	m = send(m, keyRune("r"))
	if m.screen != ScreenReview || m.review.caller != ScreenSummary {
		t.Fatalf("r não abriu a revisão do retry: %v", m.screen)
	}
	nm, cmd := m.Update(keyRune("q"))
	if nm.(Model).confirmQuit || !isQuit(cmd) {
		t.Fatalf("q na revisão do retry deveria sair direto (confirm=%v)", nm.(Model).confirmQuit)
	}
}
