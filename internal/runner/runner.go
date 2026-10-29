// Package runner executa comandos externos (ou os simula, nos testes).
package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Cmd struct {
	Argv    []string
	Shell   string // != "" → roda via "sh -c Shell"
	Env     []string
	Dir     string
	Sudo    bool
	Lock    string // serializa comandos do mesmo gerenciador no executor
	Display string // texto amigável para o plano
	Stdin   bool   // liga o stdin do terminal (sudo -v, prompts)
}

// String é o comando real, usado como identidade (e chave do FakeRunner).
func (c Cmd) String() string {
	s := c.Shell
	if s == "" {
		s = strings.Join(c.Argv, " ")
	}
	if c.Sudo {
		s = "sudo " + s
	}
	return s
}

// Shown é o texto exibido ao usuário.
func (c Cmd) Shown() string {
	if c.Display != "" {
		return c.Display
	}
	return c.String()
}

type Result struct {
	ExitCode int
	Output   string
	Err      error
}

func (r Result) OK() bool { return r.Err == nil && r.ExitCode == 0 }

type Runner interface {
	Run(ctx context.Context, c Cmd, out io.Writer) Result
}

// DefaultGrace é quanto o processo tem para encerrar depois do SIGTERM, antes do SIGKILL.
const DefaultGrace = 10 * time.Second

type ExecRunner struct {
	Grace time.Duration
}

// execArgv monta o argv real. Comandos com sudo que não leem o terminal usam
// "sudo -n": se a credencial expirou, falham na hora em vez de travar num prompt invisível.
func execArgv(c Cmd) []string {
	argv := c.Argv
	if c.Shell != "" {
		argv = []string{"sh", "-c", c.Shell}
	}
	if c.Sudo {
		prefix := []string{"sudo", "-n"}
		if c.Stdin {
			prefix = []string{"sudo"}
		}
		argv = append(prefix, argv...)
	}
	return argv
}

// procAttr: comandos interativos ficam no grupo em primeiro plano do terminal
// (um grupo novo leva SIGTTIN ao ler o tty e fica parado para sempre).
func procAttr(c Cmd) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: !c.Stdin}
}

func (r ExecRunner) Run(ctx context.Context, c Cmd, out io.Writer) Result {
	argv := execArgv(c)
	if len(argv) == 0 {
		return Result{ExitCode: -1, Err: errors.New("empty command")}
	}
	grace := r.Grace
	if grace == 0 {
		grace = DefaultGrace
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	cmd.SysProcAttr = procAttr(c)
	cmd.WaitDelay = 2 * time.Second // não espera para sempre por netos que seguram o pipe
	if c.Stdin {
		cmd.Stdin = os.Stdin
	}
	var buf bytes.Buffer
	w := &lockedWriter{w: io.MultiWriter(&buf, out)}
	cmd.Stdout, cmd.Stderr = w, w

	if err := cmd.Start(); err != nil {
		return Result{ExitCode: -1, Err: err}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		// Encerramento gentil primeiro: apt/dpkg e brew precisam terminar de forma limpa.
		signalGroup(cmd, !c.Stdin, syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(grace):
			signalGroup(cmd, !c.Stdin, syscall.SIGKILL)
			<-done
		}
		return Result{ExitCode: -1, Output: buf.String(), Err: ctx.Err()}
	}
	res := Result{Output: buf.String()}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	case err != nil:
		res.ExitCode, res.Err = -1, err
	}
	return res
}

func signalGroup(cmd *exec.Cmd, group bool, sig syscall.Signal) {
	if group {
		_ = syscall.Kill(-cmd.Process.Pid, sig)
		return
	}
	_ = cmd.Process.Signal(sig)
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
