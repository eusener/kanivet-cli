package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/backend"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/platform"
)

func osName(p platform.Platform) string {
	if p.OS == platform.MacOS {
		return "macOS"
	}
	if p.Distro != "" {
		return p.Distro
	}
	return "Linux"
}

// sudoCount conta os itens da base comum (o perfil blank, que todo perfil inclui)
// cujo método escolhido, ou o próprio item, pede sudo.
func (m Model) sudoCount() int {
	n := 0
	for id := range app.SelectedFor(m.s.Cat, m.s.P, plan.Selection{Profile: blankProfile}) {
		it := m.s.Cat.Items[id]
		if it == nil {
			continue
		}
		if meth, _, err := backend.Choose(it, m.s.P); err == nil && (it.NeedsSudo || meth.NeedsSudo) {
			n++
		}
	}
	return n
}

func (m Model) welcomeLines() []string {
	cw := m.contentWidth()
	p := m.s.P
	row := func(s string) string { return ansi.Truncate(s, cw, "…") }
	out := append([]string{""}, m.welcomeHead(cw)...)
	out = append(out, "")
	if m.detecting {
		return append(out, m.spin.View()+" "+i18n.T("tui.welcome.detecting"))
	}
	boxW := min(cw, 52)
	inner := m.boxInner(boxW)
	muted := m.th.Ink(colInkMuted)
	label := func(k string) string {
		return muted.Render(fmt.Sprintf("%-10s", strings.ToUpper(i18n.T(k))))
	}
	rows := []string{
		label("tui.welcome.system") + osName(p) + " " + p.DistroVersion + " · " + p.Arch,
		label("tui.welcome.shell") + p.Shell,
	}
	if p.PkgManager != "" {
		r := label("tui.welcome.pkg") + p.PkgManager
		if n := m.sudoCount(); n > 0 {
			r += "   " + m.th.Tag(i18n.N("tui.welcome.sudo", n), colSun)
		}
		rows = append(rows, r)
	}
	for i := range rows {
		rows[i] = ansi.Truncate(rows[i], inner, "…")
	}
	out = append(out, strings.Split(m.th.Box(strings.Join(rows, "\n"), boxW, BoxOpts{}), "\n")...)
	if w := m.platformWarnings(); len(w) > 0 {
		out = append(out, "")
		for _, s := range w {
			out = append(out, row(m.th.Warn.Render("! "+s)))
		}
	}
	return out
}

// logoGap é o espaço entre o símbolo e a manchete.
const logoGap = 3

// welcomeHead monta a manchete com a introdução, à direita do símbolo e centradas na
// altura dele; sem símbolo (modo plain), só o texto.
func (m Model) welcomeHead(cw int) []string {
	logo := m.th.Logo()
	lead := 0
	if len(logo) > 0 {
		lead = logoWidth + logoGap
	}
	tw := max(cw-lead, 1)
	wrap := func(s string) []string { return strings.Split(ansi.Wrap(s, tw, ""), "\n") }
	text := []string{
		m.th.Title.Render(strings.ToUpper(i18n.T("tui.welcome.title1"))),
		m.th.Title.Render(strings.ToUpper(i18n.T("tui.welcome.title2"))),
		"",
	}
	for _, l := range wrap(i18n.T("tui.welcome.intro")) {
		text = append(text, m.th.S().Render(l))
	}
	for _, l := range wrap(i18n.T("tui.welcome.promise")) {
		text = append(text, m.th.Subtle.Render(l))
	}
	if len(logo) == 0 {
		return text
	}
	top := max((len(logo)-len(text))/2, 0)
	gap := strings.Repeat(" ", logoGap)
	blank := strings.Repeat(" ", logoWidth)
	out := make([]string, max(len(logo), top+len(text)))
	for i := range out {
		l := blank
		if i < len(logo) {
			l = logo[i]
		}
		if k := i - top; k >= 0 && k < len(text) {
			l += gap + text[k]
		}
		out[i] = l
	}
	return out
}

// platformWarnings deriva os avisos dos itens sem suporte nesta plataforma.
func (m Model) platformWarnings() []string {
	var ids []string
	for id, x := range m.statuses {
		if x.Status == detect.StatusUnsupported {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var systemd, graphical bool
	for _, id := range ids {
		it := m.s.Cat.Items[id]
		if it == nil {
			continue
		}
		if it.Requires.Systemd && !m.s.P.Systemd {
			systemd = true
		}
		if it.Requires.Graphical && it.Category == "terminal" && !m.s.P.Graphical {
			graphical = true
		}
	}
	var out []string
	if systemd {
		out = append(out, i18n.T("tui.welcome.no_systemd"))
	}
	if graphical {
		out = append(out, i18n.T("tui.welcome.no_graphical"))
	}
	return out
}
