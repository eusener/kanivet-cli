package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

// BoxOpts configura Theme.Box.
type BoxOpts struct {
	Title    string
	Fill     lipgloss.Color // interior; padrão colCard
	Fg       lipgloss.Color // texto; padrão colInk
	NoShadow bool
}

var resetRe = regexp.MustCompile(`\x1b\[(?:0?|49)m`)

// bgSeq devolve a sequência SGR que liga o fundo c no perfil do renderer.
func (t Theme) bgSeq(c lipgloss.Color) string {
	return "\x1b[" + t.R.ColorProfile().Color(string(c)).Sequence(true) + "m"
}

// repaint reaplica o fundo c depois de cada reset SGR (ESC[0m, ESC[m, ESC[49m),
// para que segmentos estilizados não deixem buracos.
func (t Theme) repaint(s string, c lipgloss.Color) string {
	bg := t.bgSeq(c)
	return resetRe.ReplaceAllStringFunc(s, func(m string) string { return m + bg })
}

// fit corta ou completa com espaços até a largura exata w.
func padTo(s string, w int) string {
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "")
	}
	if n := w - lipgloss.Width(s); n > 0 {
		s += strings.Repeat(" ", n)
	}
	return s
}

// Box desenha body numa caixa de largura total width (sombra incluída). Com
// Paper: borda grossa, interior colorido e sombra sólida; caso contrário,
// borda normal sem cor nem sombra.
func (t Theme) Box(body string, width int, o BoxOpts) string {
	shadow := t.Paper && !o.NoShadow
	bw := width
	if shadow {
		bw--
	}
	if bw < 4 {
		bw = 4
	}
	fill, fg := o.Fill, o.Fg
	if fill == "" {
		fill = colCard
	}
	if fg == "" {
		fg = colInk
	}
	b := lipgloss.NormalBorder()
	if t.Paper {
		b = lipgloss.ThickBorder()
	}
	edge := t.S()
	if t.Color {
		edge = edge.Foreground(colInk)
	}
	inner := t.R.NewStyle().Width(bw-2).Padding(0, 1)
	if t.Paper {
		inner = inner.Background(fill).Foreground(fg)
	}
	var rows []string
	for _, ln := range strings.Split(inner.Render(body), "\n") {
		if t.Paper {
			ln = t.repaint(ln, fill)
		}
		rows = append(rows, edge.Render(b.Left)+ln+edge.Render(b.Right))
	}
	top := b.Top
	title := ""
	if o.Title != "" {
		title = " " + strings.ToUpper(o.Title) + " "
		if lipgloss.Width(title) > bw-4 {
			title = ansi.Truncate(title, bw-4, "")
		}
	}
	fillTop := strings.Repeat(top, bw-2-1-lipgloss.Width(title))
	if title == "" {
		fillTop = strings.Repeat(top, bw-2)
		rows = append([]string{edge.Render(b.TopLeft + fillTop + b.TopRight)}, rows...)
	} else {
		head := edge.Render(b.TopLeft+top) + edge.Bold(true).Render(title) + edge.Render(fillTop+b.TopRight)
		rows = append([]string{head}, rows...)
	}
	rows = append(rows, edge.Render(b.BottomLeft+strings.Repeat(b.Bottom, bw-2)+b.BottomRight))
	if !shadow {
		return strings.Join(rows, "\n")
	}
	// Caixa deslocada 1 coluna à direita; a sombra ocupa a coluna 0 (da 2ª
	// linha em diante) e uma linha extra embaixo.
	for i := range rows {
		lead := edge.Render("▐")
		if i == 0 {
			lead = t.S().Render(" ")
		}
		rows[i] = lead + rows[i]
	}
	last := edge.Render("▝"+strings.Repeat("▀", width-2)) + t.S().Render(" ")
	return strings.Join(append(rows, last), "\n")
}

// Tag desenha texto em maiúsculas sobre cor chapada; no modo plain, [TEXTO].
func (t Theme) Tag(text string, bg lipgloss.Color) string {
	up := strings.ToUpper(text)
	if !t.Paper {
		return t.S().Render("[" + up + "]")
	}
	return t.S().Background(bg).Foreground(colInkStrong).Bold(true).Render(" " + up + " ")
}

// Button desenha a tecla-botão da ação principal: "ENTER  INSTALAR →".
func (t Theme) Button(key, label string) string {
	txt := strings.ToUpper(key) + "  " + strings.ToUpper(label) + " " + pick(t.Nerd, "→", "->")
	if !t.Paper {
		return t.Selected.Render(" " + txt + " ")
	}
	return t.S().Background(colSky).Foreground(colInkStrong).Bold(true).Render(" " + txt + " ")
}

// Trail desenha a trilha de 5 passos; step vai de 0 a 4 e done marca todos como concluídos.
func (t Theme) Trail(step int, done bool) string {
	names := []string{
		i18n.T("tui.trail.welcome"), i18n.T("tui.trail.profile"), i18n.T("tui.trail.items"),
		i18n.T("tui.trail.review"), i18n.T("tui.trail.install"),
	}
	// sem Nerd Font a marca é "[x]" (larga): um espaço basta, a trilha cabe em 80 colunas
	sep := t.Subtle.Render(pick(t.Nerd, " ─ ", " "))
	parts := make([]string, len(names))
	for i, n := range names {
		up := strings.ToUpper(n)
		switch {
		case done || i < step:
			parts[i] = t.OK.Render(pick(t.Nerd, "✓", "x")) + " " + t.Subtle.Render(up)
		case i == step:
			parts[i] = t.Tag(n, colSky)
		default:
			parts[i] = t.Subtle.Render(up)
		}
	}
	return strings.Join(parts, sep)
}

// Tabs desenha abas numa linha de largura width; a ativa vai em sun.
func (t Theme) Tabs(names []string, active, width int) string {
	var parts []string
	for i, n := range names {
		if i == active {
			parts = append(parts, t.Tag(n, colSun))
			continue
		}
		parts = append(parts, t.Subtle.Render(" "+strings.ToUpper(n)+" "))
	}
	line := strings.Join(parts, t.S().Render(" "))
	if lipgloss.Width(line) > width {
		line = ansi.Truncate(line, width, "")
	}
	if n := width - lipgloss.Width(line); n > 0 {
		line += t.S().Render(strings.Repeat(" ", n))
	}
	return line
}

// Canvas devolve exatamente h linhas de largura w; no modo Paper toda célula
// recebe o fundo creme, inclusive depois dos resets de estilos internos.
func (t Theme) Canvas(lines []string, w, h int) string {
	var all []string
	for _, l := range lines {
		all = append(all, strings.Split(l, "\n")...)
	}
	out := make([]string, h)
	for i := range out {
		ln := ""
		if i < len(all) {
			ln = all[i]
		}
		ln = padTo(ln, w)
		if t.Paper {
			ln = t.bgSeq(colPaper) + t.repaint(ln, colPaper) + "\x1b[0m"
		}
		out[i] = ln
	}
	return strings.Join(out, "\n")
}
