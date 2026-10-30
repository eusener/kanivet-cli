package tui

import (
	"context"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/executor"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
)

func TestStartScreenFirstRun(t *testing.T) {
	if got := StartScreen(&state.State{Items: map[string]state.InstalledItem{}}); got != ScreenWelcome {
		t.Fatalf("estado vazio: %v, want welcome", got)
	}
	if got := StartScreen(nil); got != ScreenWelcome {
		t.Fatalf("estado nil: %v, want welcome", got)
	}
}

func TestStartScreenDashboard(t *testing.T) {
	if got := StartScreen(&state.State{Items: map[string]state.InstalledItem{"fzf": {}}}); got != ScreenDashboard {
		t.Fatalf("com itens: %v, want dashboard", got)
	}
	if got := StartScreen(&state.State{Profile: "web", Items: map[string]state.InstalledItem{}}); got != ScreenDashboard {
		t.Fatalf("com perfil: %v, want dashboard", got)
	}
}

// unfinishedSession cria um journal inacabado de threeSteps com chsh:s1 já concluído.
func unfinishedSession(t *testing.T) (*app.Session, *state.Journal) {
	t.Helper()
	s := okSession(t)
	data, err := threeSteps().MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	j, err := state.NewJournal(s.StateDir, s.Now(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Set("chsh:s1", state.StepDone); err != nil {
		t.Fatal(err)
	}
	return s, j
}

func resumeModel(t *testing.T, s *app.Session) Model {
	t.Helper()
	i18n.SetLang(i18n.En)
	m, err := initialModel(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	return send(m, size(100, 30))
}

func TestResumeOpensPendingReview(t *testing.T) {
	s, j := unfinishedSession(t)
	m := resumeModel(t, s)
	if m.screen != ScreenReview || !m.review.resume {
		t.Fatalf("screen = %v resume=%v, want review de retomada", m.screen, m.review.resume)
	}
	v := m.View()
	if title := i18n.T("tui.resume_title", j.RunID); !strings.Contains(v, title) {
		t.Fatalf("sem título de retomada %q:\n%s", title, v)
	}
	var ids []string
	for _, st := range m.review.pl.Steps {
		ids = append(ids, st.ID)
	}
	if strings.Join(ids, ",") != "chsh:s2,chsh:s3" {
		t.Fatalf("passos = %v, want só os pendentes", ids)
	}
	if strings.Contains(v, "chsh:s1") {
		t.Fatalf("passo concluído aparece na revisão:\n%s", v)
	}
	m = send(m, size(80, 24))
	if lines := strings.Split(m.View(), "\n"); len(lines) != 24 {
		t.Fatalf("80x24: %d linhas", len(lines))
	}
}

func TestWithoutUnfinishedStartsAtStartScreen(t *testing.T) {
	s := okSession(t)
	m := resumeModel(t, s)
	if m.screen != ScreenWelcome || m.review.resume {
		t.Fatalf("screen = %v, want welcome", m.screen)
	}
}

// I2: com a detecção inicial ainda rodando, o enter da retomada não pode começar o run
// (o detectCmd e o executor tocariam o estado ao mesmo tempo).
func TestResumeReviewIgnoresEnterWhileDetecting(t *testing.T) {
	s, _ := unfinishedSession(t)
	m := resumeModel(t, s)
	if !m.detecting {
		t.Fatal("a retomada abre com a detecção em curso")
	}
	nm, cmd := m.Update(enter())
	m = nm.(Model)
	if m.screen != ScreenReview || cmd != nil {
		t.Fatalf("enter durante a detecção: screen=%v cmd=%v, want review sem cmd", m.screen, cmd != nil)
	}
	for _, b := range m.bindings() {
		if b.Help().Key == "enter" && b.Enabled() {
			t.Fatal("o atalho enter deveria sumir enquanto detecta")
		}
	}
	m = send(m, detectMsg{})
	if nm, _ := m.Update(enter()); nm.(Model).screen != ScreenExecute {
		t.Fatalf("depois da detecção o enter retoma: screen=%v", nm.(Model).screen)
	}
}

func TestResumeRunsWithExistingJournal(t *testing.T) {
	s, j := unfinishedSession(t)
	m := send(resumeModel(t, s), detectMsg{})
	nm, cmd := m.Update(enter())
	m = nm.(Model)
	if m.screen != ScreenExecute {
		t.Fatalf("screen = %v, want execute", m.screen)
	}
	m = pump(t, m, cmd)
	if m.runJ == nil || m.runJ.RunID != j.RunID {
		t.Fatalf("run usou outro journal: %v, want %s", m.runJ, j.RunID)
	}
	for _, c := range s.R.(*runner.FakeRunner).Strings() {
		if strings.Contains(c, "echo s1") {
			t.Fatalf("passo concluído rodou de novo: %v", s.R.(*runner.FakeRunner).Strings())
		}
	}
	if len(m.runRep.Done) != 2 {
		t.Fatalf("done = %v, want s2 e s3", m.runRep.Done)
	}
	if uj, _, _ := s.Unfinished(); uj != nil {
		t.Fatalf("o journal retomado devia terminar: %s", uj.RunID)
	}
}

func TestResumeEscDiscardsWithoutDeletingJournal(t *testing.T) {
	s, j := unfinishedSession(t)
	m := resumeModel(t, s)
	m = send(m, esc())
	if m.screen != ScreenWelcome {
		t.Fatalf("esc: screen = %v, want welcome (estado vazio)", m.screen)
	}
	if uj, _, err := s.Unfinished(); err != nil || uj == nil || uj.RunID != j.RunID {
		t.Fatalf("descartar não pode apagar o journal: %v %v", uj, err)
	}

	s2, _ := unfinishedSession(t)
	s2.St.Items["fzf"] = state.InstalledItem{}
	m = resumeModel(t, s2)
	nm, cmd := m.Update(esc())
	m = nm.(Model)
	if m.screen != ScreenDashboard || cmd == nil {
		t.Fatalf("esc com itens: screen = %v, want dashboard com detecção", m.screen)
	}
}

func TestFilterRoutesSignalsIntoInterrupt(t *testing.T) {
	for _, sig := range []tea.Msg{tea.InterruptMsg{}, tea.QuitMsg{}} {
		m, _ := executing(t)
		cancelled := 0
		m.exec.cancel = func() { cancelled++ }
		msg := signalFilter(m, sig)
		if _, ok := msg.(interruptMsg); !ok {
			t.Fatalf("%T durante o run virou %T, want interruptMsg", sig, msg)
		}
		nm, cmd := m.Update(msg)
		m = nm.(Model)
		if cancelled != 1 || !m.exec.interrupted || isQuit(cmd) {
			t.Fatalf("%T: cancel=%d interrupted=%v quit=%v", sig, cancelled, m.exec.interrupted, isQuit(cmd))
		}
		nm, cmd = m.Update(runDoneMsg{err: context.Canceled})
		m = nm.(Model)
		if !isQuit(cmd) {
			t.Fatalf("%T: runDoneMsg deve sair", sig)
		}
		// Com o run terminado, o filtro deixa o sinal passar (o programa sai).
		if got := signalFilter(m, sig); got != sig {
			t.Fatalf("%T depois do run virou %T", sig, got)
		}
	}
}

func TestFilterPassesSignalsOutsideRun(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	for _, sig := range []tea.Msg{tea.InterruptMsg{}, tea.QuitMsg{}} {
		if got := signalFilter(m, sig); got != sig {
			t.Fatalf("%T fora do run virou %T", sig, got)
		}
	}
	if got, ok := signalFilter(m, enter()).(tea.KeyMsg); !ok || got.Type != tea.KeyEnter {
		t.Fatal("teclas passam intactas")
	}
}

func TestStopRunCancelsAndDrains(t *testing.T) {
	m, _ := executing(t)
	events := make(chan executor.Event) // sem buffer: o executor trava no envio se ninguém ler
	m.exec.events = events
	ctx, cancel := context.WithCancel(context.Background())
	m.exec.cancel = cancel
	go func() { // executor falso: manda eventos até o cancel e então fecha o canal
		for {
			select {
			case events <- executor.Event{StepID: "chsh:s1", Line: "x"}:
			case <-ctx.Done():
				close(events)
				return
			}
		}
	}()
	done := make(chan struct{})
	go func() { m.stopRun(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stopRun não terminou")
	}
	if ctx.Err() == nil {
		t.Fatal("stopRun não cancelou o contexto")
	}
	m.exec.running = false
	m.stopRun() // sem run em curso é um no-op
}

// M1: o Bubble Tea para de ouvir depois do primeiro sinal; um segundo SIGINT/SIGTERM no meio do
// run mataria o processo (terminal cru, filhos órfãos). O nosso encaminhador segue ouvindo.
func TestForwardSignalsKeepsListening(t *testing.T) {
	sigs := make(chan os.Signal, 4)
	for _, s := range []os.Signal{syscall.SIGINT, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		sigs <- s
	}
	var got []tea.Msg
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		forwardSignals(sigs, func(msg tea.Msg) {
			got = append(got, msg)
			if len(got) == 4 {
				close(stop)
			}
		}, stop)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("forwardSignals parou de ouvir: %v", got)
	}
	want := []tea.Msg{tea.InterruptMsg{}, tea.InterruptMsg{}, tea.QuitMsg{}, tea.QuitMsg{}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("msgs = %v, want %v", got, want)
		}
	}
	if !slices.Contains(runSignals, os.Signal(syscall.SIGINT)) || !slices.Contains(runSignals, os.Signal(syscall.SIGTERM)) || !slices.Contains(runSignals, os.Signal(syscall.SIGHUP)) {
		t.Fatalf("runSignals = %v, want SIGINT, SIGTERM e SIGHUP", runSignals)
	}
}

// M1: o segundo sinal no meio do run passa pelo mesmo caminho e o interrupt é idempotente.
func TestSecondSignalDuringRunIsIdempotent(t *testing.T) {
	m, _ := executing(t)
	cancelled := 0
	m.exec.cancel = func() { cancelled++ }
	for _, sig := range []tea.Msg{tea.InterruptMsg{}, tea.QuitMsg{}, tea.InterruptMsg{}} {
		msg := signalFilter(m, sig)
		if _, ok := msg.(interruptMsg); !ok {
			t.Fatalf("%T durante o run virou %T", sig, msg)
		}
		nm, cmd := m.Update(msg)
		m = nm.(Model)
		if isQuit(cmd) {
			t.Fatal("sinal repetido não pode sair antes do runDoneMsg")
		}
	}
	if cancelled != 1 {
		t.Fatalf("cancel = %d, want 1", cancelled)
	}
}

// Com o terminal cedido (sudo/chsh), o ctrl+c vai para o filho: o SIGINT que também chega aqui
// é ignorado, como o Bubble Tea fazia.
func TestFilterDropsSigintWhileInteractive(t *testing.T) {
	m, _ := executing(t)
	m.exec.busy = true
	if got := signalFilter(m, tea.InterruptMsg{}); got != nil {
		t.Fatalf("SIGINT com o terminal cedido virou %T, want descartado", got)
	}
	if _, ok := signalFilter(m, tea.QuitMsg{}).(interruptMsg); !ok {
		t.Fatal("SIGTERM com o terminal cedido ainda interrompe o run")
	}
}
