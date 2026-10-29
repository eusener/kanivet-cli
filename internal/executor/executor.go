// Package executor executa um plano: em paralelo, tolerante a falhas e retomável pelo journal.
package executor

import (
	"bufio"
	"context"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/configs"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/shellinit"
	"github.com/eusener/kanivet-cli/internal/state"
)

type Event struct {
	StepID, ItemID string
	Status         state.StepStatus
	Line           string // uma linha de output (ou o comando, no dry-run)
	Note           string
	Err            error
}

type Options struct {
	Concurrency  int // padrão 4
	DryRun       bool
	ForceConfigs bool
	Events       chan<- Event
}

type Report struct {
	Done, Failed, Skipped []string
	Conflicts             []configs.Conflict
}

type Executor struct {
	R        runner.Runner
	J        *state.Journal
	St       *state.State
	Ap       *configs.Applier
	Cat      *catalog.Catalog
	P        platform.Platform
	Det      *detect.Detector
	ShellDir string // ~/.config/kanivet-cli/shell
	Rc       string // ~/.zshrc ou ~/.bashrc
}

const sudoKeepAlive = 60 * time.Second

type run struct {
	e      *Executor
	o      Options
	mu     sync.Mutex
	rep    Report
	bad    map[string]bool // passos que falharam ou foram pulados
	locks  map[string]*sync.Mutex
	sudoOK bool
}

func (e *Executor) Execute(ctx context.Context, pl *plan.Plan, o Options) (Report, error) {
	if o.Concurrency <= 0 {
		o.Concurrency = 4
	}
	r := &run{e: e, o: o, bad: map[string]bool{}, locks: map[string]*sync.Mutex{}, sudoOK: true}
	stopSudo := r.startSudo(ctx, pl)
	defer stopSudo()

	for _, layer := range pl.Order() {
		sem := make(chan struct{}, o.Concurrency)
		var wg sync.WaitGroup
		for _, s := range layer {
			if ctx.Err() != nil {
				break
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(s plan.Step) {
				defer wg.Done()
				defer func() { <-sem }()
				r.step(ctx, s)
			}(s)
		}
		wg.Wait()
		if ctx.Err() != nil {
			return r.rep, ctx.Err()
		}
	}
	if !o.DryRun && e.J != nil {
		if err := e.J.Finish(); err != nil {
			return r.rep, err
		}
	}
	return r.rep, nil
}

// startSudo pede a senha uma vez (sudo -v) se o plano tiver passos com sudo pendentes.
func (r *run) startSudo(ctx context.Context, pl *plan.Plan) func() {
	if r.o.DryRun {
		return func() {}
	}
	need := false
	for _, s := range pl.Steps {
		if s.NeedsSudo && r.journal(s.ID) != state.StepDone {
			need = true
		}
	}
	if !need {
		return func() {}
	}
	if res := r.e.R.Run(ctx, runner.Cmd{Argv: []string{"sudo", "-v"}, Stdin: true}, io.Discard); !res.OK() {
		r.sudoOK = false
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(sudoKeepAlive)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				r.e.R.Run(ctx, runner.Cmd{Argv: []string{"sudo", "-n", "-v"}}, io.Discard)
			}
		}
	}()
	return func() { close(done) }
}

func (r *run) journal(id string) state.StepStatus {
	if r.e.J == nil {
		return ""
	}
	return r.e.J.Get(id)
}

func (r *run) emit(ev Event) {
	if r.o.Events != nil {
		r.o.Events <- ev
	}
}

func (r *run) finish(s plan.Step, st state.StepStatus, note string, err error) {
	r.mu.Lock()
	switch st {
	case state.StepDone:
		r.rep.Done = append(r.rep.Done, s.ID)
	case state.StepFailed:
		r.rep.Failed = append(r.rep.Failed, s.ID)
		r.bad[s.ID] = true
	case state.StepSkipped:
		r.rep.Skipped = append(r.rep.Skipped, s.ID)
		r.bad[s.ID] = true
	}
	r.mu.Unlock()
	if !r.o.DryRun && r.e.J != nil {
		_ = r.e.J.Set(s.ID, st)
	}
	r.emit(Event{StepID: s.ID, ItemID: s.ItemID, Status: st, Note: note, Err: err})
}

func (r *run) step(ctx context.Context, s plan.Step) {
	prev := r.journal(s.ID)
	if prev == state.StepDone {
		r.finish(s, state.StepDone, i18n.T("note.already_done"), nil)
		return
	}
	r.mu.Lock()
	blocked := false
	for _, d := range s.DependsOn {
		blocked = blocked || r.bad[d]
	}
	r.mu.Unlock()
	// O shell init só inclui o que está presente: uma falha não pode deixar o dev sem nenhum init.
	if blocked && s.Kind != plan.KindShell {
		r.finish(s, state.StepSkipped, i18n.T("note.dependency_failed"), nil)
		return
	}
	if s.NeedsSudo && !r.sudoOK {
		r.finish(s, state.StepSkipped, i18n.T("note.sudo_denied"), nil)
		return
	}
	if r.o.DryRun {
		for _, c := range s.Cmds {
			r.emit(Event{StepID: s.ID, ItemID: s.ItemID, Line: c.Shown()})
		}
		for _, f := range s.Files {
			r.emit(Event{StepID: s.ID, ItemID: s.ItemID, Line: f.Target})
		}
		r.finish(s, state.StepDone, "", nil)
		return
	}

	_ = r.e.J.Set(s.ID, state.StepRunning)
	r.emit(Event{StepID: s.ID, ItemID: s.ItemID, Status: state.StepRunning})
	logName := s.ItemID
	if logName == "" {
		logName = strings.ReplaceAll(s.ID, ":", "-")
	}
	logw, err := r.e.J.LogWriter(logName)
	if err != nil {
		r.finish(s, state.StepFailed, "", err)
		return
	}
	defer logw.Close()
	out := r.lineWriter(s, logw)
	defer out.Close()

	switch s.Kind {
	case plan.KindInstall:
		if it := r.e.Cat.Items[s.ItemID]; it != nil && r.e.Det.DetectItem(ctx, it).Status.Present() {
			if prev == state.StepRunning { // começamos esta instalação antes de uma interrupção
				r.own(s)
			}
			r.finish(s, state.StepDone, i18n.T("note.already_present"), nil)
			return
		}
		if !r.runCmds(ctx, s, out) {
			return
		}
		r.own(s)
	case plan.KindUpdate, plan.KindChsh:
		if !r.runCmds(ctx, s, out) {
			return
		}
		if s.Kind == plan.KindUpdate {
			r.own(s)
		}
	case plan.KindUninstall:
		if !r.runCmds(ctx, s, out) {
			return
		}
		r.mu.Lock()
		delete(r.e.St.Items, s.ItemID)
		for _, f := range s.Files {
			if err := r.e.Ap.Remove(f.Target); err != nil {
				io.WriteString(out, err.Error()+"\n")
			}
		}
		err := r.e.St.Save()
		r.mu.Unlock()
		if err != nil {
			r.finish(s, state.StepFailed, "", err)
			return
		}
	case plan.KindConfig:
		r.mu.Lock()
		var conflict *configs.Conflict
		var err error
		for _, f := range s.Files {
			if f.Dir {
				if err = r.e.Ap.ApplyDir(f); err != nil {
					break
				}
				continue
			}
			var c *configs.Conflict
			if c, err = r.e.Ap.Apply(f, r.o.ForceConfigs); err != nil {
				break
			}
			if c != nil {
				conflict = c
				r.rep.Conflicts = append(r.rep.Conflicts, *c)
			}
		}
		if err == nil {
			err = r.e.St.Save()
		}
		r.mu.Unlock()
		if err != nil {
			r.finish(s, state.StepFailed, "", err)
			return
		}
		if conflict != nil {
			r.finish(s, state.StepSkipped, i18n.T("note.config_conflict", conflict.Target), nil)
			return
		}
	case plan.KindShell:
		if err := r.writeShell(ctx); err != nil {
			r.finish(s, state.StepFailed, "", err)
			return
		}
	}
	r.finish(s, state.StepDone, "", nil)
}

// runCmds roda os comandos em ordem; devolve false se falhou ou foi cancelado.
func (r *run) runCmds(ctx context.Context, s plan.Step, out io.Writer) bool {
	for _, c := range s.Cmds {
		res := r.withLock(c.Lock, func() runner.Result { return r.e.R.Run(ctx, c, out) })
		if ctx.Err() != nil {
			return false // fica "running" no journal para retomar
		}
		if !res.OK() {
			r.finish(s, state.StepFailed, c.Shown(), res.Err)
			return false
		}
	}
	return true
}

func (r *run) withLock(name string, fn func() runner.Result) runner.Result {
	if name == "" {
		return fn()
	}
	r.mu.Lock()
	l, ok := r.locks[name]
	if !ok {
		l = &sync.Mutex{}
		r.locks[name] = l
	}
	r.mu.Unlock()
	l.Lock()
	defer l.Unlock()
	return fn()
}

func (r *run) own(s plan.Step) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.e.St.Items[s.ItemID] = state.InstalledItem{Backend: s.Backend, Package: s.Package, At: time.Now().UTC()}
	_ = r.e.St.Save()
}

// writeShell gera init.zsh e init.bash com os itens presentes que têm integração de shell.
func (r *run) writeShell(ctx context.Context) error {
	var items []*catalog.Item
	ids := make([]string, 0, len(r.e.Cat.Items))
	for id := range r.e.Cat.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		it := r.e.Cat.Items[id]
		if len(it.Shell.Aliases) == 0 && len(it.Shell.Init) == 0 && len(it.Shell.Env) == 0 && len(it.Env) == 0 {
			continue
		}
		r.mu.Lock()
		owned := r.e.St.Owned(id)
		r.mu.Unlock()
		if owned || r.e.Det.DetectItem(ctx, it).Status.Present() {
			items = append(items, it)
		}
	}
	for _, sh := range []string{"zsh", "bash"} {
		path := filepath.Join(r.e.ShellDir, "init."+sh)
		if err := state.WriteFileAtomic(path, []byte(shellinit.Generate(items, sh)), 0o644); err != nil {
			return err
		}
	}
	_, err := configs.EnsureRcBlock(r.e.Rc, r.e.P.Shell)
	return err
}

// lineWriter manda o output para o log e, linha a linha, para os eventos.
func (r *run) lineWriter(s plan.Step, log io.Writer) io.WriteCloser {
	pr, pw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			io.WriteString(log, line+"\n")
			r.emit(Event{StepID: s.ID, ItemID: s.ItemID, Line: line})
		}
		io.Copy(io.Discard, pr)
	}()
	return &pipeCloser{pw: pw, done: done}
}

type pipeCloser struct {
	pw   *io.PipeWriter
	done chan struct{}
}

func (p *pipeCloser) Write(b []byte) (int, error) { return p.pw.Write(b) }
func (p *pipeCloser) Close() error {
	err := p.pw.Close()
	<-p.done
	return err
}
