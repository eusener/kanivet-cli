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

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, c Cmd, out io.Writer) Result {
	argv := c.Argv
	if c.Shell != "" {
		argv = []string{"sh", "-c", c.Shell}
	}
	if c.Sudo {
		argv = append([]string{"sudo"}, argv...)
	}
	if len(argv) == 0 {
		return Result{ExitCode: -1, Err: errors.New("empty command")}
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
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

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
