package tui

import (
	"fmt"
	"sort"

	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
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

func (m Model) welcomeLines() []string {
	p := m.s.P
	out := []string{m.th.Subtle.Render(i18n.T("tui.welcome.intro")), ""}
	if m.detecting {
		return append(out, m.spin.View()+" "+i18n.T("tui.welcome.detecting"))
	}
	out = append(out,
		fmt.Sprintf("%-18s %s %s · %s", i18n.T("tui.welcome.system")+":", osName(p), p.DistroVersion, p.Arch),
		fmt.Sprintf("%-18s %s", i18n.T("tui.welcome.shell")+":", p.Shell),
	)
	if p.PkgManager != "" {
		out = append(out, fmt.Sprintf("%-18s %s", i18n.T("tui.welcome.pkg")+":", p.PkgManager))
	}
	if w := m.platformWarnings(); len(w) > 0 {
		out = append(out, "")
		for _, s := range w {
			out = append(out, m.th.Warn.Render("! "+s))
		}
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
