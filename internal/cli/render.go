package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/state"
)

func (e *Env) icon(s state.StepStatus) string {
	plain := map[state.StepStatus]string{state.StepDone: "[ok]", state.StepFailed: "[x]", state.StepSkipped: "[skip]", state.StepRunning: "[..]"}
	fancy := map[state.StepStatus]string{state.StepDone: "✓", state.StepFailed: "✗", state.StepSkipped: "↷", state.StepRunning: "…"}
	if e.plain() {
		return plain[s]
	}
	return fancy[s]
}

// printPlan mostra o plano agrupado: sudo, instalar, configurar, atualizar, remover, pular, avisos.
// expand resolve o destino real de um arquivo (variáveis e ~).
func printPlan(w io.Writer, pl *plan.Plan, expand func(string) string) {
	groups := []struct {
		title string
		match func(plan.Step) bool
	}{
		{i18n.T("plan.sudo"), func(s plan.Step) bool { return s.NeedsSudo }},
		{i18n.T("plan.install"), func(s plan.Step) bool { return !s.NeedsSudo && s.Kind == plan.KindInstall }},
		{i18n.T("plan.update"), func(s plan.Step) bool { return !s.NeedsSudo && s.Kind == plan.KindUpdate }},
		{i18n.T("plan.uninstall"), func(s plan.Step) bool { return !s.NeedsSudo && s.Kind == plan.KindUninstall }},
		{i18n.T("plan.config"), func(s plan.Step) bool {
			return !s.NeedsSudo && (s.Kind == plan.KindConfig || s.Kind == plan.KindShell || s.Kind == plan.KindChsh)
		}},
	}
	if len(pl.Steps) == 0 {
		fmt.Fprintln(w, i18n.T("plan.empty"))
	}
	for _, g := range groups {
		var lines []string
		for _, s := range pl.Steps {
			if !g.match(s) {
				continue
			}
			head := "  " + s.ID
			if s.Backend != "" {
				head += "  (" + s.Backend + ")"
			}
			if s.Note != "" {
				head += "  — " + s.Note
			}
			lines = append(lines, head)
			for _, c := range s.Cmds {
				lines = append(lines, "      $ "+compact(c.Shown()))
			}
			for _, f := range s.Files {
				verb := "→"
				if f.Remove {
					verb = "✗"
				}
				target := expand(f.Target)
				line := "      " + verb + " " + target
				if _, err := os.Stat(target); err == nil && !f.Remove {
					line += "  (" + i18n.T("plan.backup") + ")"
				}
				lines = append(lines, line)
			}
			if s.Kind == plan.KindShell {
				lines = append(lines, "      → ~/.config/kanivet-cli/shell/init.{zsh,bash}")
			}
		}
		if len(lines) > 0 {
			fmt.Fprintln(w, g.title)
			fmt.Fprintln(w, strings.Join(lines, "\n"))
		}
	}
	if len(pl.Skipped) > 0 {
		fmt.Fprintln(w, i18n.T("plan.skip"))
		for _, s := range pl.Skipped {
			fmt.Fprintf(w, "  %s — %s\n", s.ItemID, s.Reason)
		}
	}
	if len(pl.Orphans) > 0 {
		fmt.Fprintln(w, i18n.T("plan.orphans"))
		fmt.Fprintln(w, "  "+strings.Join(pl.Orphans, ", "))
	}
	if len(pl.Warnings) > 0 {
		fmt.Fprintln(w, i18n.T("plan.warnings"))
		for _, m := range pl.Warnings {
			fmt.Fprintln(w, "  ! "+m)
		}
	}
}

// compact mostra só a primeira linha de um comando de várias linhas.
func compact(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if first, _, multi := strings.Cut(cmd, "\n"); multi {
		return strings.TrimSpace(first) + " …"
	}
	return cmd
}
