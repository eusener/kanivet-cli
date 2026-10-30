package tui

import (
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
	// Flush tira o recuo lateral: o corpo ocupa a largura toda entre as bordas, para
	// que linhas selecionadas vão de borda a borda.
	Flush bool
}

// bgSeq devolve a sequência SGR que liga o fundo c no perfil do renderer.
func (t Theme) bgSeq(c lipgloss.Color) string {
	if s, ok := t.seqs[c]; ok {
		return s[0]
	}
	return sgr(t.R.ColorProfile(), c, true)
}

// fgSeq devolve a sequência SGR que liga a cor de texto c no perfil do renderer.
func (t Theme) fgSeq(c lipgloss.Color) string {
	if s, ok := t.seqs[c]; ok {
		return s[1]
	}
	return sgr(t.R.ColorProfile(), c, false)
}

// resetLen devolve o tamanho do reset SGR que começa em s[i:] (ESC[0m, ESC[m, ESC[39m ou
// ESC[49m), ou 0 se não houver um ali.
func resetLen(s string, i int) int {
	r := s[i+2:]
	switch {
	case strings.HasPrefix(r, "m"):
		return 3
	case strings.HasPrefix(r, "0m"):
		return 4
	case strings.HasPrefix(r, "39m"), strings.HasPrefix(r, "49m"):
		return 5
	}
	return 0
}

// repaint reaplica o fundo bg e a tinta fg depois de cada reset SGR, para que segmentos
// estilizados não deixem buracos nem texto na cor padrão do terminal.
func (t Theme) repaint(s string, bg, fg lipgloss.Color) string {
	i := strings.Index(s, "\x1b[")
	if i < 0 {
		return s
	}
	restore := t.bgSeq(bg) + t.fgSeq(fg)
	var b strings.Builder
	b.Grow(len(s) + 8*len(restore))
	last := 0
	for i >= 0 {
		if n := resetLen(s, i); n > 0 {
			b.WriteString(s[last : i+n])
			b.WriteString(restore)
			last = i + n
			i += n
		} else {
			i += 2
		}
		j := strings.Index(s[i:], "\x1b[")
		if j < 0 {
			break
		}
		i += j
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
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
	edge := t.S() // no Paper já vem com a tinta; nas 16 cores a borda fica na cor padrão
	inner := t.R.NewStyle().Width(bw-2).Padding(0, 1)
	if o.Flush {
		inner = t.R.NewStyle().Width(bw - 2)
	}
	if t.Paper {
		inner = inner.Background(fill).Foreground(fg)
	}
	var rows []string
	for _, ln := range strings.Split(inner.Render(body), "\n") {
		if t.Paper {
			ln = t.repaint(ln, fill, fg)
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

// Tabs desenha abas numa linha de largura width; a ativa vai em sun. Quando não cabem
// todas, rolam mantendo a ativa visível, com "…" nas pontas que ficaram de fora.
func (t Theme) Tabs(names []string, active, width int) string {
	n := len(names)
	if n == 0 {
		return t.S().Render(strings.Repeat(" ", max(width, 0)))
	}
	active = min(max(active, 0), n-1)
	cell := func(i int) string {
		if i == active {
			return t.Tag(names[i], colSun)
		}
		return t.Subtle.Render(" " + strings.ToUpper(names[i]) + " ")
	}
	cells := make([]string, n)
	for i := range cells {
		cells[i] = cell(i)
	}
	// largura da janela [lo,hi], com a reserva de "… " / " …" nas pontas cortadas
	span := func(lo, hi int) int {
		w := hi - lo // separadores
		for i := lo; i <= hi; i++ {
			w += lipgloss.Width(cells[i])
		}
		if lo > 0 {
			w += 2
		}
		if hi < n-1 {
			w += 2
		}
		return w
	}
	lo, hi := active, active
	for grew := true; grew; {
		grew = false
		if hi < n-1 && span(lo, hi+1) <= width {
			hi++
			grew = true
		}
		if lo > 0 && span(lo-1, hi) <= width {
			lo--
			grew = true
		}
	}
	sp := t.S().Render(" ")
	var line string
	if lo > 0 {
		line += t.Subtle.Render("…") + sp
	}
	line += strings.Join(cells[lo:hi+1], sp)
	if hi < n-1 {
		line += sp + t.Subtle.Render("…")
	}
	if lipgloss.Width(line) > width {
		line = ansi.Truncate(line, width, "…")
	}
	if k := width - lipgloss.Width(line); k > 0 {
		line += t.S().Render(strings.Repeat(" ", k))
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
			ln = t.bgSeq(colPaper) + t.fgSeq(colInk) + t.repaint(ln, colPaper, colInk) + "\x1b[0m"
		}
		out[i] = ln
	}
	return strings.Join(out, "\n")
}

// Ink devolve um estilo sem o fundo creme (para texto dentro de caixas). Fora do Paper
// as tintas neutras não viram cor (seriam preto em 16 cores): ink-muted fica esmaecido
// e as demais ficam na cor padrão do terminal.
func (t Theme) Ink(c lipgloss.Color) lipgloss.Style {
	s := t.R.NewStyle()
	switch {
	case !t.Color:
	case t.Paper || !inkColor(c):
		s = s.Foreground(c)
	case c == colInkMuted:
		s = s.Faint(true)
	}
	return s
}

// BoxInner é a largura útil (de borda a borda) de uma caixa de largura total w.
func (t Theme) BoxInner(w int) int {
	n := w - 2
	if t.Paper {
		n--
	}
	return max(n, 1)
}

// BoxOverhead é o que uma caixa gasta além das linhas de conteúdo: bordas e, no modo
// Paper, a linha da sombra.
func (t Theme) BoxOverhead() int {
	if t.Paper {
		return 3
	}
	return 2
}
