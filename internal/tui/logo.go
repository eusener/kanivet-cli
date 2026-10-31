package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Cores do símbolo que não estão na paleta da TUI.
const (
	colLogoBar   = lipgloss.Color("#ece6df")
	colLogoCoral = lipgloss.Color("#ff7169")
	colLogoTeal  = lipgloss.Color("#16aa98")
)

// logoPixels é o miolo da janela do símbolo: o prompt (p), o K (k) e o cursor (c).
// Cada linha de células junta duas linhas de pixels com meios-blocos.
var logoPixels = [6]string{
	"p...kk..kk......",
	"pp..kk.kk.......",
	".pp.kkkk........",
	".pp.kkkk........",
	"pp..kk.kk.......",
	"p...kk..kk.cccc.",
}

var logoInk = map[byte]lipgloss.Color{'p': colLogoTeal, 'k': colInkStrong, 'c': colSun}

// logoWidth é a largura do símbolo, sombra incluída.
const logoWidth = len("p...kk..kk......") + 2 + 2 + 1

// Logo desenha o símbolo do kanivet-cli (conceito B): uma janela de terminal com o
// prompt, o K e o cursor, com as cores e a sombra do logo. Só existe no modo Paper:
// sem cor, os meios-blocos do desenho se confundiriam com sombra.
func (t Theme) Logo() []string {
	if !t.Paper {
		return nil
	}
	b := lipgloss.ThickBorder()
	inner := len(logoPixels[0]) + 2
	edge := t.S()
	paint := func(s string, fg, bg lipgloss.Color) string {
		return t.R.NewStyle().Foreground(fg).Background(bg).Render(s)
	}
	frame := func(l, fill, r string) string { return edge.Render(l + strings.Repeat(fill, inner) + r) }

	bar := paint(" ", colInk, colLogoBar)
	for i, c := range []lipgloss.Color{colLogoCoral, colSun, colLogoTeal} {
		if i > 0 {
			bar += paint(" ", colInk, colLogoBar)
		}
		bar += paint("■", c, colLogoBar)
	}
	bar += paint(strings.Repeat(" ", inner-6), colInk, colLogoBar)

	rows := []string{
		frame(b.TopLeft, b.Top, b.TopRight),
		edge.Render(b.Left) + bar + edge.Render(b.Right),
		frame(b.MiddleLeft, b.Top, b.MiddleRight),
	}
	for j := 0; j < len(logoPixels); j += 2 {
		ln := paint(" ", colInk, colCard)
		for i := range logoPixels[j] {
			top, bot := logoPixels[j][i], logoPixels[j+1][i]
			switch {
			case top == '.' && bot == '.':
				ln += paint(" ", colInk, colCard)
			case top == bot:
				ln += paint("█", logoInk[top], colCard)
			case bot == '.':
				ln += paint("▀", logoInk[top], colCard)
			case top == '.':
				ln += paint("▄", logoInk[bot], colCard)
			default:
				ln += paint("▀", logoInk[top], logoInk[bot])
			}
		}
		ln += paint(" ", colInk, colCard)
		rows = append(rows, edge.Render(b.Left)+ln+edge.Render(b.Right))
	}
	rows = append(rows, frame(b.BottomLeft, b.Bottom, b.BottomRight))
	// A sombra do logo fica embaixo e à esquerda, como a das caixas.
	shade, blank := edge.Render("▐"), edge.Render(" ")
	for i := range rows {
		lead := shade
		if i == 0 {
			lead = blank
		}
		rows[i] = lead + rows[i]
	}
	return append(rows, edge.Render("▝"+strings.Repeat("▀", logoWidth-2))+blank)
}

// Mark é a versão de uma linha do símbolo para o cabeçalho, na variante escura do logo
// (prompt, K e cursor sobre a tinta), seguida de um espaço. Fora do Paper, nada.
func (t Theme) Mark() string {
	if !t.Paper {
		return ""
	}
	on := func(fg lipgloss.Color) lipgloss.Style {
		return t.R.NewStyle().Background(colInkStrong).Foreground(fg).Bold(true)
	}
	return on(colPaper).Render(" ") + on(colLogoTeal).Render("❯") + on(colPaper).Render("K") +
		on(colSun).Render("▂") + on(colPaper).Render(" ") + t.S().Render(" ")
}
