package tui

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/state"
)

// interruptMsg é um sinal externo (SIGINT, SIGTERM, SIGHUP) que chegou durante o run:
// segue o mesmo caminho do ctrl+c (cancela, espera o runDoneMsg e só então sai).
type interruptMsg struct{}

// StartScreen escolhe a tela inicial: sem itens nem perfil no estado é a primeira vez (wizard).
func StartScreen(st *state.State) Screen {
	if st == nil || (len(st.Items) == 0 && st.Profile == "") {
		return ScreenWelcome
	}
	return ScreenDashboard
}

// initialModel monta o modelo inicial. Com um run inacabado, abre a revisão de retomada
// só com os passos pendentes; o esc dela descarta e segue para StartScreen.
func initialModel(s *app.Session, env map[string]string) (Model, error) {
	start := StartScreen(s.St)
	j, pending, err := s.Unfinished()
	if err != nil {
		return Model{}, err
	}
	if j == nil {
		return New(s, env, start), nil
	}
	m := New(s, env, ScreenReview)
	m.review = reviewState{caller: start, resume: true, journal: j}
	return m.onPlan(planMsg{pl: pending, groups: s.GroupPlan(pending)}), nil
}

// signalFilter é o tea.WithFilter: durante o run, InterruptMsg (SIGINT) e QuitMsg (SIGTERM,
// SIGHUP) viram interruptMsg, para o executor não ficar órfão com filhos vivos.
func signalFilter(tm tea.Model, msg tea.Msg) tea.Msg {
	m, ok := tm.(Model)
	if !ok || m.screen != ScreenExecute || !m.exec.running {
		return msg
	}
	switch msg.(type) {
	case tea.InterruptMsg, tea.QuitMsg:
		return interruptMsg{}
	}
	return msg
}

// stopRun cancela um run ainda em curso e drena os eventos até o executor fechar o canal.
// É a rede de segurança depois que o programa sai por outro caminho (ex.: p.Kill).
func (m Model) stopRun() {
	if !m.exec.running || m.exec.cancel == nil {
		return
	}
	m.exec.cancel()
	for range m.exec.events {
	}
}

// Run abre a TUI no terminal, liga o hook interativo e, ao sair, imprime a mensagem final.
func Run(s *app.Session, env map[string]string) error {
	m, err := initialModel(s, env)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithFilter(signalFilter))
	m.SetProgram(p)

	// O Bubble Tea trata SIGINT e SIGTERM; o SIGHUP (terminal fechado) entra como QuitMsg
	// e passa pelo filtro como os outros.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	stop := make(chan struct{})
	go func() {
		select {
		case <-hup:
			p.Send(tea.QuitMsg{})
		case <-stop:
		}
	}()
	final, err := p.Run()
	signal.Stop(hup)
	close(stop)

	if fm, ok := final.(Model); ok {
		fm.stopRun()
		if msg := fm.FinalMessage(); msg != "" {
			fmt.Fprintln(os.Stdout, msg)
		}
	}
	if errors.Is(err, tea.ErrInterrupted) { // SIGINT fora de um run: sai como o q
		return nil
	}
	return err
}
