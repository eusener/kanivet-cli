// Package configs aplica as configs opinativas com backup, controle por hash e o bloco no rc do shell.
package configs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/state"
)

const (
	RcMarkerStart = "# >>> kanivet-cli >>>"
	RcMarkerEnd   = "# <<< kanivet-cli <<<"
)

type Vars struct {
	HOME, XDG_CONFIG, XDG_DATA, OS, Arch string
}

func VarsFor(p platform.Platform, env map[string]string) Vars {
	v := Vars{HOME: p.Home, XDG_CONFIG: env["XDG_CONFIG_HOME"], XDG_DATA: env["XDG_DATA_HOME"], OS: string(p.OS), Arch: p.Arch}
	if v.XDG_CONFIG == "" {
		v.XDG_CONFIG = filepath.Join(p.Home, ".config")
	}
	if v.XDG_DATA == "" {
		v.XDG_DATA = filepath.Join(p.Home, ".local", "share")
	}
	return v
}

func Render(fsys fs.FS, tmpl string, v Vars) ([]byte, error) {
	src, err := fs.ReadFile(fsys, tmpl)
	if err != nil {
		return nil, err
	}
	return renderString(tmpl, string(src), v)
}

func renderString(name, src string, v Vars) ([]byte, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(src)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type Applier struct {
	St        *state.State
	BackupDir string // backups/<run>
	Vars      Vars
	Files     fs.FS
}

type Conflict struct {
	Target string
	Diff   string
}

// ExpandTarget resolve variáveis e "~" num caminho de destino.
func (a *Applier) ExpandTarget(target string) (string, error) {
	out, err := renderString("target", target, a.Vars)
	if err != nil {
		return "", err
	}
	s := string(out)
	if s == "~" || strings.HasPrefix(s, "~/") {
		s = a.Vars.HOME + s[1:]
	}
	return filepath.Clean(s), nil
}

// realPath segue symlinks para escrever no arquivo real sem trocar o link.
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func (a *Applier) backup(target string) (string, error) {
	data, err := os.ReadFile(realPath(target))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	dest := filepath.Join(a.BackupDir, strings.TrimPrefix(target, "/"))
	if _, err := os.Stat(dest); err == nil {
		return dest, nil // já tem backup neste run
	}
	return dest, state.WriteFileAtomic(dest, data, 0o644)
}

func (a *Applier) Apply(fc plan.FileChange, force bool) (*Conflict, error) {
	target, err := a.ExpandTarget(fc.Target)
	if err != nil {
		return nil, err
	}
	content, err := Render(a.Files, fc.Template, a.Vars)
	if err != nil {
		return nil, err
	}
	real := realPath(target)
	current, readErr := os.ReadFile(real)
	exists := readErr == nil
	prev, applied := a.St.Configs[target]
	if exists && sha(current) == sha(content) {
		a.record(fc.ItemID, target, prev.Backup, content)
		return nil, nil
	}
	if applied && exists && sha(current) != prev.SHA256 && !force {
		return &Conflict{Target: target, Diff: diff(string(current), string(content))}, nil
	}
	backup, err := a.backup(target)
	if err != nil {
		return nil, err
	}
	if applied {
		backup = prev.Backup // guarda sempre o original de antes do kanivet-cli
	}
	perm := fs.FileMode(0o644)
	if fi, err := os.Stat(real); err == nil {
		perm = fi.Mode().Perm()
	}
	if err := state.WriteFileAtomic(real, content, perm); err != nil {
		return nil, err
	}
	a.record(fc.ItemID, target, backup, content)
	return nil, nil
}

func (a *Applier) record(item, target, backup string, content []byte) {
	a.St.Configs[target] = state.AppliedConfig{Item: item, Target: target, Backup: backup, SHA256: sha(content), At: time.Now().UTC()}
}

// Remove desfaz uma config: restaura o backup original ou apaga o arquivo se ele não existia.
func (a *Applier) Remove(target string) error {
	target, err := a.ExpandTarget(target)
	if err != nil {
		return err
	}
	rec, ok := a.St.Configs[target]
	if !ok {
		return nil
	}
	if _, err := a.backup(target); err != nil { // guarda a versão atual antes de desfazer
		return err
	}
	real := realPath(target)
	if rec.Backup != "" {
		data, err := os.ReadFile(rec.Backup)
		if err != nil {
			return err
		}
		if err := state.WriteFileAtomic(real, data, 0o644); err != nil {
			return err
		}
	} else if err := os.Remove(real); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	delete(a.St.Configs, target)
	return nil
}

func RcBlock(shell string) string {
	f := `$HOME/.config/kanivet-cli/shell/init.` + shell
	return RcMarkerStart + "\n" + `[ -f "` + f + `" ] && . "` + f + `"` + "\n" + RcMarkerEnd + "\n"
}

// EnsureRcBlock adiciona o bloco do kanivet-cli no fim do rc, se ainda não houver.
func EnsureRcBlock(rc, shell string) (bool, error) {
	real := realPath(rc)
	content, err := os.ReadFile(real)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if bytes.Contains(content, []byte(RcMarkerStart)) {
		return false, nil
	}
	if len(content) > 0 && !bytes.HasSuffix(content, []byte("\n")) {
		content = append(content, '\n')
	}
	content = append(content, []byte("\n"+RcBlock(shell))...)
	perm := fs.FileMode(0o644)
	if fi, err := os.Stat(real); err == nil {
		perm = fi.Mode().Perm()
	}
	return true, state.WriteFileAtomic(real, content, perm)
}

// RestoreRun copia de volta todos os arquivos de backups/<run>.
func RestoreRun(backupRunDir string) ([]string, error) {
	var restored []string
	err := filepath.WalkDir(backupRunDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(backupRunDir, p)
		target := "/" + filepath.ToSlash(rel)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := state.WriteFileAtomic(realPath(target), data, 0o644); err != nil {
			return err
		}
		restored = append(restored, target)
		return nil
	})
	return restored, err
}

// diff é um diff simples (atual vs. novo), suficiente para o usuário decidir.
func diff(current, next string) string {
	var b strings.Builder
	b.WriteString("--- current\n+++ kanivet-cli\n")
	for _, l := range strings.SplitAfter(current, "\n") {
		if l != "" {
			fmt.Fprintf(&b, "-%s", l)
		}
	}
	for _, l := range strings.SplitAfter(next, "\n") {
		if l != "" {
			fmt.Fprintf(&b, "+%s", l)
		}
	}
	return b.String()
}
