package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/executor"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
	"github.com/muesli/termenv"
)

// logCap é quantas linhas de log cada passo guarda em memória (o log completo fica no journal).
const logCap = 500

// eventMsg é um evento do executor entregue ao Update.
type eventMsg executor.Event

// eventsClosedMsg avisa que o executor terminou e fechou o canal de eventos.
type eventsClosedMsg struct{}

// runDoneMsg é o resultado do Session.Run.
type runDoneMsg struct {
	rep executor.Report
	j   *state.Journal
	err error
}

// interactiveReq pede ao Update que ceda o terminal a cmd; a resposta vai em reply (com buffer 1).
type interactiveReq struct {
	cmd   runner.Cmd
	reply chan runner.Result
}

// interactiveDoneMsg chega quando o ExecProcess termina e o reply já foi escrito.
type interactiveDoneMsg struct{}

// programRef liga o modelo ao *tea.Program. É um ponteiro: o tea.NewProgram recebe uma cópia
// do modelo antes do SetProgram, e as cópias precisam ver o mesmo Send.
type programRef struct {
	mu   sync.Mutex
	send func(tea.Msg)
}

func (r *programRef) set(send func(tea.Msg)) {
	r.mu.Lock()
	r.send = send
	r.mu.Unlock()
}

func (r *programRef) get() func(tea.Msg) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.send
}

// runInteractive é o RunOptions.RunInteractive: roda na goroutine do executor e espera o Update
// responder depois de ceder o terminal.
func (r *programRef) runInteractive(ctx context.Context, c runner.Cmd) runner.Result {
	send := r.get()
	if send == nil {
		return runner.Result{ExitCode: -1, Err: errors.New(i18n.T("tui.execute.no_terminal"))}
	}
	reply := make(chan runner.Result, 1)
	send(interactiveReq{cmd: c, reply: reply})
	select {
	case res := <-reply:
		return res
	case <-ctx.Done():
		return runner.Result{ExitCode: -1, Err: ctx.Err()}
	}
}

// SetProgram liga o modelo ao programa (o hook interativo usa p.Send). Chame antes de p.Run.
func (m Model) SetProgram(p *tea.Program) { m.prog.set(p.Send) }

// FinalMessage é o texto para imprimir depois que a TUI sai (vazio se não houve interrupção).
func (m Model) FinalMessage() string {
	if m.exec.interrupted {
		return i18n.T("msg.interrupted")
	}
	return ""
}

// logRing guarda as últimas logCap linhas de um passo.
type logRing struct {
	buf   []string
	start int
}

func (r *logRing) add(s string) {
	if len(r.buf) < logCap {
		r.buf = append(r.buf, s)
		return
	}
	r.buf[r.start] = s
	r.start = (r.start + 1) % logCap
}

// last devolve as últimas n linhas, em ordem.
func (r *logRing) last(n int) []string {
	if r == nil {
		return nil
	}
	all := append(append([]string{}, r.buf[r.start:]...), r.buf[:r.start]...)
	if n < len(all) {
		all = all[len(all)-n:]
	}
	return all
}

func (r *logRing) lines() []string { return r.last(logCap) }

// cleanLine tira cores e controles do output e fica com o que sobra depois do último \r (barras de progresso).
func cleanLine(s string) string {
	s = strings.TrimRight(s, "\r\n")
	if i := strings.LastIndex(s, "\r"); i >= 0 {
		s = s[i+1:]
	}
	return strings.ReplaceAll(ansi.Strip(s), "\t", "    ")
}

type execState struct {
	steps    []plan.Step
	index    map[string]int
	status   map[string]state.StepStatus
	notes    map[string]string
	logs     map[string]*logRing
	cursor   int
	follow   bool
	current  string // passo em execução que o log segue
	finished int

	started time.Time
	elapsed time.Duration // congelado quando o run termina

	events  <-chan executor.Event
	done    <-chan runDoneMsg
	cancel  context.CancelFunc
	running bool

	interrupted bool
	queue       []interactiveReq // pedidos interativos esperando a vez
	busy        bool             // um ExecProcess em curso
	bar         progress.Model
}

func (e execState) selected() string {
	if e.cursor < 0 || e.cursor >= len(e.steps) {
		return ""
	}
	return e.steps[e.cursor].ID
}

func (e execState) terminal(id string) bool {
	switch e.status[id] {
	case state.StepDone, state.StepFailed, state.StepSkipped:
		return true
	}
	return false
}

// waitEvent lê um evento do executor; o Update o re-agenda a cada eventMsg.
func waitEvent(ch <-chan executor.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return eventsClosedMsg{}
		}
		return eventMsg(ev)
	}
}

// startRun entra na execução de m.plan. O Session.Run roda na goroutine do cmd devolvido
// (nada roda até o Bubble Tea executar o cmd).
func (m Model) startRun() (Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan executor.Event, 256)
	done := make(chan runDoneMsg, 1)

	e := execState{
		index: map[string]int{}, status: map[string]state.StepStatus{}, notes: map[string]string{},
		logs: map[string]*logRing{}, follow: true, started: time.Now(),
		events: events, done: done, cancel: cancel, running: true,
	}
	for _, layer := range m.plan.Order() {
		for _, st := range layer {
			e.index[st.ID] = len(e.steps)
			e.steps = append(e.steps, st)
		}
	}
	opts := []progress.Option{progress.WithoutPercentage(), progress.WithDefaultGradient()}
	if !m.th.Color {
		opts = append(opts, progress.WithColorProfile(termenv.Ascii))
	}
	e.bar = progress.New(opts...)
	m.exec = e
	m.screen = ScreenExecute

	s, pl, ro := m.s, m.plan, app.RunOptions{Events: events, RunInteractive: m.prog.runInteractive, Profile: m.review.profile}
	launch := func() tea.Msg {
		rep, j, err := s.Run(ctx, pl, nil, ro)
		done <- runDoneMsg{rep: rep, j: j, err: err}
		close(events) // depois do done: o eventsClosedMsg sempre encontra o resultado
		return nil
	}
	return m, tea.Batch(launch, waitEvent(events), m.spin.Tick)
}

func (m Model) onEvent(ev eventMsg) (Model, tea.Cmd) {
	e := m.exec
	next := waitEvent(e.events)
	if _, ok := e.index[ev.StepID]; !ok {
		return m, next
	}
	if ev.Line != "" {
		r := e.logs[ev.StepID]
		if r == nil {
			r = &logRing{}
			e.logs[ev.StepID] = r
		}
		r.add(cleanLine(ev.Line))
	}
	if ev.Status != "" {
		wasTerminal := e.terminal(ev.StepID)
		e.status[ev.StepID] = ev.Status
		switch {
		case ev.Note != "":
			e.notes[ev.StepID] = ev.Note
		case ev.Err != nil:
			e.notes[ev.StepID] = ev.Err.Error()
		}
		if ev.Status == state.StepRunning {
			e.current = ev.StepID
		} else if e.terminal(ev.StepID) {
			if !wasTerminal {
				e.finished++
			}
			if e.current == ev.StepID {
				e.current = ""
				for _, st := range e.steps { // segue outro passo ainda em execução, se houver
					if e.status[st.ID] == state.StepRunning {
						e.current = st.ID
					}
				}
			}
		}
	}
	if e.follow && e.current != "" {
		e.cursor = e.index[e.current]
	}
	m.exec = e
	return m, next
}

func (m Model) onRunDone(msg runDoneMsg) (Model, tea.Cmd) {
	m.exec.running = false
	m.exec.elapsed = time.Since(m.exec.started)
	m.runRep, m.runJ, m.runErr = msg.rep, msg.j, msg.err
	m.sum = summaryState{}
	if m.exec.interrupted {
		return m, tea.Quit
	}
	m.screen = ScreenSummary
	return m, nil
}

func (m Model) updateExecute(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := m.exec
	switch {
	case km.String() == "ctrl+c":
		if !e.running {
			return m, tea.Quit
		}
		if !e.interrupted {
			e.interrupted = true
			e.cancel()
			for _, req := range e.queue {
				req.reply <- canceled()
			}
			e.queue = nil
		}
	case key.Matches(km, m.keys.Up):
		if e.cursor > 0 {
			e.cursor--
		}
		e.follow = false
	case key.Matches(km, m.keys.Down):
		if e.cursor < len(e.steps)-1 {
			e.cursor++
		}
		e.follow = false
	case key.Matches(km, m.keys.Follow):
		e.follow = true
		if e.current != "" {
			e.cursor = e.index[e.current]
		}
	}
	m.exec = e
	return m, nil
}

func canceled() runner.Result { return runner.Result{ExitCode: -1, Err: context.Canceled} }

// onInteractive enfileira o pedido: só um ExecProcess por vez (sudo -v e chsh podem chegar juntos).
func (m Model) onInteractive(req interactiveReq) (Model, tea.Cmd) {
	if m.exec.interrupted || !m.exec.running {
		req.reply <- canceled()
		return m, nil
	}
	m.exec.queue = append(m.exec.queue, req)
	return m.nextInteractive()
}

func (m Model) nextInteractive() (Model, tea.Cmd) {
	e := m.exec
	if e.busy || len(e.queue) == 0 {
		return m, nil
	}
	req := e.queue[0]
	e.queue = e.queue[1:]
	if len(runner.ExecArgv(req.cmd)) == 0 {
		req.reply <- runner.Result{ExitCode: -1, Err: errors.New("empty command")}
		m.exec = e
		return m.nextInteractive()
	}
	e.busy = true
	m.exec = e
	return m, tea.ExecProcess(execCommand(req.cmd), interactiveCallback(req.reply))
}

// execCommand monta o processo que recebe o terminal. Sem Setpgid: o filho fica no grupo em
// primeiro plano do terminal (num grupo novo levaria SIGTTIN ao ler a senha e pararia).
func execCommand(c runner.Cmd) *exec.Cmd {
	argv := runner.ExecArgv(c)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	return cmd
}

// interactiveCallback converte o fim do ExecProcess em runner.Result e responde ao executor.
func interactiveCallback(reply chan<- runner.Result) tea.ExecCallback {
	return func(err error) tea.Msg {
		res := runner.Result{}
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr):
			res.ExitCode = exitErr.ExitCode()
		case err != nil:
			res.ExitCode, res.Err = -1, err
		}
		reply <- res
		return interactiveDoneMsg{}
	}
}

func (m Model) stepIcon(id string) string {
	e := m.exec
	switch e.status[id] {
	case state.StepRunning:
		if e.interrupted || !e.running {
			return m.th.Warn.Render(m.th.StepIcon(state.StepRunning))
		}
		return m.th.Accent.Render(m.spin.View())
	case state.StepDone:
		return m.th.OK.Render(m.th.StepIcon(state.StepDone))
	case state.StepFailed:
		return m.th.Danger.Render(m.th.StepIcon(state.StepFailed))
	case state.StepSkipped:
		return m.th.Subtle.Render(m.th.StepIcon(state.StepSkipped))
	}
	return m.th.Subtle.Render(m.th.StepIcon(state.StepPending))
}

// fit corta s em w colunas e completa com espaços.
func fit(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	if n := ansi.StringWidth(s); n < w {
		s += strings.Repeat(" ", w-n)
	}
	return s
}

// executeLines desenha a lista de passos à esquerda e o log do passo escolhido à direita.
func (m Model) executeLines(height int) []string {
	e := m.exec
	head := m.th.Subtle.Render(i18n.T("tui.execute.running"))
	if e.interrupted {
		head = m.th.Warn.Render(i18n.T("tui.execute.interrupted"))
	}
	rows := height - 1
	if rows < 1 {
		return []string{head}
	}
	lw := m.w / 3
	if lw < 24 {
		lw = 24
	}
	if lw > 44 {
		lw = 44
	}
	rw := m.w - lw - 3
	if rw < 1 {
		rw = 1
	}

	top := 0
	if len(e.steps) > rows {
		top = e.cursor - rows/2
		if top > len(e.steps)-rows {
			top = len(e.steps) - rows
		}
		if top < 0 {
			top = 0
		}
	}
	left := make([]string, 0, rows)
	for i := top; i < len(e.steps) && len(left) < rows; i++ {
		st := e.steps[i]
		prefix, label := "  ", st.ID
		if i == e.cursor {
			prefix, label = "> ", m.th.Selected.Render(st.ID)
		}
		text := prefix + m.stepIcon(st.ID) + " " + label
		if n := e.notes[st.ID]; n != "" {
			text += " " + m.th.Subtle.Render(n)
		}
		left = append(left, fit(text, lw))
	}

	sel := e.selected()
	logs := e.logs[sel].last(rows - 1)
	for i, l := range logs {
		logs[i] = ansi.Truncate(l, rw, "…")
	}
	vp := viewport.New(rw, rows-1)
	vp.SetContent(strings.Join(logs, "\n"))
	vp.GotoBottom()
	right := append([]string{m.th.Title.Render(ansi.Truncate(i18n.T("tui.execute.log", sel), rw, "…"))}, strings.Split(vp.View(), "\n")...)

	out := []string{head}
	sep := m.th.Border.Render("│")
	for i := 0; i < rows; i++ {
		l, r := strings.Repeat(" ", lw), ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out = append(out, l+" "+sep+" "+r)
	}
	return out
}

// progressLine é a barra do rodapé: concluídos/total e o tempo decorrido.
func (m Model) progressLine() string {
	e := m.exec
	total := len(e.steps)
	ratio := 0.0
	if total > 0 {
		ratio = float64(e.finished) / float64(total)
	}
	el := e.elapsed
	if e.running {
		el = time.Since(e.started)
	}
	txt := " " + i18n.T("tui.execute.progress", e.finished, total, el.Round(time.Second).String())
	bar := e.bar
	bar.Width = m.w - ansi.StringWidth(txt)
	if bar.Width < 10 {
		bar.Width = 10
	}
	return ansi.Truncate(bar.ViewAs(ratio)+txt, m.w, "")
}
