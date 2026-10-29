package tui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
)

var (
	linuxSystemd = platform.Platform{OS: platform.Linux, Arch: "amd64", Distro: "ubuntu", DistroVersion: "24.04", PkgManager: "apt", Shell: "zsh", Systemd: true, Graphical: true, Home: "/home/dev"}
	macARM       = platform.Platform{OS: platform.MacOS, Arch: "arm64", DistroVersion: "15.2", Shell: "zsh", Graphical: true, Home: "/Users/dev"}
)

func emb(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func newSel(t *testing.T, p platform.Platform, st map[string]detect.ItemStatus, m Mode) *Selection {
	t.Helper()
	return NewSelection(emb(t), p, st, m)
}

func TestApplyProfileMarksItemsAndExclusiveByOS(t *testing.T) {
	s := newSel(t, linuxSystemd, nil, ModeWizard)
	s.ApplyProfile("laravel")
	if !s.Marked("lerd") || s.Marked("flock") || !s.Marked("fzf") {
		t.Errorf("linux: lerd=%v flock=%v", s.Marked("lerd"), s.Marked("flock"))
	}
	s = newSel(t, macARM, nil, ModeWizard)
	s.ApplyProfile("laravel")
	if s.Marked("lerd") || !s.Marked("flock") {
		t.Errorf("mac: lerd=%v flock=%v", s.Marked("lerd"), s.Marked("flock"))
	}
}

func TestExclusiveIsRadio(t *testing.T) {
	s := newSel(t, linuxSystemd, nil, ModeWizard)
	s.ApplyProfile("laravel")
	s.Toggle("flock")
	if s.Marked("lerd") || !s.Marked("flock") {
		t.Fatal("flock should replace lerd")
	}
	s.Toggle("flock")
	if s.Marked("lerd") || s.Marked("flock") {
		t.Fatal("both should be unmarked")
	}
}

func TestUnsupportedNotSelectable(t *testing.T) {
	st := map[string]detect.ItemStatus{"kitty": {ID: "kitty", Status: detect.StatusUnsupported, Reason: i18n.T("reason.graphical")}}
	s := newSel(t, linuxSystemd, st, ModeWizard)
	ok, why := s.Selectable("kitty")
	if ok || !strings.Contains(why, i18n.T("reason.graphical")) {
		t.Errorf("ok=%v why=%q", ok, why)
	}
	if changed, _ := s.Toggle("kitty"); changed || s.Marked("kitty") {
		t.Error("unsupported must not toggle")
	}
}

func TestWizardToPlan(t *testing.T) {
	s := newSel(t, linuxSystemd, nil, ModeWizard)
	s.ApplyProfile("web")
	s.Toggle("bun")
	s.CycleVersion("node", 1)
	got := s.ToPlan()
	if got.Profile != "web" {
		t.Errorf("profile %q", got.Profile)
	}
	if !slices.Contains(got.Remove, "bun") || slices.Contains(got.Add, "bun") || !slices.Contains(got.Add, "node") {
		t.Errorf("add=%v remove=%v", got.Add, got.Remove)
	}
	if !reflect.DeepEqual(got.Versions, map[string]string{"node": "24"}) {
		t.Errorf("versions %v", got.Versions)
	}
}

func TestDashboardUninstallOwned(t *testing.T) {
	st := map[string]detect.ItemStatus{"bat": {ID: "bat", Status: detect.StatusInstalled}}
	s := newSel(t, linuxSystemd, st, ModeDashboard)
	if !s.Marked("bat") {
		t.Fatal("installed starts marked")
	}
	s.Toggle("bat")
	if got := s.ToPlan().Uninstall; !reflect.DeepEqual(got, []string{"bat"}) {
		t.Errorf("uninstall %v", got)
	}
}

func TestDeselectExternalDoesNotUninstall(t *testing.T) {
	st := map[string]detect.ItemStatus{"eza": {ID: "eza", Status: detect.StatusExternal}}
	s := newSel(t, linuxSystemd, st, ModeDashboard)
	changed, note := s.Toggle("eza")
	if changed || note != i18n.T("tui.external_untouched") || !s.Marked("eza") {
		t.Errorf("changed=%v note=%q marked=%v", changed, note, s.Marked("eza"))
	}
	if len(s.ToPlan().Uninstall) != 0 {
		t.Error("uninstall must be empty")
	}
}

func TestPendingCounts(t *testing.T) {
	st := map[string]detect.ItemStatus{
		"bat": {ID: "bat", Status: detect.StatusInstalled},
		"fd":  {ID: "fd", Status: detect.StatusOutdated},
		"eza": {ID: "eza", Status: detect.StatusExternal},
	}
	s := newSel(t, linuxSystemd, st, ModeDashboard)
	if i, u, r := s.Pending(); i+u+r != 0 {
		t.Errorf("initial %d %d %d", i, u, r)
	}
	s.Toggle("jq")
	s.Toggle("bat")
	s.MarkUpdate("fd")
	if i, u, r := s.Pending(); i != 1 || u != 1 || r != 1 {
		t.Errorf("got %d %d %d", i, u, r)
	}
}

func TestConfigToggle(t *testing.T) {
	s := newSel(t, linuxSystemd, nil, ModeWizard)
	if !s.ConfigOn("kitty-config") {
		t.Fatal("default on")
	}
	s.ToggleConfig("kitty-config")
	if got := s.ToPlan().Configs; !reflect.DeepEqual(got, map[string]bool{"kitty-config": false}) {
		t.Errorf("configs %v", got)
	}
}

func TestConfigsOnCountsMarkedItemsConfigs(t *testing.T) {
	s := newSel(t, linuxSystemd, nil, ModeWizard)
	if s.ConfigsOn() != 0 {
		t.Fatal("sem marcados deveria ser 0")
	}
	s.Toggle("kitty")
	on := s.ConfigsOn()
	if on == 0 {
		t.Fatal("kitty tem config padrão ligada")
	}
	s.ToggleConfig("kitty-config")
	if s.ConfigsOn() != on-1 {
		t.Errorf("desligar config deveria reduzir: %d -> %d", on, s.ConfigsOn())
	}
}
