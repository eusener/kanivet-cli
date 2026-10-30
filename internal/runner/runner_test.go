package runner

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestExecEcho(t *testing.T) {
	var out strings.Builder
	r := ExecRunner{}.Run(context.Background(), Cmd{Argv: []string{"sh", "-c", "echo hi; echo err 1>&2"}}, &out)
	if r.ExitCode != 0 || !strings.Contains(r.Output, "hi") || !strings.Contains(r.Output, "err") {
		t.Fatalf("result: %+v", r)
	}
	if !strings.Contains(out.String(), "hi") {
		t.Fatalf("stream: %q", out.String())
	}
}

func TestExecShellField(t *testing.T) {
	r := ExecRunner{}.Run(context.Background(), Cmd{Shell: "echo $((1+2))"}, io.Discard)
	if strings.TrimSpace(r.Output) != "3" {
		t.Fatalf("output: %q", r.Output)
	}
}

func TestExecExitCode(t *testing.T) {
	r := ExecRunner{}.Run(context.Background(), Cmd{Argv: []string{"sh", "-c", "exit 3"}}, io.Discard)
	if r.ExitCode != 3 {
		t.Fatalf("exit: %+v", r)
	}
}

func TestExecCancelKillsGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	r := ExecRunner{}.Run(ctx, Cmd{Argv: []string{"sh", "-c", "sleep 30 & sleep 30"}}, io.Discard)
	if time.Since(start) > 2*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
	if r.Err == nil {
		t.Fatal("want error after cancel")
	}
}

func TestFakeRecords(t *testing.T) {
	f := &FakeRunner{Responses: map[string]Result{"brew install fzf": {ExitCode: 1, Output: "boom"}}}
	a := f.Run(context.Background(), Cmd{Argv: []string{"brew", "install", "fzf"}}, io.Discard)
	b := f.Run(context.Background(), Cmd{Argv: []string{"mise", "use", "-g", "node@22"}}, io.Discard)
	if a.ExitCode != 1 || a.Output != "boom" || b.ExitCode != 0 {
		t.Fatalf("responses: %+v %+v", a, b)
	}
	if len(f.Calls) != 2 || f.Calls[1].String() != "mise use -g node@22" {
		t.Fatalf("calls: %+v", f.Calls)
	}
}

func TestCmdStringPrefersDisplay(t *testing.T) {
	if (Cmd{Shell: "curl x | sh", Display: "install lerd"}).String() != "curl x | sh" {
		t.Fatal("String must be the real command")
	}
	if (Cmd{Shell: "curl x | sh", Display: "install lerd"}).Shown() != "install lerd" {
		t.Fatal("Shown must prefer Display")
	}
}

// C1: comandos que leem o terminal precisam ficar no grupo de processos em primeiro plano.
func TestStdinCommandsStayInForegroundGroup(t *testing.T) {
	if procAttr(Cmd{Stdin: true}).Setpgid {
		t.Fatal("Stdin commands must not get a new process group (SIGTTIN would stop them)")
	}
	if !procAttr(Cmd{}).Setpgid {
		t.Fatal("non-interactive commands keep their own group")
	}
}

func TestNonInteractiveSudoIsNonBlocking(t *testing.T) {
	if got := execArgv(Cmd{Argv: []string{"apt-get", "install", "-y", "x"}, Sudo: true}); strings.Join(got, " ") != "sudo -n apt-get install -y x" {
		t.Fatalf("argv %v", got)
	}
	if got := execArgv(Cmd{Argv: []string{"sudo", "-v"}, Stdin: true}); strings.Join(got, " ") != "sudo -v" {
		t.Fatalf("argv %v", got)
	}
}

// I8: no cancelamento, o processo recebe SIGTERM antes do SIGKILL.
func TestCancelSendsTermFirst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	r := ExecRunner{Grace: 2 * time.Second}.Run(ctx, Cmd{Shell: `trap 'echo got-term; exit 0' TERM; while :; do sleep 0.05; done`}, io.Discard)
	if !strings.Contains(r.Output, "got-term") {
		t.Fatalf("TERM trap did not run: %+v", r)
	}
}
