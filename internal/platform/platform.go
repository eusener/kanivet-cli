// Package platform detecta o sistema em que o kanivet-cli está rodando.
package platform

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type OS string

const (
	MacOS OS = "macos"
	Linux OS = "linux"
)

type Platform struct {
	OS            OS
	Arch          string // "amd64" | "arm64"
	Distro        string // ID do /etc/os-release; "" no macOS
	DistroVersion string // VERSION_ID; no macOS, a versão do sistema
	PkgManager    string // "apt" | "dnf" | "pacman" | "zypper" | ""
	Shell         string // "zsh" | "bash"
	Graphical     bool
	Systemd       bool
	Home          string
}

// Detect lê o ambiente real da máquina.
func Detect() (Platform, error) {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	if _, ok := env["HOME"]; !ok {
		if h, err := os.UserHomeDir(); err == nil {
			env["HOME"] = h
		}
	}
	osRelease, _ := os.ReadFile("/etc/os-release")
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	look := func(n string) bool { _, err := exec.LookPath(n); return err == nil }
	p, err := detectFrom(env, runtime.GOOS, runtime.GOARCH, string(osRelease), exists, look)
	if err == nil && p.OS == MacOS {
		if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
			p.DistroVersion = strings.TrimSpace(string(out))
		}
	}
	return p, err
}

func detectFrom(env map[string]string, goos, goarch, osRelease string,
	fileExists func(string) bool, lookPath func(string) bool) (Platform, error) {
	var p Platform
	switch goos {
	case "darwin":
		p.OS = MacOS
	case "linux":
		p.OS = Linux
	default:
		return p, fmt.Errorf("unsupported OS: %s", goos)
	}
	switch goarch {
	case "amd64", "arm64":
		p.Arch = goarch
	default:
		return p, fmt.Errorf("unsupported architecture: %s", goarch)
	}

	p.Home = env["HOME"]
	p.Shell = "bash"
	if filepath.Base(env["SHELL"]) == "zsh" {
		p.Shell = "zsh"
	}

	if p.OS == MacOS {
		p.Graphical = true
		return p, nil
	}

	fields := parseOSRelease(osRelease)
	p.Distro = fields["ID"]
	p.DistroVersion = fields["VERSION_ID"]
	p.Graphical = env["DISPLAY"] != "" || env["WAYLAND_DISPLAY"] != ""
	p.Systemd = fileExists("/run/systemd/system")
	for _, m := range []struct{ bin, name string }{
		{"apt-get", "apt"}, {"dnf", "dnf"}, {"pacman", "pacman"}, {"zypper", "zypper"},
	} {
		if lookPath(m.bin) {
			p.PkgManager = m.name
			break
		}
	}
	return p, nil
}

func parseOSRelease(s string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "="); ok {
			out[k] = strings.Trim(v, `"'`)
		}
	}
	return out
}
