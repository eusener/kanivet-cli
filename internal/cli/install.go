package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/eusener/kanivet-cli/internal/configs"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/executor"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/state"
)

type selFlags struct {
	profile                         string
	add, remove, versions, noConfig []string
	zsh                             bool
}

func (f *selFlags) bind(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.profile, "profile", "", i18n.T("flag.profile"))
	fl.StringSliceVar(&f.add, "add", nil, i18n.T("flag.add"))
	fl.StringSliceVar(&f.remove, "remove", nil, i18n.T("flag.remove"))
	fl.StringArrayVar(&f.versions, "version", nil, i18n.T("flag.version"))
	fl.StringSliceVar(&f.noConfig, "no-config", nil, i18n.T("flag.no_config"))
	fl.BoolVar(&f.zsh, "zsh", false, i18n.T("flag.zsh"))
}

func (f *selFlags) selection(args []string) (plan.Selection, error) {
	sel := plan.Selection{Profile: f.profile, Add: append(append([]string{}, f.add...), args...), Remove: f.remove,
		Versions: map[string]string{}, Configs: map[string]bool{}, SwitchToZsh: f.zsh}
	for _, v := range f.versions {
		item, line, ok := strings.Cut(v, "=")
		if !ok {
			return sel, exit(1, i18n.T("err.bad_version", v))
		}
		sel.Versions[item] = line
	}
	for _, c := range f.noConfig {
		sel.Configs[c] = false
	}
	return sel, nil
}

// selectedFor aproxima o conjunto escolhido, para o detector saber o que fornece o quê (php pelo lerd).
func selectedFor(e *Env, sel plan.Selection) map[string]bool {
	out := map[string]bool{}
	for _, p := range e.Cat.Profiles {
		if p.ID != sel.Profile {
			continue
		}
		for _, id := range p.Items {
			out[id] = true
		}
		excl := map[string]string{}
		for cat, id := range p.Exclusive {
			excl[cat] = id
		}
		for cat, id := range p.ExclusiveByOS[e.P.OS] {
			excl[cat] = id
		}
		for _, id := range excl {
			out[id] = true
		}
	}
	for _, id := range sel.Add {
		if it := e.Cat.Items[id]; it != nil {
			for other := range out { // um Add explícito substitui o padrão da categoria exclusiva
				if e.Cat.Items[other] != nil && e.Cat.Items[other].Category == it.Category && isExclusive(e, it.Category) {
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

func isExclusive(e *Env, cat string) bool {
	for _, c := range e.Cat.Categories {
		if c.ID == cat {
			return c.Exclusive
		}
	}
	return false
}

func (e *Env) buildPlan(ctx context.Context, st *state.State, sel plan.Selection) (*plan.Plan, map[string]detect.ItemStatus, error) {
	statuses := e.detector(st).Detect(ctx, e.Cat, selectedFor(e, sel))
	pl, err := plan.Build(e.Cat, e.P, statuses, sel)
	return pl, statuses, err
}

func planCmd(e *Env) *cobra.Command {
	var f selFlags
	cmd := &cobra.Command{
		Use:   "plan [item...]",
		Short: i18n.T("cli.plan"),
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := f.selection(args)
			if err != nil {
				return err
			}
			st, err := e.loadState()
			if err != nil {
				return err
			}
			pl, _, err := e.buildPlan(context.Background(), st, sel)
			if err != nil {
				return exit(1, err.Error())
			}
			printPlan(e.Stdout, pl, e.expandTarget)
			return nil
		},
	}
	f.bind(cmd)
	return cmd
}

type runFlags struct{ yes, dryRun, forceConfigs bool }

func (r *runFlags) bind(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&r.yes, "yes", "y", false, i18n.T("flag.yes"))
	cmd.Flags().BoolVar(&r.dryRun, "dry-run", false, i18n.T("flag.dry_run"))
	cmd.Flags().BoolVar(&r.forceConfigs, "force-configs", false, i18n.T("flag.force_configs"))
}

func installCmd(e *Env) *cobra.Command {
	var f selFlags
	var r runFlags
	cmd := &cobra.Command{
		Use:   "install [item...]",
		Short: i18n.T("cli.install"),
		RunE: func(cmd *cobra.Command, args []string) error {
			sel, err := f.selection(args)
			if err != nil {
				return err
			}
			return e.apply(r, func(st *state.State) (*plan.Plan, error) {
				pl, _, err := e.buildPlan(context.Background(), st, sel)
				return pl, err
			}, sel.Profile)
		},
	}
	f.bind(cmd)
	r.bind(cmd)
	return cmd
}

// apply é o fluxo comum de install/update/uninstall: retomar ou planejar, mostrar, confirmar, executar, resumir.
func (e *Env) apply(r runFlags, build func(*state.State) (*plan.Plan, error), profile string) error {
	if !r.yes && !r.dryRun && !e.IsTTY {
		return exit(2, i18n.T("err.non_interactive"))
	}
	st, err := e.loadState()
	if err != nil {
		return err
	}
	var pl *plan.Plan
	var j *state.Journal
	if !r.dryRun {
		if unfinished, data, err := state.LatestUnfinished(e.StateDir); err == nil && unfinished != nil {
			if r.yes || e.confirm(i18n.T("prompt.resume", unfinished.RunID)) {
				if pl, err = plan.Decode(data); err != nil {
					return err
				}
				j = unfinished
				fmt.Fprintln(e.Stdout, i18n.T("msg.resuming", unfinished.RunID))
			}
		}
	}
	if pl == nil {
		if pl, err = build(st); err != nil {
			return exit(1, err.Error())
		}
		printPlan(e.Stdout, pl, e.expandTarget)
		if r.dryRun || len(pl.Steps) == 0 {
			return nil
		}
		if !r.yes && !e.confirm(i18n.T("prompt.proceed")) {
			return exit(1, i18n.T("msg.aborted"))
		}
		data, err := pl.MarshalJSON()
		if err != nil {
			return err
		}
		if j, err = state.NewJournal(e.StateDir, e.Now(), data); err != nil {
			return err
		}
	}
	if profile != "" {
		st.Profile = profile
	}
	return e.execute(st, j, pl, r, e.extrasFor(st.Profile))
}

// extrasFor devolve os extras do LazyVim do perfil (renderizados no lazyvim.json).
func (e *Env) extrasFor(profile string) []string {
	for _, p := range e.Cat.Profiles {
		if p.ID == profile {
			return p.Extras["lazyvim"]
		}
	}
	return nil
}

func (e *Env) confirm(question string) bool {
	if !e.IsTTY {
		return false
	}
	fmt.Fprint(e.Stdout, question+" [y/N] ")
	line, _ := bufio.NewReader(e.Stdin).ReadString('\n')
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes" || ans == "s" || ans == "sim"
}

func (e *Env) execute(st *state.State, j *state.Journal, pl *plan.Plan, r runFlags, extras []string) error {
	vars := configs.VarsFor(e.P, e.Getenv)
	vars.Extras = extras
	ex := &executor.Executor{
		R: e.R, J: j, St: st, Cat: e.Cat, P: e.P, Det: e.detector(st),
		Ap:       &configs.Applier{St: st, BackupDir: filepath.Join(e.StateDir, "backups", j.RunID), Vars: vars, Files: e.Cat.Files},
		ShellDir: e.shellDir(), Rc: e.rcPath(),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	events := make(chan executor.Event, 64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ev := range events {
			if ev.Status == state.StepDone || ev.Status == state.StepFailed || ev.Status == state.StepSkipped {
				line := e.icon(ev.Status) + " " + ev.StepID
				if ev.Note != "" {
					line += "  — " + ev.Note
				}
				fmt.Fprintln(e.Stdout, line)
			}
		}
	}()
	rep, err := ex.Execute(ctx, pl, executor.Options{ForceConfigs: r.forceConfigs, Events: events})
	close(events)
	wg.Wait()
	if errors.Is(err, context.Canceled) {
		return exit(130, i18n.T("msg.interrupted"))
	}
	if err != nil {
		return err
	}
	return e.summary(j, pl, rep)
}

func (e *Env) summary(j *state.Journal, pl *plan.Plan, rep executor.Report) error {
	fmt.Fprintln(e.Stdout)
	fmt.Fprintln(e.Stdout, i18n.T("summary.counts", len(rep.Done), len(rep.Failed), len(rep.Skipped)))
	for _, id := range rep.Failed {
		s, _ := pl.Step(id)
		name := s.ItemID
		if name == "" {
			name = strings.ReplaceAll(id, ":", "-")
		}
		fmt.Fprintf(e.Stdout, "  %s %s — %s\n", e.icon(state.StepFailed), id, i18n.T("summary.log", j.LogPath(name)))
	}
	if len(rep.Failed) > 0 {
		fmt.Fprintln(e.Stdout, i18n.T("summary.retry"))
	}
	for _, c := range rep.Conflicts {
		fmt.Fprintln(e.Stdout, i18n.T("summary.conflict", c.Target))
	}
	if _, ok := pl.Step(plan.ShellStepID); ok && len(rep.Failed) == 0 {
		fmt.Fprintln(e.Stdout, i18n.T("summary.new_terminal"))
	}
	if len(rep.Failed) > 0 {
		return exit(1, "")
	}
	return nil
}

func (e *Env) expandTarget(target string) string {
	ap := &configs.Applier{Vars: configs.VarsFor(e.P, e.Getenv)}
	if out, err := ap.ExpandTarget(target); err == nil {
		return out
	}
	return target
}
