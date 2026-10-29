package platform

import (
	"strings"
	"testing"
)

func lookOnly(names ...string) func(string) bool {
	return func(n string) bool {
		for _, x := range names {
			if x == n {
				return true
			}
		}
		return false
	}
}

func noFile(string) bool { return false }

func TestDetectUbuntu(t *testing.T) {
	p, err := detectFrom(map[string]string{"SHELL": "/bin/bash", "HOME": "/home/dev"},
		"linux", "amd64", "ID=ubuntu\nVERSION_ID=\"24.04\"\n", noFile, lookOnly("apt-get"))
	if err != nil {
		t.Fatal(err)
	}
	if p.OS != Linux || p.Distro != "ubuntu" || p.DistroVersion != "24.04" || p.PkgManager != "apt" ||
		p.Shell != "bash" || p.Graphical || p.Home != "/home/dev" {
		t.Fatalf("unexpected platform: %+v", p)
	}
}

func TestDetectFedoraWayland(t *testing.T) {
	p, err := detectFrom(map[string]string{"SHELL": "/usr/bin/zsh", "WAYLAND_DISPLAY": "wayland-0"},
		"linux", "arm64", "ID=fedora\nVERSION_ID=41\n", func(s string) bool { return s == "/run/systemd/system" }, lookOnly("dnf"))
	if err != nil {
		t.Fatal(err)
	}
	if !p.Graphical || p.PkgManager != "dnf" || p.Shell != "zsh" || !p.Systemd || p.Arch != "arm64" {
		t.Fatalf("unexpected platform: %+v", p)
	}
}

func TestDetectMacOS(t *testing.T) {
	p, err := detectFrom(map[string]string{"SHELL": "/bin/zsh"}, "darwin", "arm64", "", noFile, lookOnly())
	if err != nil {
		t.Fatal(err)
	}
	if p.OS != MacOS || !p.Graphical || p.PkgManager != "" {
		t.Fatalf("unexpected platform: %+v", p)
	}
}

func TestDetectWindowsFails(t *testing.T) {
	_, err := detectFrom(map[string]string{}, "windows", "amd64", "", noFile, lookOnly())
	if err == nil || !strings.Contains(err.Error(), "unsupported OS") {
		t.Fatalf("want unsupported OS error, got %v", err)
	}
}

func TestArchMapping(t *testing.T) {
	if _, err := detectFrom(map[string]string{}, "linux", "386", "ID=debian\n", noFile, lookOnly("apt-get")); err == nil {
		t.Fatal("want error for 386")
	}
}
