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
// SIGHUP) viram interruptMsg, para o executor não ficar órfão com filhos vivos. Com o terminal
// cedido a um comando interativo, o SIGINT é descartado (o filho já o recebeu).
func signalFilter(tm tea.Model, msg tea.Msg) tea.Msg {
	m, ok := tm.(Model)
	if !ok || m.screen != ScreenExecute || !m.exec.running {
		return msg
	}
	switch msg.(type) {
	case tea.InterruptMsg:
		if m.exec.busy { // terminal cedido (sudo/chsh): o ctrl+c é do filho, como no ignoreSignals do Bubble Tea
			return nil
		}
		return interruptMsg{}
	case tea.QuitMsg:
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

// runSignals são os sinais que a TUI trata durante todo o p.Run (o handler do Bubble Tea fica
// desligado: ele para de ouvir depois do primeiro sinal, e um segundo SIGINT/SIGTERM no meio do
// run mataria o processo com o terminal cru e os filhos órfãos).
var runSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP}

// signalMsg traduz o sinal como o Bubble Tea faria: SIGINT vira InterruptMsg, os outros QuitMsg.
func signalMsg(s os.Signal) tea.Msg {
	if s == syscall.SIGINT {
		return tea.InterruptMsg{}
	}
	return tea.QuitMsg{}
}

// forwardSignals encaminha cada sinal (não só o primeiro) até stop fechar; o filtro decide o que fazer.
func forwardSignals(sigs <-chan os.Signal, send func(tea.Msg), stop <-chan struct{}) {
	for {
		select {
		case s := <-sigs:
			send(signalMsg(s))
		case <-stop:
			return
		}
	}
}

// Run abre a TUI no terminal, liga o hook interativo e, ao sair, imprime a mensagem final.
func Run(s *app.Session, env map[string]string) error {
	m, err := initialModel(s, env)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithFilter(signalFilter), tea.WithoutSignalHandler())
	m.SetProgram(p)

	// Os sinais ficam assinados até o run ter sido drenado (stopRun): nenhum deles pode matar o
	// processo no meio do caminho.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, runSignals...)
	stop := make(chan struct{})
	go forwardSignals(sigs, p.Send, stop)
	final, err := p.Run()

	if fm, ok := final.(Model); ok {
		fm.stopRun()
		if msg := fm.FinalMessage(); msg != "" {
			fmt.Fprintln(os.Stdout, msg)
		}
	}
	signal.Stop(sigs)
	close(stop)
	if errors.Is(err, tea.ErrInterrupted) { // SIGINT fora de um run: sai como o q
		return nil
	}
	return err
}
