package tui

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

var allStatuses = []detect.Status{
	detect.StatusInstalled, detect.StatusOutdated, detect.StatusExternal,
	detect.StatusExternalOutdated, detect.StatusAbsent, detect.StatusProvided,
	detect.StatusUnsupported, detect.StatusUnknown,
}

func TestStatusIconsASCII(t *testing.T) {
	want := map[detect.Status]string{
		detect.StatusInstalled: "[x]", detect.StatusOutdated: "[^]", detect.StatusExternal: "[e]",
		detect.StatusExternalOutdated: "[!]", detect.StatusAbsent: "[ ]", detect.StatusProvided: "[p]",
		detect.StatusUnsupported: "[-]", detect.StatusUnknown: "[?]",
	}
	th := NewTheme(nil, false)
	for _, s := range allStatuses {
		if got := th.StatusIcon(s); got != want[s] {
			t.Errorf("%v: got %q want %q", s, got, want[s])
		}
	}
}

func TestStatusIconsNerd(t *testing.T) {
	want := map[detect.Status]string{
		detect.StatusInstalled: "", detect.StatusOutdated: "", detect.StatusExternal: "",
		detect.StatusExternalOutdated: "", detect.StatusAbsent: "", detect.StatusProvided: "",
		detect.StatusUnsupported: "", detect.StatusUnknown: "",
	}
	th := NewTheme(nil, true)
	for _, s := range allStatuses {
		if got := th.StatusIcon(s); got != want[s] {
			t.Errorf("%v: got %q want %q", s, got, want[s])
		}
	}
}

func TestCheckMarkers(t *testing.T) {
	a, n := NewTheme(nil, false), NewTheme(nil, true)
	cases := []struct {
		th            Theme
		marked, radio bool
		want          string
	}{
		{a, true, false, "[x]"}, {a, false, false, "[ ]"}, {a, true, true, "(•)"}, {a, false, true, "( )"},
		{n, true, false, "✓"}, {n, false, false, "○"}, {n, true, true, "●"}, {n, false, true, "○"},
	}
	for _, c := range cases {
		if got := c.th.Check(c.marked, c.radio); got != c.want {
			t.Errorf("Check(%v,%v) nerd=%v: got %q want %q", c.marked, c.radio, c.th.Nerd, got, c.want)
		}
	}
}

func TestNoColorRendersPlain(t *testing.T) {
	th := NewTheme(map[string]string{"NO_COLOR": "1"}, false)
	if th.Color {
		t.Error("Color should be false")
	}
	if got := th.Accent.Render("x"); got != "x" {
		t.Errorf("got %q", got)
	}
	if !NewTheme(map[string]string{}, false).Color {
		t.Error("Color should be true without NO_COLOR")
	}
}

func TestThemePaperModes(t *testing.T) {
	cases := []struct {
		name  string
		p     termenv.Profile
		env   map[string]string
		paper bool
		color bool
	}{
		{"truecolor", termenv.TrueColor, nil, true, true},
		{"256", termenv.ANSI256, nil, true, true},
		{"16", termenv.ANSI, nil, false, true},
		{"no_color", termenv.TrueColor, map[string]string{"NO_COLOR": "1"}, false, false},
	}
	for _, c := range cases {
		th := themeWithProfile(c.p, c.env)
		if th.Paper != c.paper || th.Color != c.color {
			t.Errorf("%s: Paper=%v Color=%v", c.name, th.Paper, th.Color)
		}
	}
}

func TestHasNerdFont(t *testing.T) {
	if HasNerdFont(nil) {
		t.Error("nil map")
	}
	if HasNerdFont(map[string]detect.ItemStatus{"nerd-font-jetbrains": {Status: detect.StatusAbsent}}) {
		t.Error("absent")
	}
	if !HasNerdFont(map[string]detect.ItemStatus{"nerd-font-jetbrains": {Status: detect.StatusInstalled}}) {
		t.Error("installed")
	}
}

func TestDefaultKeysHaveHelp(t *testing.T) {
	k := DefaultKeys()
	if !k.Quit.Enabled() || k.Quit.Help().Key == "" || k.Quit.Help().Desc == "" {
		t.Error("quit help missing")
	}
	if len(k.Toggle.Keys()) == 0 || k.Toggle.Keys()[0] != " " {
		t.Errorf("toggle keys: %v", k.Toggle.Keys())
	}
}
