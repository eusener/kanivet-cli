package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

var update = flag.Bool("update", false, "reescreve os arquivos golden")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("%s difere do golden (rode com -update):\n%s", name, got)
	}
}

func newList(t *testing.T, st map[string]detect.ItemStatus, mode Mode, cats ...string) (ItemList, *Selection) {
	t.Helper()
	i18n.SetLang(i18n.En)
	sel := newSel(t, linuxSystemd, st, mode)
	return NewItemList(sel, NewTheme(nil, false), DefaultKeys(), cats), sel
}

func press(l ItemList, keys ...string) ItemList {
	for _, k := range keys {
		var m tea.KeyMsg
		switch k {
		case "space":
			m = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		case "right":
			m = tea.KeyMsg{Type: tea.KeyRight}
		case "left":
			m = tea.KeyMsg{Type: tea.KeyLeft}
		case "down":
			m = tea.KeyMsg{Type: tea.KeyDown}
		case "esc":
			m = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			m = tea.KeyMsg{Type: tea.KeyTab}
		case "enter":
			m = tea.KeyMsg{Type: tea.KeyEnter}
		default:
			m = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		l, _ = l.Update(m)
	}
	return l
}

func trimView(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestItemListGolden(t *testing.T) {
	st := map[string]detect.ItemStatus{
		"atuin":    {ID: "atuin", Status: detect.StatusInstalled, Version: "18.3.0"},
		"direnv":   {ID: "direnv", Status: detect.StatusOutdated, Version: "2.32.0"},
		"fzf":      {ID: "fzf", Status: detect.StatusExternal, Version: "0.56.3"},
		"starship": {ID: "starship", Status: detect.StatusUnsupported, Reason: "needs something"},
	}
	l, _ := newList(t, st, ModeDashboard, "shell", "replacements")
	l = press(l, "j")
	if l.Current() != "direnv" {
		t.Fatalf("cursor = %q", l.Current())
	}
	golden(t, "itemlist.golden", trimView(l.View(100, 30)))
}

func TestSpaceTogglesAndRadio(t *testing.T) {
	l, sel := newList(t, nil, ModeWizard, "php-local")
	sel.ApplyProfile("laravel")
	if !sel.Marked("lerd") {
		t.Fatal("precondition: lerd marked")
	}
	for l.Current() != "flock" {
		l = press(l, "j")
	}
	l = press(l, "space")
	if !sel.Marked("flock") || sel.Marked("lerd") {
		t.Errorf("flock=%v lerd=%v", sel.Marked("flock"), sel.Marked("lerd"))
	}
	if !strings.Contains(l.View(100, 30), "(•)") {
		t.Error("radio marker missing")
	}
}

func TestUnsupportedLineIgnoresSpace(t *testing.T) {
	st := map[string]detect.ItemStatus{"atuin": {ID: "atuin", Status: detect.StatusUnsupported, Reason: "no way"}}
	l, sel := newList(t, st, ModeWizard, "shell")
	if l.Current() != "atuin" {
		t.Fatalf("cursor = %q", l.Current())
	}
	l = press(l, "space")
	if sel.Marked("atuin") {
		t.Error("unsupported item was marked")
	}
	if !strings.Contains(l.View(100, 30), "no way") {
		t.Error("reason not shown")
	}
}

func TestVersionCycleInDetail(t *testing.T) {
	l, sel := newList(t, nil, ModeWizard, "languages")
	for l.Current() != "node" {
		l = press(l, "j")
	}
	if !strings.Contains(l.View(100, 30), "‹ 22 ›") {
		t.Error("version selector missing")
	}
	l = press(l, "right")
	if got := sel.Version("node"); got != "24" {
		t.Fatalf("version = %q", got)
	}
	if !strings.Contains(l.View(100, 30), "‹ 24 ›") {
		t.Error("version selector not updated")
	}
	l = press(l, "left")
	if got := sel.Version("node"); got != "22" {
		t.Errorf("version = %q", got)
	}
}

func TestConfigToggleInPanel(t *testing.T) {
	l, sel := newList(t, nil, ModeWizard, "terminal")
	for l.Current() != "kitty" {
		l = press(l, "j")
	}
	cfg := "kitty-config"
	before := sel.ConfigOn(cfg)
	l = press(l, "c", "space")
	if sel.ConfigOn(cfg) == before {
		t.Error("config not toggled")
	}
	if sel.Marked("kitty") {
		t.Error("space in panel must not mark the item")
	}
	l = press(l, "esc", "space")
	if !sel.Marked("kitty") {
		t.Error("esc should return focus to list")
	}
}

func TestFilter(t *testing.T) {
	l, _ := newList(t, nil, ModeWizard, "replacements")
	l = press(l, "/")
	if !l.Filtering() {
		t.Fatal("not filtering")
	}
	l = press(l, "R", "i", "p")
	v := l.View(100, 30)
	if !strings.Contains(v, "ripgrep") || strings.Contains(v, "procs") || strings.Contains(v, "btop") {
		t.Errorf("filter wrong:\n%s", v)
	}
	if got := l.visible(); len(got) != 1 || got[0] != "ripgrep" {
		t.Errorf("visible = %v", got)
	}
	l = press(l, "esc")
	if l.Filtering() || len(l.visible()) < 5 {
		t.Error("esc should clear the filter")
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	c := &catalog.Catalog{
		Categories: []catalog.Category{{ID: "c"}},
		Items:      map[string]*catalog.Item{},
	}
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("item%02d", i)
		c.Items[id] = &catalog.Item{ID: id, Category: "c"}
	}
	sel := NewSelection(c, linuxSystemd, nil, ModeWizard)
	l := NewItemList(sel, NewTheme(nil, false), DefaultKeys(), []string{"c"})
	v := l.View(100, 10)
	if strings.Contains(v, "item39") || strings.Count(v, "\n") > 9 {
		t.Errorf("renders offscreen rows:\n%s", v)
	}
	l = press(l, "G")
	v = l.View(100, 10)
	if !strings.Contains(v, "item39") || strings.Contains(v, "item00") {
		t.Errorf("last item not visible:\n%s", v)
	}
	if lines := strings.Count(v, "\n") + 1; lines != 10 {
		t.Errorf("height = %d", lines)
	}
}

func TestUnknownShownWithHint(t *testing.T) {
	st := map[string]detect.ItemStatus{"atuin": {ID: "atuin", Status: detect.StatusUnknown}}
	l, _ := newList(t, st, ModeWizard, "shell")
	v := l.View(100, 30)
	if !strings.Contains(v, "[?] unknown") || !strings.Contains(v, i18n.T("tui.unknown_hint")) {
		t.Errorf("unknown not explained:\n%s", v)
	}
}

func TestDetailKeepsConfigsAndNoteAt80x24(t *testing.T) {
	c := &catalog.Catalog{
		Categories: []catalog.Category{{ID: "c"}},
		Items:      map[string]*catalog.Item{},
	}
	long := strings.Repeat("palavra longa de descricao ", 8)
	c.Items["multi"] = &catalog.Item{
		ID: "multi", Category: "c",
		Name:        i18n.Text{i18n.En: "Multi"},
		Description: i18n.Text{i18n.En: long},
		Notes:       i18n.Text{i18n.En: long},
		Configs: []catalog.ConfigSpec{
			{ID: "a", Description: i18n.Text{i18n.En: "config alpha"}},
			{ID: "b", Description: i18n.Text{i18n.En: "config beta"}},
			{ID: "d", Description: i18n.Text{i18n.En: "config gamma"}},
			{ID: "e", Description: i18n.Text{i18n.En: "config delta"}},
		},
	}
	st := map[string]detect.ItemStatus{"multi": {ID: "multi", Status: detect.StatusUnknown}}
	sel := NewSelection(c, linuxSystemd, st, ModeWizard)
	l := NewItemList(sel, NewTheme(nil, false), DefaultKeys(), []string{"c"})

	// Rows 18 = 24 - header/footer; detail gets ~7.
	h := 18
	checkWidth := func(v string) {
		t.Helper()
		for i, ln := range strings.Split(v, "\n") {
			if w := lipgloss.Width(ln); w > 80 {
				t.Errorf("line %d has width %d > 80: %q", i, w, ln)
			}
		}
	}
	// Move the config cursor to the last config: it must stay visible.
	l = press(l, "c", "c", "c", "c") // wraps back to index 0 after 4 presses
	l = press(l, "c", "c", "c")      // index 3
	v := l.View(80, h)
	checkWidth(v)
	if !strings.Contains(v, "> [ ] config delta") {
		t.Errorf("active config hidden:\n%s", v)
	}
	if got := strings.Count(v, "\n") + 1; got != h {
		t.Errorf("height = %d", got)
	}
	// Refused toggle note stays visible: use an unsupported item.
	st["multi"] = detect.ItemStatus{ID: "multi", Status: detect.StatusUnsupported, Reason: "motivo da recusa"}
	l = press(l, "esc", "space")
	v = l.View(80, h)
	checkWidth(v)
	if !strings.Contains(v, "motivo da recusa") {
		t.Errorf("note hidden:\n%s", v)
	}
	// Full 80x24 for the real catalog too.
	l2, _ := newList(t, nil, ModeWizard, "shell", "replacements", "languages")
	checkWidth(l2.View(80, 24))
	checkWidth(press(l2, "/", "e").View(80, 24))
}
