package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/executor"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
)

// threeSteps: s1 e s3 na primeira camada, s2 depende de s1.
func threeSteps() *plan.Plan {
	cmd := func(s string) []runner.Cmd { return []runner.Cmd{{Argv: []string{"echo", s}}} }
	return &plan.Plan{Steps: []plan.Step{
		{ID: "chsh:s1", Kind: plan.KindChsh, Cmds: cmd("s1")},
		{ID: "chsh:s2", Kind: plan.KindChsh, Cmds: cmd("s2"), DependsOn: []string{"chsh:s1"}},
		{ID: "chsh:s3", Kind: plan.KindChsh, Cmds: cmd("s3")},
	}}
}

// okSession usa um FakeRunner que responde sucesso a tudo.
func okSession(t *testing.T) *app.Session {
	t.Helper()
	c, err := catalog.Load(os.DirFS("../catalog/testdata"))
	if err != nil {
		t.Fatal(err)
	}
	p := linuxSystemd
	p.Home = t.TempDir()
	s, err := app.Open(app.Session{R: &runner.FakeRunner{}, Cat: c, P: p, StateDir: t.TempDir(), Getenv: map[string]string{}, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// executing devolve o modelo na tela de execução sem disparar a goroutine (o cmd é descartado).
func executing(t *testing.T) (Model, tea.Cmd) {
	t.Helper()
	i18n.SetLang(i18n.En)
	m := New(okSession(t), nil, ScreenReview)
	m = send(m, size(120, 40))
	m.plan = threeSteps()
	m, cmd := m.startRun()
	t.Cleanup(m.exec.cancel)
	return m, cmd
}

func ev(step string, st state.StepStatus) tea.Msg {
	return eventMsg(executor.Event{StepID: step, Status: st})
}

func line(step, l string) tea.Msg { return eventMsg(executor.Event{StepID: step, Line: l}) }

func ctrlC() tea.Msg { return tea.KeyMsg{Type: tea.KeyCtrlC} }

func isQuit(cmd tea.Cmd) bool {
	// compara com tea.Quit sem executar o cmd (um waitEvent bloquearia)
	return cmd != nil && reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()
}

func TestExecutionMarksStepsFromEvents(t *testing.T) {
	m, _ := executing(t)
	if m.screen != ScreenExecute {
		t.Fatalf("screen = %v", m.screen)
	}
	m = send(m, ev("chsh:s1", state.StepRunning), ev("chsh:s1", state.StepDone))
	m = send(m, ev("chsh:s3", state.StepRunning), eventMsg(executor.Event{StepID: "chsh:s3", Status: state.StepFailed, Note: "boom"}))
	v := m.View()
	for _, want := range []string{"[ok] chsh:s1", "[x] chsh:s3", "boom", "2/3"} {
		if !strings.Contains(v, want) {
			t.Errorf("View sem %q:\n%s", want, v)
		}
	}
}

func TestLogFollowsRunningStep(t *testing.T) {
	m, _ := executing(t)
	m = send(m, ev("chsh:s1", state.StepRunning), line("chsh:s1", "hello from s1"))
	if !strings.Contains(m.View(), "hello from s1") {
		t.Fatalf("log de s1 ausente:\n%s", m.View())
	}
	m = send(m, ev("chsh:s3", state.StepRunning), line("chsh:s3", "hello from s3"))
	v := m.View()
	if !strings.Contains(v, "hello from s3") || strings.Contains(v, "hello from s1") {
		t.Fatalf("o log não seguiu s3:\n%s", v)
	}
	// ↑ escolhe outro passo e para de seguir; f volta a seguir.
	for m.exec.selected() != "chsh:s1" {
		m = send(m, tea.KeyMsg{Type: tea.KeyUp})
	}
	if m.exec.follow || !strings.Contains(m.View(), "hello from s1") {
		t.Fatalf("↑ não mostrou s1 (follow=%v):\n%s", m.exec.follow, m.View())
	}
	m = send(m, line("chsh:s3", "more s3"))
	if m.exec.selected() != "chsh:s1" {
		t.Fatal("sem follow, o passo escolhido não pode mudar")
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if !m.exec.follow || m.exec.selected() != "chsh:s3" {
		t.Fatalf("f não voltou a seguir: follow=%v sel=%s", m.exec.follow, m.exec.selected())
	}
}

func TestLogBufferBounded(t *testing.T) {
	m, _ := executing(t)
	m = send(m, ev("chsh:s1", state.StepRunning))
	for i := 0; i < 5000; i++ {
		m = send(m, line("chsh:s1", fmt.Sprintf("line %d", i)))
	}
	got := m.exec.logs["chsh:s1"].lines()
	if len(got) != 500 || got[0] != "line 4500" || got[499] != "line 4999" {
		t.Fatalf("buffer: len=%d first=%q last=%q", len(got), got[0], got[len(got)-1])
	}
	start := time.Now()
	v := m.View()
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("View levou %v", d)
	}
	if !strings.Contains(v, "line 4999") {
		t.Fatalf("última linha ausente:\n%s", v)
	}
}

func TestInteractiveRequestUsesExecProcess(t *testing.T) {
	m, _ := executing(t)
	reply := make(chan runner.Result, 1)
	_, cmd := m.Update(interactiveReq{cmd: runner.Cmd{Argv: []string{"sudo", "-v"}, Stdin: true}, reply: reply})
	if cmd == nil {
		t.Fatal("interactiveReq sem tea.Cmd")
	}
	if got := fmt.Sprintf("%T", cmd()); got != "tea.execMsg" {
		t.Fatalf("cmd não é tea.ExecProcess: %s", got)
	}
	if _, ok := interactiveCallback(reply)(nil).(interactiveDoneMsg); !ok {
		t.Fatal("callback deve devolver interactiveDoneMsg")
	}
	if r := <-reply; r.ExitCode != 0 || r.Err != nil {
		t.Fatalf("reply = %+v", r)
	}
	reply2 := make(chan runner.Result, 1)
	interactiveCallback(reply2)(errors.New("no tty"))
	if r := <-reply2; r.OK() {
		t.Fatalf("erro do exec virou sucesso: %+v", r)
	}
}

func TestExecCommandKeepsForegroundAndNoSudoN(t *testing.T) {
	c := execCommand(runner.Cmd{Argv: []string{"chsh", "-s", "/bin/zsh"}, Sudo: true, Stdin: true, Env: []string{"X=1"}})
	if got := strings.Join(c.Args, " "); got != "sudo chsh -s /bin/zsh" {
		t.Fatalf("argv = %q", got)
	}
	if c.SysProcAttr != nil && c.SysProcAttr.Setpgid {
		t.Fatal("comando interativo não pode ir para outro grupo de processos (SIGTTIN)")
	}
	if c.Env[len(c.Env)-1] != "X=1" {
		t.Fatalf("env sem X=1: %v", c.Env[len(c.Env)-3:])
	}
}

func TestInteractiveRequestsSerialized(t *testing.T) {
	m, _ := executing(t)
	r1, r2 := make(chan runner.Result, 1), make(chan runner.Result, 1)
	req1 := interactiveReq{cmd: runner.Cmd{Argv: []string{"sudo", "-v"}, Stdin: true}, reply: r1}
	req2 := interactiveReq{cmd: runner.Cmd{Argv: []string{"chsh", "-s", "/bin/zsh"}, Stdin: true}, reply: r2}
	nm, cmd1 := m.Update(req1)
	m = nm.(Model)
	nm, cmd2 := m.Update(req2)
	m = nm.(Model)
	if cmd1 == nil {
		t.Fatal("primeiro pedido não foi despachado")
	}
	if cmd2 != nil {
		t.Fatal("segundo pedido despachado com o primeiro em curso")
	}
	// O ExecProcess do primeiro termina: o callback responde e só então o segundo sai.
	done := interactiveCallback(r1)(nil)
	select {
	case <-r1:
	default:
		t.Fatal("primeiro sem resposta")
	}
	nm, cmd3 := m.Update(done)
	m = nm.(Model)
	if !dispatched(cmd3) {
		t.Fatal("segundo pedido não foi despachado depois do primeiro")
	}
	select {
	case r := <-r2:
		t.Fatalf("segundo respondido antes do seu exec: %+v", r)
	default:
	}
	nm, cmd4 := m.Update(interactiveCallback(r2)(nil))
	if dispatched(cmd4) || nm.(Model).exec.busy {
		t.Fatal("fila deveria estar vazia e livre")
	}
}

func TestInteractiveHookSendsRequest(t *testing.T) {
	m, _ := executing(t)
	var got runner.Cmd
	m.prog.set(func(msg tea.Msg) {
		req := msg.(interactiveReq)
		got = req.cmd
		req.reply <- runner.Result{ExitCode: 3}
	})
	res := m.prog.runInteractive(context.Background(), runner.Cmd{Argv: []string{"sudo", "-v"}, Stdin: true})
	if res.ExitCode != 3 || got.String() != "sudo -v" {
		t.Fatalf("res=%+v cmd=%q", res, got.String())
	}
	if r := New(okSession(t), nil, ScreenReview).prog.runInteractive(context.Background(), runner.Cmd{}); r.OK() {
		t.Fatal("sem programa ligado o hook deve falhar")
	}
}

func TestCtrlCDuringExecution(t *testing.T) {
	m, _ := executing(t)
	cancelled := 0
	m.exec.cancel = func() { cancelled++ }
	m = send(m, ev("chsh:s1", state.StepRunning))
	pending := make(chan runner.Result, 1)
	busy := make(chan runner.Result, 1)
	nm, _ := m.Update(interactiveReq{cmd: runner.Cmd{Argv: []string{"sudo", "-v"}, Stdin: true}, reply: busy})
	m = nm.(Model)
	m = send(m, interactiveReq{cmd: runner.Cmd{Argv: []string{"chsh"}, Stdin: true}, reply: pending}) // na fila

	nm, cmd := m.Update(ctrlC())
	m = nm.(Model)
	if cancelled != 1 || !m.exec.interrupted {
		t.Fatalf("cancel=%d interrupted=%v", cancelled, m.exec.interrupted)
	}
	if isQuit(cmd) {
		t.Fatal("ctrl+c não pode sair antes do runDoneMsg")
	}
	if !strings.Contains(m.View(), i18n.T("tui.execute.interrupted")) {
		t.Fatalf("sem aviso de interrompido:\n%s", m.View())
	}
	if r := <-pending; r.OK() {
		t.Fatalf("pedido na fila deve ser cancelado: %+v", r)
	}
	nm, cmd = m.Update(interactiveCallback(busy)(nil)) // o que estava no exec termina
	m = nm.(Model)
	if dispatched(cmd) {
		t.Fatal("nada pode ser despachado depois da interrupção")
	}
	// Um novo pedido interativo depois da interrupção é respondido sem ceder o terminal.
	late := make(chan runner.Result, 1)
	nm, cmd = m.Update(interactiveReq{cmd: runner.Cmd{Argv: []string{"chsh"}, Stdin: true}, reply: late})
	m = nm.(Model)
	if cmd != nil {
		t.Fatal("pedido após interrupção não pode abrir ExecProcess")
	}
	if r := <-late; r.OK() {
		t.Fatalf("pedido após interrupção respondeu sucesso: %+v", r)
	}
	// Eventos continuam sendo consumidos (o executor não pode travar no canal).
	nm, cmd = m.Update(ev("chsh:s1", state.StepFailed))
	m = nm.(Model)
	if cmd == nil || isQuit(cmd) {
		t.Fatal("eventos devem continuar sendo lidos depois do ctrl+c")
	}
	nm, cmd = m.Update(ctrlC())
	m = nm.(Model)
	if cancelled != 1 || isQuit(cmd) {
		t.Fatal("segundo ctrl+c não sai nem cancela de novo")
	}
	nm, cmd = m.Update(runDoneMsg{err: context.Canceled})
	m = nm.(Model)
	if !isQuit(cmd) {
		t.Fatal("runDoneMsg após interrupção deve devolver tea.Quit")
	}
	if fm := m.FinalMessage(); fm != i18n.T("msg.interrupted") || !strings.Contains(fm, "kanivet-cli install --resume") {
		t.Fatalf("FinalMessage = %q", fm)
	}
}

func TestExecutionSurvivesResize(t *testing.T) {
	m, _ := executing(t)
	m = send(m, ev("chsh:s1", state.StepRunning), line("chsh:s1", "working"))
	m = send(m, size(70, 20))
	if want := i18n.T("tui.too_small", 70, 20, minWidth, minHeight); !strings.HasPrefix(m.View(), want) {
		t.Fatalf("View = %q, want too-small", m.View())
	}
	m = send(m, line("chsh:s1", strings.Repeat("x", 300)), size(120, 40))
	v := m.View()
	if !strings.Contains(v, "chsh:s1") || !strings.Contains(v, "working") {
		t.Fatalf("não voltou à execução:\n%s", v)
	}
	for _, l := range strings.Split(v, "\n") {
		if w := len([]rune(l)); w > 120 {
			t.Fatalf("linha com %d colunas: %q", w, l)
		}
	}
	m = send(m, size(80, 24))
	if n := len(strings.Split(m.View(), "\n")); n != 24 {
		t.Fatalf("80x24: %d linhas", n)
	}
}

// pump roda os cmds (e Batches) e entrega as mensagens até a tela sair da execução.
func pump(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	deadline := time.After(10 * time.Second)
	queue := []tea.Cmd{cmd}
	for m.screen == ScreenExecute && len(queue) > 0 {
		select {
		case <-deadline:
			t.Fatal("execução não terminou")
		default:
		}
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil, spinner.TickMsg:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			nm, next := m.Update(msg)
			m = nm.(Model)
			queue = append(queue, next)
		}
	}
	return m
}

func TestRunDoneGoesToSummary(t *testing.T) {
	m, cmd := executing(t)
	m = pump(t, m, cmd)
	if m.screen != ScreenSummary {
		t.Fatalf("screen = %v, want summary", m.screen)
	}
	if len(m.runRep.Done) != 3 || m.runJ == nil || m.runErr != nil {
		t.Fatalf("rep=%+v j=%v err=%v", m.runRep, m.runJ, m.runErr)
	}
	if m.exec.finished != 3 {
		t.Fatalf("finished = %d", m.exec.finished)
	}
	if m.FinalMessage() != "" {
		t.Fatalf("FinalMessage sem interrupção = %q", m.FinalMessage())
	}
}

// msgsOf roda o cmd (e os de um Batch/Sequence) e devolve as mensagens produzidas.
func msgsOf(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch r := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range r {
			out = append(out, msgsOf(c)...)
		}
		return out
	default:
		return []tea.Msg{r}
	}
}

// M3: o ReleaseTerminal do ExecProcess desliga o mouse e o RestoreTerminal não religa.
func TestInteractiveDoneReenablesMouse(t *testing.T) {
	m, _ := executing(t)
	m.exec.busy = true
	nm, cmd := m.Update(interactiveDoneMsg{})
	if nm.(Model).exec.busy {
		t.Fatal("busy deveria voltar a false")
	}
	want := tea.EnableMouseCellMotion()
	for _, msg := range msgsOf(cmd) {
		if msg == want {
			return
		}
	}
	t.Fatalf("interactiveDoneMsg deve religar o mouse; msgs = %v", msgsOf(cmd))
}

// dispatched diz se o cmd faz algo além de religar o mouse (ex.: um novo ExecProcess).
func dispatched(cmd tea.Cmd) bool {
	for _, msg := range msgsOf(cmd) {
		if msg != nil && msg != tea.EnableMouseCellMotion() {
			return true
		}
	}
	return false
}
