// Package app é a sessão que orquestra o motor (detectar, planejar, executar), compartilhada pela CLI e pela TUI.
package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/configs"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/executor"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
)

type Session struct {
	R        runner.Runner
	P        platform.Platform
	Cat      *catalog.Catalog
	StateDir string
	Getenv   map[string]string
	Now      func() time.Time
	St       *state.State // carregado por Open
}

// Open carrega o estado e devolve a sessão pronta.
func Open(s Session) (*Session, error) {
	st, err := state.Load(filepath.Join(s.StateDir, "state.json"))
	if err != nil {
		return nil, err
	}
	s.St = st
	return &s, nil
}

func (s *Session) detector() *detect.Detector {
	return &detect.Detector{R: s.R, P: s.P, Owner: s.St}
}

// Statuses detecta o estado de todos os itens do catálogo.
func (s *Session) Statuses(ctx context.Context, sel plan.Selection) map[string]detect.ItemStatus {
	return s.detector().Detect(ctx, s.Cat, SelectedFor(s.Cat, s.P, sel))
}

// Plan detecta e monta o plano da seleção.
func (s *Session) Plan(ctx context.Context, sel plan.Selection) (*plan.Plan, map[string]detect.ItemStatus, error) {
	statuses := s.Statuses(ctx, sel)
	pl, err := plan.Build(s.Cat, s.P, statuses, sel)
	return pl, statuses, err
}

// Unfinished devolve o run mais recente não finalizado com o plano só dos passos pendentes, ou nils se não houver.
func (s *Session) Unfinished() (*state.Journal, *plan.Plan, error) {
	j, data, err := state.LatestUnfinished(s.StateDir)
	if err != nil || j == nil {
		return nil, nil, err
	}
	pl, err := plan.Decode(data)
	if err != nil {
		return nil, nil, err
	}
	return j, pendingSteps(pl, j), nil
}

// pendingSteps devolve o plano salvo sem os passos já concluídos.
func pendingSteps(pl *plan.Plan, j *state.Journal) *plan.Plan {
	out := &plan.Plan{Skipped: pl.Skipped, Warnings: pl.Warnings}
	for _, st := range pl.Steps {
		if j.Get(st.ID) != state.StepDone {
			out.Steps = append(out.Steps, st)
		}
	}
	return out
}

type RunOptions struct {
	ForceConfigs bool
	Events       chan<- executor.Event
	// RunInteractive roda um comando com o terminal cedido (sudo, prompts).
	RunInteractive func(context.Context, runner.Cmd) runner.Result
	Profile        string // gravado no estado antes de executar, se não vazio
}

// Run executa o plano. j nil cria um journal novo com o plano; senão retoma j.
func (s *Session) Run(ctx context.Context, pl *plan.Plan, j *state.Journal, o RunOptions) (executor.Report, *state.Journal, error) {
	if j == nil {
		data, err := pl.MarshalJSON()
		if err != nil {
			return executor.Report{}, nil, err
		}
		if j, err = state.NewJournal(s.StateDir, s.Now(), data); err != nil {
			return executor.Report{}, nil, err
		}
	}
	if o.Profile != "" {
		s.St.Profile = o.Profile
	}
	vars := configs.VarsFor(s.P, s.Getenv)
	vars.Extras = s.ExtrasFor(s.St.Profile)
	ex := &executor.Executor{
		R: s.R, J: j, St: s.St, Cat: s.Cat, P: s.P, Det: s.detector(),
		Ap:       &configs.Applier{St: s.St, BackupDir: filepath.Join(s.StateDir, "backups", j.RunID), Vars: vars, Files: s.Cat.Files},
		ShellDir: s.ShellDir(), Rc: s.RcPath(),
	}
	rep, err := ex.Execute(ctx, pl, executor.Options{ForceConfigs: o.ForceConfigs, Events: o.Events, RunInteractive: o.RunInteractive})
	return rep, j, err
}

func (s *Session) RcPath() string {
	if s.P.Shell == "zsh" {
		return filepath.Join(s.P.Home, ".zshrc")
	}
	return filepath.Join(s.P.Home, ".bashrc")
}

func (s *Session) ShellDir() string {
	return filepath.Join(s.P.Home, ".config", "kanivet-cli", "shell")
}

// ExpandTarget resolve variáveis e ~ no destino de um arquivo.
func (s *Session) ExpandTarget(target string) string {
	ap := &configs.Applier{Vars: configs.VarsFor(s.P, s.Getenv)}
	if out, err := ap.ExpandTarget(target); err == nil {
		return out
	}
	return target
}

// ExtrasFor devolve os extras do LazyVim do perfil (renderizados no lazyvim.json).
func (s *Session) ExtrasFor(profile string) []string {
	for _, p := range s.Cat.Profiles {
		if p.ID == profile {
			return p.Extras["lazyvim"]
		}
	}
	return nil
}

// SelectedFor aproxima o conjunto escolhido, para o detector saber o que fornece o quê (php pelo lerd).
func SelectedFor(c *catalog.Catalog, p platform.Platform, sel plan.Selection) map[string]bool {
	out := map[string]bool{}
	for _, pr := range c.Profiles {
		if pr.ID != sel.Profile {
			continue
		}
		for _, id := range pr.Items {
			out[id] = true
		}
		excl := map[string]string{}
		for cat, id := range pr.Exclusive {
			excl[cat] = id
		}
		for cat, id := range pr.ExclusiveByOS[p.OS] {
			excl[cat] = id
		}
		for _, id := range excl {
			out[id] = true
		}
	}
	for _, id := range sel.Add {
		if it := c.Items[id]; it != nil {
			for other := range out { // um Add explícito substitui o padrão da categoria exclusiva
				if c.Items[other] != nil && c.Items[other].Category == it.Category && isExclusive(c, it.Category) {
					delete(out, other)
				}
			}
		}
		out[id] = true
	}
	for _, id := range sel.Remove {
		delete(out, id)
	}
	return out
}

func isExclusive(c *catalog.Catalog, cat string) bool {
	for _, k := range c.Categories {
		if k.ID == cat {
			return k.Exclusive
		}
	}
	return false
}
