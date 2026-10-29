package cli

import (
	"fmt"
	"io"

	"github.com/eusener/kanivet-cli/internal/app"
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

// printPlan imprime o plano já agrupado pela sessão.
func printPlan(w io.Writer, pl *plan.Plan, groups []app.Group) {
	if len(pl.Steps) == 0 {
		fmt.Fprintln(w, i18n.T("plan.empty"))
	}
	for _, g := range groups {
		fmt.Fprintln(w, g.Title)
		for _, l := range g.Lines {
			switch g.Kind {
			case app.GroupWarnings:
				fmt.Fprintln(w, "  ! "+l.Text)
				continue
			default:
				fmt.Fprintln(w, "  "+l.Text)
			}
			for _, c := range l.Cmds {
				fmt.Fprintln(w, "      $ "+c)
			}
			for _, f := range l.Files {
				fmt.Fprintln(w, "      "+fileLine(f))
			}
		}
	}
}

func fileLine(f app.FileLine) string {
	if f.Rc {
		line := "→ " + f.Path + "  (" + i18n.T("plan.rc_block") + ")"
		if f.Exists {
			line += " (" + i18n.T("plan.backup") + ")"
		}
		return line
	}
	verb := "→"
	if f.Remove {
		verb = "✗"
	}
	line := verb + " " + f.Path
	if f.Exists && !f.Remove {
		line += "  (" + i18n.T("plan.backup") + ")"
	}
	return line
}
