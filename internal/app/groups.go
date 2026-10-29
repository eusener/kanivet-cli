package app

import (
	"os"
	"strings"

	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
)

type GroupKind string

const (
	GroupSudo      GroupKind = "sudo"
	GroupInstall   GroupKind = "install"
	GroupUpdate    GroupKind = "update"
	GroupUninstall GroupKind = "uninstall"
	GroupConfig    GroupKind = "config"
	GroupSkip      GroupKind = "skip"
	GroupOrphans   GroupKind = "orphans"
	GroupWarnings  GroupKind = "warnings"
)

// Line é uma linha do plano. Em grupos de passos, Text é "id  (backend)  — nota"; nos demais, o texto puro.
type Line struct {
	StepID, Text string
	Cmds         []string // já compactados (1ª linha + " …")
	Files        []FileLine
}

// FileLine é um arquivo tocado por um passo; Path já vem expandido.
type FileLine struct {
	Path               string
	Remove, Exists, Rc bool
}

type Group struct {
	Kind  GroupKind
	Title string
	Lines []Line
}

// GroupPlan agrupa o plano: sudo, instalar, atualizar, remover, configurar, pular, órfãos e avisos.
// Grupos vazios são omitidos.
func (s *Session) GroupPlan(pl *plan.Plan) []Group {
	defs := []struct {
		kind  GroupKind
		title string
		match func(plan.Step) bool
	}{
		{GroupSudo, i18n.T("plan.sudo"), func(st plan.Step) bool { return st.NeedsSudo }},
		{GroupInstall, i18n.T("plan.install"), func(st plan.Step) bool { return !st.NeedsSudo && st.Kind == plan.KindInstall }},
		{GroupUpdate, i18n.T("plan.update"), func(st plan.Step) bool { return !st.NeedsSudo && st.Kind == plan.KindUpdate }},
		{GroupUninstall, i18n.T("plan.uninstall"), func(st plan.Step) bool { return !st.NeedsSudo && st.Kind == plan.KindUninstall }},
		{GroupConfig, i18n.T("plan.config"), func(st plan.Step) bool {
			return !st.NeedsSudo && (st.Kind == plan.KindConfig || st.Kind == plan.KindShell || st.Kind == plan.KindChsh)
		}},
	}
	var out []Group
	for _, d := range defs {
		g := Group{Kind: d.kind, Title: d.title}
		for _, st := range pl.Steps {
			if d.match(st) {
				g.Lines = append(g.Lines, s.stepLine(st))
			}
		}
		if len(g.Lines) > 0 {
			out = append(out, g)
		}
	}
	if len(pl.Skipped) > 0 {
		g := Group{Kind: GroupSkip, Title: i18n.T("plan.skip")}
		for _, sk := range pl.Skipped {
			g.Lines = append(g.Lines, Line{Text: sk.ItemID + " — " + sk.Reason})
		}
		out = append(out, g)
	}
	if len(pl.Orphans) > 0 {
		out = append(out, Group{Kind: GroupOrphans, Title: i18n.T("plan.orphans"), Lines: []Line{{Text: strings.Join(pl.Orphans, ", ")}}})
	}
	if len(pl.Warnings) > 0 {
		g := Group{Kind: GroupWarnings, Title: i18n.T("plan.warnings")}
		for _, m := range pl.Warnings {
			g.Lines = append(g.Lines, Line{Text: m})
		}
		out = append(out, g)
	}
	return out
}

func (s *Session) stepLine(st plan.Step) Line {
	text := st.ID
	if st.Backend != "" {
		text += "  (" + st.Backend + ")"
	}
	if st.Note != "" {
		text += "  — " + st.Note
	}
	l := Line{StepID: st.ID, Text: text}
	for _, c := range st.Cmds {
		l.Cmds = append(l.Cmds, compact(c.Shown()))
	}
	for _, f := range st.Files {
		target := s.ExpandTarget(f.Target)
		_, err := os.Stat(target)
		l.Files = append(l.Files, FileLine{Path: target, Remove: f.Remove, Exists: err == nil})
	}
	if st.Kind == plan.KindShell {
		rc := s.RcPath()
		_, err := os.Stat(rc)
		l.Files = append(l.Files,
			FileLine{Path: "~/.config/kanivet-cli/shell/init.{zsh,bash}"},
			FileLine{Path: rc, Exists: err == nil, Rc: true})
	}
	return l
}

// compact mostra só a primeira linha de um comando de várias linhas.
func compact(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if first, _, multi := strings.Cut(cmd, "\n"); multi {
		return strings.TrimSpace(first) + " …"
	}
	return cmd
}
