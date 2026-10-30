package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/eusener/kanivet-cli/internal/app"
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

// selectedFor e extrasFor delegam à sessão (mantidos para os testes da CLI).
func selectedFor(e *Env, sel plan.Selection) map[string]bool {
	return app.SelectedFor(e.Cat, e.P, sel)
}

func (e *Env) extrasFor(profile string) []string {
	s := e.session()
	return s.ExtrasFor(profile)
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
			s, err := e.open()
			if err != nil {
				return err
			}
			pl, _, err := s.Plan(context.Background(), sel)
			if err != nil {
				return exit(1, err.Error())
			}
			printPlan(e.Stdout, pl, s.GroupPlan(pl))
			return nil
		},
	}
	f.bind(cmd)
	return cmd
}

type runFlags struct{ yes, dryRun, forceConfigs, resume bool }

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
			return e.apply(r, func(s *app.Session) (*plan.Plan, error) {
				pl, _, err := s.Plan(context.Background(), sel)
				return pl, err
			}, sel.Profile, true)
		},
	}
	f.bind(cmd)
	r.bind(cmd)
	cmd.Flags().BoolVar(&r.resume, "resume", false, i18n.T("flag.resume"))
	return cmd
}

// apply é o fluxo comum de install/update/uninstall: retomar ou planejar, mostrar, confirmar, executar, resumir.
// Só o install retoma, e só com --resume ou confirmação interativa depois de mostrar o que falta.
func (e *Env) apply(r runFlags, build func(*app.Session) (*plan.Plan, error), profile string, allowResume bool) error {
	if !r.yes && !r.dryRun && !e.IsTTY {
		return exit(2, i18n.T("err.non_interactive"))
	}
	s, err := e.open()
	if err != nil {
		return err
	}
	var pl *plan.Plan
	var j *state.Journal
	if !r.dryRun {
		unfinished, pending, err := s.Unfinished()
		if err != nil {
			return err
		}
		if unfinished != nil {
			switch {
			case !allowResume:
				fmt.Fprintln(e.Stdout, i18n.T("msg.unfinished_hint", unfinished.RunID))
			case r.resume || (!r.yes && e.IsTTY):
				fmt.Fprintln(e.Stdout, i18n.T("msg.pending", unfinished.RunID))
				printPlan(e.Stdout, pending, s.GroupPlan(pending))
				if r.resume || e.confirm(i18n.T("prompt.resume", unfinished.RunID)) {
					pl, j = pending, unfinished
					fmt.Fprintln(e.Stdout, i18n.T("msg.resuming", unfinished.RunID))
				}
			default:
				fmt.Fprintln(e.Stdout, i18n.T("msg.unfinished_hint", unfinished.RunID))
			}
		}
	}
	if pl == nil {
		if pl, err = build(s); err != nil {
			return exit(1, err.Error())
		}
		printPlan(e.Stdout, pl, s.GroupPlan(pl))
		if r.dryRun || len(pl.Steps) == 0 {
			return nil
		}
		if !r.yes && !e.confirm(i18n.T("prompt.proceed")) {
			return exit(1, i18n.T("msg.aborted"))
		}
	}
	return e.execute(s, j, pl, r, profile)
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

func (e *Env) execute(s *app.Session, j *state.Journal, pl *plan.Plan, r runFlags, profile string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
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
	rep, j, err := s.Run(ctx, pl, j, app.RunOptions{ForceConfigs: r.forceConfigs, Events: events, Profile: profile})
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
