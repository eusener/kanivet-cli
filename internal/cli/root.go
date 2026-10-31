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

	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/buildinfo"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
	"github.com/eusener/kanivet-cli/internal/tui"
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
	// RunTUI abre a TUI (nil usa tui.Run); os testes trocam por um falso.
	RunTUI func(*app.Session, map[string]string) error
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
		Version:       buildinfo.String(),
		// Sem subcomando: TUI quando stdin e stdout são terminais; senão, a ajuda.
		RunE: func(cmd *cobra.Command, args []string) error {
			if !e.IsTTY {
				return cmd.Help()
			}
			s, err := e.open()
			if err != nil {
				return err
			}
			run := e.RunTUI
			if run == nil {
				run = tui.Run
			}
			return run(s, e.Getenv)
		},
	}
	// --version é tratado pelo cobra antes do RunE: não abre a TUI nem lê o estado.
	root.SetVersionTemplate("kanivet-cli {{.Version}}\n")
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
	// Os passos seguintes precisam enxergar o que os anteriores instalaram (mise, cargo, brew...).
	env["PATH"] = withToolPaths(env["PATH"], p.Home)
	os.Setenv("PATH", env["PATH"])
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

// session monta a sessão do motor sem carregar o estado.
func (e *Env) session() app.Session {
	return app.Session{R: e.R, P: e.P, Cat: e.Cat, StateDir: e.StateDir, Getenv: e.Getenv, Now: e.Now}
}

// open monta a sessão e carrega o estado.
func (e *Env) open() (*app.Session, error) { return app.Open(e.session()) }

// plain diz se a saída deve evitar cores e ícones.
func (e *Env) plain() bool {
	_, noColor := e.Getenv["NO_COLOR"]
	return noColor || !e.IsTTY
}

// withToolPaths põe na frente do PATH os diretórios onde os instaladores colocam binários.
func withToolPaths(path, home string) string {
	dirs := []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".local", "share", "mise", "shims"),
		filepath.Join(home, ".cargo", "bin"),
		"/opt/homebrew/bin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin",
	}
	current := filepath.SplitList(path)
	have := map[string]bool{}
	for _, d := range current {
		have[d] = true
	}
	var add []string
	for _, d := range dirs {
		if !have[d] {
			add = append(add, d)
		}
	}
	return strings.Join(append(add, current...), string(os.PathListSeparator))
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
