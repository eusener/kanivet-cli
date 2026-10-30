package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/executor"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
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
	elapsed time.Duration    // congelado quando o run termina
	now     func() time.Time // relógio; o teste o substitui
	began   map[string]time.Time
	took    map[string]time.Duration

	events  <-chan executor.Event
	done    <-chan runDoneMsg
	cancel  context.CancelFunc
	running bool

	interrupted bool
	queue       []interactiveReq // pedidos interativos esperando a vez
	busy        bool             // um ExecProcess em curso
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
		logs: map[string]*logRing{}, follow: true, started: time.Now(), now: time.Now,
		began: map[string]time.Time{}, took: map[string]time.Duration{},
		events: events, done: done, cancel: cancel, running: true,
	}
	for _, layer := range m.plan.Order() {
		for _, st := range layer {
			e.index[st.ID] = len(e.steps)
			e.steps = append(e.steps, st)
		}
	}
	m.exec = e
	m.runRep, m.runJ, m.runErr, m.sum = executor.Report{}, nil, nil, summaryState{} // o resumo seguinte mostra só este run
	m.screen = ScreenExecute

	// Na retomada, o plano pendente continua o journal existente (os passos feitos não rodam de novo).
	s, pl, j, ro := m.s, m.plan, m.review.journal, app.RunOptions{Events: events, RunInteractive: m.prog.runInteractive, Profile: m.review.profile}
	launch := func() tea.Msg {
		rep, j, err := s.Run(ctx, pl, j, ro)
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
		if _, ok := e.began[ev.StepID]; !ok && ev.Status == state.StepRunning {
			e.began[ev.StepID] = e.now()
		}
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
				if t0, ok := e.began[ev.StepID]; ok {
					e.took[ev.StepID] = e.now().Sub(t0)
				}
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
	m.exec.elapsed = m.exec.now().Sub(m.exec.started)
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
		return m.interrupt(), nil
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

// interrupt cancela o run uma vez e responde aos pedidos interativos na fila; a saída
// espera o runDoneMsg. Serve ao ctrl+c e aos sinais externos (interruptMsg).
func (m Model) interrupt() Model {
	e := m.exec
	if !e.running || e.interrupted {
		return m
	}
	e.interrupted = true
	e.cancel()
	for _, req := range e.queue {
		req.reply <- canceled()
	}
	e.queue = nil
	m.exec = e
	return m
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

// fmtDur formata uma duração curta: 3.1s, 12s, 1m12s, 1h02m.
func fmtDur(d time.Duration) string {
	switch {
	case d < 10*time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

// elapsedNow é o tempo decorrido do run (congelado quando ele termina).
func (e execState) elapsedNow() time.Duration {
	if e.running && e.now != nil {
		return e.now().Sub(e.started)
	}
	return e.elapsed
}

// stepGlyph é o ícone do passo; plain=true devolve texto sem estilo (para linhas já estilizadas).
func (m Model) stepGlyph(id string, sel bool) string {
	e := m.exec
	st := e.status[id]
	g := m.th.StepIcon(st)
	if st == state.StepRunning && e.running && !e.interrupted {
		g = m.spin.View()
	}
	if sel {
		return g
	}
	c := colInkMuted
	switch st {
	case state.StepDone:
		c = colOK
	case state.StepFailed:
		c = colErr
	case state.StepRunning:
		c = colInkStrong
	}
	return m.th.Ink(c).Render(g)
}

// progressBar desenha a barra de uma linha: preenchida em sky na proporção feitos/total.
func (m Model) progressBar(w int) string {
	e := m.exec
	w = max(w, 4)
	ratio := 0.0
	if total := len(e.steps); total > 0 {
		ratio = float64(e.finished) / float64(total)
	}
	if !m.th.Paper {
		n := int(ratio*float64(w-2) + 0.5)
		return "[" + strings.Repeat(pick(m.th.Nerd, "█", "#"), n) + strings.Repeat(pick(m.th.Nerd, "░", "-"), w-2-n) + "]"
	}
	n := int(ratio*float64(w) + 0.5)
	on := m.th.R.NewStyle().Background(colSky).Render(strings.Repeat(" ", n))
	off := m.th.R.NewStyle().Background(colCard).Render(strings.Repeat(" ", w-n))
	return on + off
}

// executeHeading é a linha de título: INSTALANDO, a barra e feitos/total · tempo.
func (m Model) executeHeading() string {
	e := m.exec
	cw := m.contentWidth()
	title := m.th.Title.Render(strings.ToUpper(i18n.T("tui.execute.title")))
	sp := m.th.S().Render("  ")
	prog := m.th.S().Render(i18n.T("tui.execute.progress", e.finished, len(e.steps), fmtDur(e.elapsedNow())))
	barW := min(cw-lipgloss.Width(title)-lipgloss.Width(prog)-2*lipgloss.Width(sp), 30)
	if barW < 6 {
		return ansi.Truncate(title+sp+prog, cw, "…")
	}
	return title + sp + m.progressBar(barW) + sp + prog
}

// stepRow desenha uma linha da caixa de passos com a duração à direita.
func (m Model) stepRow(id string, sel bool, w int) string {
	e := m.exec
	name := id
	if sel {
		name = strings.ToUpper(name)
	}
	base := m.th.R.NewStyle()
	if sel {
		base = m.th.Selected
	}
	muted := m.th.Ink(colInkMuted)
	if sel {
		muted = base
	}
	right := ""
	if d, ok := e.took[id]; ok {
		right = fmtDur(d)
	}
	tag := ""
	if e.status[id] == state.StepFailed {
		tag = " " + m.th.Tag(i18n.T("tui.execute.failed"), colCoral)
	}
	lead := base.Render(" ") + m.stepGlyph(id, sel) + base.Render(" ")
	rw := 0
	if right != "" {
		rw = lipgloss.Width(right) + 2 // espaço antes e depois da duração
	}
	room := max(w-lipgloss.Width(lead)-lipgloss.Width(tag)-rw-1, 0)
	text := name
	if n := e.notes[id]; n != "" {
		text += " " + n
	}
	text = ansi.Truncate(text, room, "…")
	head := ansi.Truncate(name, room, "…")
	if lipgloss.Width(text) > lipgloss.Width(head) {
		text = base.Render(head) + muted.Render(ansi.Cut(text, lipgloss.Width(head), lipgloss.Width(text)))
	} else {
		text = base.Render(text)
	}
	line := lead + text + tag
	pad := max(w-lipgloss.Width(line)-rw, 0)
	line += base.Render(strings.Repeat(" ", pad))
	if right != "" {
		line += base.Render(" ") + muted.Render(right) + base.Render(" ")
	}
	return padTo(line, w)
}

// pendingLine é "· próximos · a · b · +N" com os passos que ainda não começaram.
func (m Model) pendingLine(ids []string, w int) string {
	muted := m.th.Ink(colInkMuted)
	line := "· " + i18n.T("tui.execute.next")
	for i, id := range ids {
		rest := len(ids) - i - 1
		cand := line + " · " + id
		tail := ""
		if rest > 0 {
			tail = fmt.Sprintf(" · +%d", rest)
		}
		if lipgloss.Width(" "+cand+tail) > w {
			return padTo(muted.Render(ansi.Truncate(" "+line+fmt.Sprintf(" · +%d", len(ids)-i), w, "…")), w)
		}
		line = cand
	}
	return padTo(muted.Render(" "+line), w)
}

// executeLines desenha título, caixa de passos e caixa de log (a única escura).
func (m Model) executeLines(height int) []string {
	e := m.exec
	cw := m.contentWidth()
	lines := []string{m.executeHeading()}
	for i := 0; i < m.respiro()/2; i++ {
		lines = append(lines, "")
	}
	if e.interrupted {
		lines = append(lines, m.th.Warn.Render(ansi.Truncate(i18n.T("tui.execute.interrupted"), cw, "…")))
	}
	oh := m.th.BoxOverhead()
	avail := height - len(lines) - 2*oh
	if len(e.steps) == 0 || avail < 2 {
		return lines
	}
	logRows := max(avail/3, 2)
	stepRows := max(avail-logRows, 1)
	if want := len(e.steps) + 1; stepRows > want { // lista curta: o que sobra vai ao log
		logRows += stepRows - want
		stepRows = want
	}
	in := m.th.BoxInner(cw)

	// passos: os que já começaram, mais o escolhido; os que faltam viram a linha "próximos"
	var shown, pending []string
	cur := -1
	sel := e.selected()
	for _, st := range e.steps {
		if e.status[st.ID] == "" || e.status[st.ID] == state.StepPending {
			if st.ID != sel {
				pending = append(pending, st.ID)
				continue
			}
		}
		if st.ID == sel {
			cur = len(shown)
		}
		shown = append(shown, st.ID)
	}
	room := stepRows
	if len(pending) > 0 {
		room--
	}
	room = max(room, 1)
	top := 0
	if len(shown) > room {
		top = min(max(cur-room/2, 0), len(shown)-room)
	}
	var rows []string
	for i := top; i < len(shown) && len(rows) < room; i++ {
		rows = append(rows, m.stepRow(shown[i], i == cur, in))
	}
	if len(pending) > 0 && len(rows) < stepRows {
		rows = append(rows, m.pendingLine(pending, in))
	}
	for len(rows) < stepRows {
		rows = append(rows, strings.Repeat(" ", in))
	}
	lines = append(lines, strings.Split(m.th.Box(strings.Join(rows, "\n"), cw, BoxOpts{Flush: true}), "\n")...)

	// log do passo escolhido
	lw := max(m.th.BoxInner(cw)-2, 1)
	logLines := []string{m.th.Ink(colSky).Bold(true).Render(ansi.Truncate(strings.ToUpper(i18n.T("tui.execute.log", sel)), lw, "…"))}
	for _, l := range e.logs[sel].last(logRows - 1) {
		logLines = append(logLines, ansi.Truncate(l, lw, "…"))
	}
	for len(logLines) < logRows {
		logLines = append(logLines, "")
	}
	lines = append(lines, strings.Split(m.th.Box(strings.Join(logLines, "\n"), cw, BoxOpts{Fill: colLogBg, Fg: colLogFg}), "\n")...)
	return lines
}
