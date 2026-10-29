package runner

import (
	"context"
	"io"
	"sync"
)

// FakeRunner registra os comandos e devolve respostas programadas (padrão: sucesso).
type FakeRunner struct {
	mu        sync.Mutex
	Calls     []Cmd
	Responses map[string]Result
	// Func, se definida, responde antes de Responses; ok=false cai no comportamento padrão.
	Func func(ctx context.Context, c Cmd) (Result, bool)
}

func (f *FakeRunner) Run(ctx context.Context, c Cmd, out io.Writer) Result {
	f.mu.Lock()
	f.Calls = append(f.Calls, c)
	fn := f.Func
	r, ok := f.Responses[c.String()]
	f.mu.Unlock()
	if fn != nil {
		if res, handled := fn(ctx, c); handled {
			_, _ = io.WriteString(out, res.Output)
			return res
		}
	}
	if ok {
		_, _ = io.WriteString(out, r.Output)
		return r
	}
	return Result{}
}

// Strings devolve os comandos executados, em ordem.
func (f *FakeRunner) Strings() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.Calls))
	for i, c := range f.Calls {
		out[i] = c.String()
	}
	return out
}
