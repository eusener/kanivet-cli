package detect

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

type owner map[string]bool

func (o owner) Owned(id string) bool { return o[id] }

var linux = platform.Platform{OS: platform.Linux, Arch: "amd64", Distro: "debian", DistroVersion: "12", PkgManager: "apt", Systemd: false}

func item(id string) *catalog.Item {
	return &catalog.Item{
		ID: id, Category: "c", Name: i18n.Text{i18n.En: id}, Platforms: []platform.OS{platform.Linux, platform.MacOS},
		Detect:  catalog.Detect{Command: id + " --version", VersionRegex: `([\d.]+)`},
		Install: []catalog.Method{{Backend: "mise", Package: id}},
	}
}

func cat(items ...*catalog.Item) *catalog.Catalog {
	c := &catalog.Catalog{Items: map[string]*catalog.Item{}}
	for _, it := range items {
		c.Items[it.ID] = it
	}
	return c
}

func respond(m map[string]runner.Result) *runner.FakeRunner { return &runner.FakeRunner{Responses: m} }

func TestAbsent(t *testing.T) {
	d := &Detector{R: respond(map[string]runner.Result{"fzf --version": {ExitCode: 1}}), P: linux, Owner: owner{}}
	if s := d.Detect(context.Background(), cat(item("fzf")), nil)["fzf"]; s.Status != StatusAbsent {
		t.Fatalf("got %+v", s)
	}
}

func TestInstalledOwned(t *testing.T) {
	d := &Detector{R: respond(map[string]runner.Result{"fzf --version": {Output: "0.56.3 (brew)"}}), P: linux, Owner: owner{"fzf": true}}
	s := d.Detect(context.Background(), cat(item("fzf")), nil)["fzf"]
	if s.Status != StatusInstalled || s.Version != "0.56.3" {
		t.Fatalf("got %+v", s)
	}
}

func TestExternalTooOld(t *testing.T) {
	it := item("fzf")
	it.MinVersion = "0.48"
	d := &Detector{R: respond(map[string]runner.Result{"fzf --version": {Output: "0.44.1"}}), P: linux, Owner: owner{}}
	if s := d.Detect(context.Background(), cat(it), nil)["fzf"]; s.Status != StatusExternalOutdated {
		t.Fatalf("got %+v", s)
	}
}

func TestExternalUnparsedVersionIsPresent(t *testing.T) {
	it := item("fzf")
	it.MinVersion = "0.48"
	d := &Detector{R: respond(map[string]runner.Result{"fzf --version": {Output: "weird"}}), P: linux, Owner: owner{}}
	if s := d.Detect(context.Background(), cat(it), nil)["fzf"]; s.Status != StatusExternal || s.Version != "" {
		t.Fatalf("got %+v", s)
	}
}

func TestProvided(t *testing.T) {
	php, lerd := item("php"), item("lerd")
	php.ProvidedBy = []string{"lerd"}
	d := &Detector{R: respond(map[string]runner.Result{"php --version": {ExitCode: 1}, "lerd --version": {ExitCode: 1}}), P: linux, Owner: owner{}}
	if s := d.Detect(context.Background(), cat(php, lerd), map[string]bool{"lerd": true})["php"]; s.Status != StatusProvided {
		t.Fatalf("got %+v", s)
	}
}

func TestUnsupportedSystemd(t *testing.T) {
	it := item("lerd")
	it.Requires.Systemd = true
	d := &Detector{R: respond(nil), P: linux, Owner: owner{}}
	s := d.Detect(context.Background(), cat(it), nil)["lerd"]
	if s.Status != StatusUnsupported || !strings.Contains(s.Reason, "systemd") {
		t.Fatalf("got %+v", s)
	}
}

func TestUnsupportedDistroVersion(t *testing.T) {
	it := item("flock")
	it.Requires.Distros = []string{"ubuntu>=24.04", "fedora"}
	d := &Detector{R: respond(nil), P: linux, Owner: owner{}}
	if s := d.Detect(context.Background(), cat(it), nil)["flock"]; s.Status != StatusUnsupported {
		t.Fatalf("debian 12 must be unsupported: %+v", s)
	}
	ok := linux
	ok.Distro, ok.DistroVersion = "ubuntu", "24.04"
	d.P = ok
	d.R = respond(map[string]runner.Result{"flock --version": {ExitCode: 1}})
	if s := d.Detect(context.Background(), cat(it), nil)["flock"]; s.Status != StatusAbsent {
		t.Fatalf("ubuntu 24.04 must be supported: %+v", s)
	}
}

func TestUnsupportedPlatformAndGraphical(t *testing.T) {
	mac := item("mac-only")
	mac.Platforms = []platform.OS{platform.MacOS}
	gui := item("kitty")
	gui.Requires.Graphical = true
	d := &Detector{R: respond(nil), P: linux, Owner: owner{}}
	st := d.Detect(context.Background(), cat(mac, gui), nil)
	if st["mac-only"].Status != StatusUnsupported || st["kitty"].Status != StatusUnsupported {
		t.Fatalf("got %+v", st)
	}
}

func TestTimeout(t *testing.T) {
	f := &runner.FakeRunner{Func: func(ctx context.Context, c runner.Cmd) (runner.Result, bool) {
		<-ctx.Done()
		return runner.Result{ExitCode: -1, Err: ctx.Err()}, true
	}}
	d := &Detector{R: f, P: linux, Owner: owner{}, Timeout: 50 * time.Millisecond}
	start := time.Now()
	s := d.Detect(context.Background(), cat(item("slow")), nil)["slow"]
	if s.Status != StatusUnknown || time.Since(start) > time.Second {
		t.Fatalf("got %+v after %v", s, time.Since(start))
	}
}

func TestCheckUpdates(t *testing.T) {
	f := respond(map[string]runner.Result{
		"fzf --version":        {Output: "1.0"},
		"mise outdated --json": {Output: `{"fzf":{"latest":"1.1"}}`},
	})
	d := &Detector{R: f, P: linux, Owner: owner{"fzf": true}}
	c := cat(item("fzf"))
	st := d.Detect(context.Background(), c, nil)
	d.CheckUpdates(context.Background(), c, st)
	if st["fzf"].Status != StatusOutdated || st["fzf"].Latest != "1.1" {
		t.Fatalf("got %+v", st["fzf"])
	}
}
