// Package cli é a interface de linha de comando do kanivet-cli, uma camada fina sobre o motor.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
)

type Env struct {
	R              runner.Runner
	P              platform.Platform
	Cat            *catalog.Catalog
	StateDir       string
	Stdout, Stderr io.Writer
	Stdin          io.Reader
	IsTTY          bool
	Now            func() time.Time
	Getenv         map[string]string
}

// exitError carrega o código de saída de um comando.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

func exit(code int, msg string) error { return &exitError{code, msg} }

func NewRoot(e *Env) *cobra.Command {
	root := &cobra.Command{
		Use:           "kanivet-cli",
		Short:         i18n.T("cli.short"),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(e.Stdout)
	root.SetErr(e.Stderr)
	root.SetIn(e.Stdin)
	root.AddCommand(listCmd(e), profilesCmd(e), statusCmd(e), planCmd(e), installCmd(e),
		updateCmd(e), uninstallCmd(e), restoreCmd(e), catalogCmd(e), doctorCmd(e))
	return root
}

// Run executa a CLI com args e devolve o código de saída.
func Run(e *Env, args []string) int {
	i18n.SetLang(i18n.DetectLang(e.Getenv))
	root := NewRoot(e)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0
	}
	var xe *exitError
	if errors.As(err, &xe) {
		if xe.msg != "" {
			fmt.Fprintln(e.Stderr, xe.msg)
		}
		return xe.code
	}
	fmt.Fprintln(e.Stderr, err)
	return 1
}

// Execute monta o ambiente real e roda a CLI.
func Execute() int {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	i18n.SetLang(i18n.DetectLang(env))
	p, err := platform.Detect()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	c, err := catalog.Embedded()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	e := &Env{
		R: runner.ExecRunner{}, P: p, Cat: c, StateDir: state.DefaultDir(p.Home),
		Stdout: os.Stdout, Stderr: os.Stderr, Stdin: os.Stdin,
		IsTTY: isTerminal(os.Stdin) && isTerminal(os.Stdout),
		Now:   time.Now, Getenv: env,
	}
	return Run(e, os.Args[1:])
}

func (e *Env) statePath() string { return filepath.Join(e.StateDir, "state.json") }

func (e *Env) loadState() (*state.State, error) { return state.Load(e.statePath()) }

func (e *Env) detector(st *state.State) *detect.Detector {
	return &detect.Detector{R: e.R, P: e.P, Owner: st}
}

func (e *Env) rcPath() string {
	if e.P.Shell == "zsh" {
		return filepath.Join(e.P.Home, ".zshrc")
	}
	return filepath.Join(e.P.Home, ".bashrc")
}

func (e *Env) shellDir() string { return filepath.Join(e.P.Home, ".config", "kanivet-cli", "shell") }

// plain diz se a saída deve evitar cores e ícones.
func (e *Env) plain() bool {
	_, noColor := e.Getenv["NO_COLOR"]
	return noColor || !e.IsTTY
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
