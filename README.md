# kanivet-cli

Instala e gerencia um ambiente de desenvolvimento focado em terminal: shell,
ferramentas CLI modernas, git, Neovim (LazyVim e outras distribuições),
linguagens, ambiente PHP local (lerd ou Flock), Python/dados, containers e o
terminal **kitty + herdr**. Tudo vem de um catálogo declarativo (`catalog/`) e de
perfis embutidos, com uma TUI para escolher o que instalar e uma CLI completa
para scripts.

Funciona no **macOS** e no **Linux** (Debian/Ubuntu, Fedora, Arch e openSUSE;
x86_64 e arm64). É um binário único em Go, sem dependências em tempo de
execução.

## Início rápido

```sh
go build -o kanivet-cli ./cmd/kanivet-cli   # precisa do Go 1.24+
./kanivet-cli                               # abre a TUI
```

Na primeira vez abre o wizard: boas-vindas, perfil, categorias, revisão,
execução e resumo. Nada é instalado antes da tela de revisão.

## Perfis

| Perfil | O que traz |
|---|---|
| `blank` | Só a base: shell, ferramentas CLI modernas, git, LazyVim e o terminal |
| `web` | Node, pnpm e Bun, ferramentas de HTTP e tarefas, LazyVim para TypeScript |
| `laravel` | PHP via lerd (Linux) ou Flock (macOS), Composer, Node e LazyVim para PHP |
| `ml` | Python, uv, Ruff, JupyterLab e CUDA quando há GPU NVIDIA |
| `backend` | Rust, Go, .NET, Java, Podman e ferramentas de Kubernetes |

Todos incluem a base do `blank`. Depois de escolher o perfil, cada categoria
pode ser ajustada item a item. Em categorias exclusivas (distribuição do
Neovim, lerd ou Flock, Docker ou Podman) só um item fica marcado.

## CLI

```sh
kanivet-cli profiles                       # perfis embutidos
kanivet-cli list                           # itens do catálogo
kanivet-cli status                         # estado de cada item nesta máquina
kanivet-cli plan --profile laravel         # o que seria feito, sem alterar nada
kanivet-cli install --profile laravel      # mostra o plano, pede confirmação e instala
kanivet-cli install lazygit --yes          # itens avulsos, sem perguntar
kanivet-cli install --profile web --add rust --remove bun --version node=24
kanivet-cli install --resume               # retoma uma execução interrompida
kanivet-cli update                         # atualiza o que o kanivet-cli instalou
kanivet-cli uninstall bat                  # desinstala e desfaz as configs do item
kanivet-cli restore                        # lista execuções com backup
kanivet-cli doctor                         # diagnóstico
kanivet-cli catalog lint                   # valida o catálogo
```

## Regras principais

- **Nada muda sem revisão.** O plano mostra cada comando e cada arquivo. Um
  arquivo que já existe aparece marcado com `(backup)` e é copiado para
  `~/.local/state/kanivet-cli/backups/<execução>/` antes de ser alterado;
  `kanivet-cli restore <execução>` desfaz.
- **Configs opinativas são opcionais.** Cada config (kitty, herdr, LazyVim...)
  pode ser desmarcada na revisão ou no dashboard.
- **Sem sudo à toa.** No macOS as ferramentas vêm do Homebrew; no Linux, do mise
  (binários no `~`). Só os itens da seção "Precisa de sudo" pedem a senha, uma
  vez.
- **Linguagens pelo mise**, com as exceções oficiais: Rust pelo rustup e PHP pelo
  lerd/Flock (ou Homebrew no macOS e static-php-cli no Linux, se nenhum dos dois
  for escolhido).
- **Seleção aditiva.** `install` nunca remove nada; `--remove` só tira o item da
  seleção. Remover é sempre `uninstall`.
- **Retomável.** Uma falha não derruba os outros itens. Se a execução for
  interrompida, rodar `kanivet-cli` ou `install` de novo oferece retomar de onde
  parou.
- **Shell.** zsh e bash. Aliases e hooks vão para
  `~/.config/kanivet-cli/shell/init.{zsh,bash}`, carregados por um único bloco
  `# >>> kanivet-cli >>>` no fim do rc. `install --zsh` também troca o shell
  padrão para o zsh.
- **Idioma.** Inglês por padrão; pt-BR quando `LANG`/`LC_ALL` começa com `pt`.

## Interface

`kanivet-cli` sem subcomando, num terminal (stdin e stdout TTY), abre a TUI: o
wizard na primeira vez e o dashboard depois que já há itens ou perfil no estado.
Se houver uma execução inacabada, a TUI abre direto na revisão dos passos
pendentes: `enter` retoma e `esc` descarta. Sem TTY, imprime a ajuda.

| Tecla | Ação |
|---|---|
| setas ou `hjkl` | mover |
| `space` | marcar/desmarcar |
| `enter` | confirmar ou avançar |
| `esc` | voltar |
| `/` | buscar |
| `?` | ajuda |
| `q` | sair (confirma se houver mudanças pendentes) |
| `ctrl+c` | sair ou, durante a execução, interromper |
| `d` (revisão) | diff do arquivo |
| `a`, `u`/`U`, `p` (dashboard) | aplicar, atualizar, trocar de perfil |
| `f` (execução) | voltar a seguir o log |

O rodapé sempre mostra os atalhos da tela. Mínimo de 80×24; `NO_COLOR` desliga
as cores. Os ícones usam Nerd Font (instalada pelo item `nerd-font-jetbrains`),
com fallback ASCII.

## Ferramentas do shell

Com os itens instalados, o init do shell ativa:

| Comando / atalho | O que faz |
|---|---|
| `ls`, `ll`, `lt` | `eza` com ícones; `ll` com status do git, `lt` em árvore |
| `cat` | `bat` sem paginação |
| `z <pasta>` / `zi` | pula para pastas frequentes (zoxide) |
| `ctrl+r` | histórico com o atuin (a seta para cima continua normal) |
| `ctrl+t` / `alt+c` | fzf: arquivos (preview com bat) / entrar numa pasta |
| `y` | abre o yazi e, ao sair, fica na pasta onde você parou |
| `lg` / `lzd` | lazygit / lazydocker |

`grep`, `find`, `sed`, `du`, `df` e `ps` **não** ganham alias, porque as
ferramentas novas usam flags diferentes e quebrariam scripts. Use `rg`, `fd`,
`sd`, `dust`, `duf` e `procs` direto.

## kitty + herdr

As configs opinativas do terminal (todas opcionais e com backup):

- **kitty**: JetBrainsMono Nerd Font 14, abas no topo, fundo translúcido com
  desfoque, tema Afterglow fixo (claro e escuro), e abas/janelas novas no
  diretório atual (`cmd+t`, `cmd+enter`, `cmd+n`; no Linux `ctrl+shift+t`,
  `ctrl+shift+enter`, `ctrl+shift+n`).
- **herdr**: tema vesper, notificações no próprio herdr, atalhos dos plugins e
  lazygit em popup. Plugins herdr-bar, herdr-auto-title, herdr-radar e
  herdr-reviewr, cada um num commit fixo, mais as integrações dos agentes de IA
  encontrados no `PATH`.

O `prefix` do herdr é `ctrl+b`. Atalhos incluídos na config:

| Ação | Atalho |
|---|---|
| Workspace anterior / próximo | `prefix+shift+h` / `prefix+shift+l` |
| Ir para o workspace N | `prefix+shift+1` … `prefix+shift+9` |
| Agente anterior / próximo | `prefix+shift+k` / `prefix+shift+j` |
| Focar o agente N | `prefix+alt+1` … `prefix+alt+9` |
| Voltar ao último painel | `prefix+;` |
| Mover a aba para trás / para frente | `prefix+shift+←` / `prefix+shift+→` |
| Limpar o painel e o histórico | `prefix+ctrl+k` |
| Redimensionar o painel | `prefix+setas` ou `ctrl+alt+h/j/k/l` |
| Lazygit em popup | `prefix+alt+g` |
| Busca rápida (herdr-bar) | `prefix+space` |
| Painel de review do diff (reviewr) | `prefix+d` |
| Radar: alternar ativos / recentes | `prefix+a` |
| Radar: configurações | `prefix+,` |

## Estrutura

```
cmd/kanivet-cli/     # main
catalog/             # catálogo embutido no binário
├── catalog.yaml
├── categories/      # uma categoria por arquivo (exclusive: true/false)
├── items/           # um item por arquivo: métodos de instalação, detecção, configs, shell
├── profiles/        # perfis embutidos
└── configs/         # templates das configs opinativas
internal/
├── platform/        # detecção de SO, distro, gerenciador de pacotes, shell
├── catalog/         # tipos, carga e lint do catálogo
├── backend/         # brew, mise, rustup, system, script, git, font, herdr-plugin
├── detect/          # estado de cada item e atualizações disponíveis
├── plan/            # seleção → passos ordenados (dependências, conflitos, sudo)
├── executor/        # execução paralela, journal, retomada
├── configs/         # aplicação das configs com backup e restauração
├── shellinit/       # geração do init do zsh/bash
├── state/           # estado, journal e backups em ~/.local/state/kanivet-cli
├── app/             # sessão compartilhada pela CLI e pela TUI
├── cli/             # comandos (cobra)
└── tui/             # interface (Bubble Tea)
test/integration/    # instalação real em containers
docs/superpowers/    # design e planos
```

Para adicionar uma ferramenta, crie `catalog/items/<id>.yaml` (use um item
parecido como modelo) e rode `kanivet-cli catalog lint`.

## Testes

```sh
go test -race ./...
test/integration/run.sh <ubuntu|debian|fedora|arch|opensuse> [perfil]   # precisa de Docker
```

Teste manual num terminal real (antes de um release):

1. wizard completo no perfil blank;
2. senha do sudo pedida e aceita no meio da execução;
3. `ctrl+c` durante um `brew install` e depois `kanivet-cli` oferecendo a
   retomada;
4. resize abaixo de 80×24;
5. `NO_COLOR=1 kanivet-cli`.

## Problemas comuns

- **Ícones aparecem como quadrados**: feche e abra o kitty. Se continuar, rode
  `herdr plugin action invoke hhdebb.herdr-radar.install-font`.
- **A barra de título do kitty não mudou de cor**: o kitty precisa ser
  reiniciado; só recarregar a config não basta para essa opção.
- **Um item falhou**: os outros continuam. O resumo mostra o log de cada passo;
  os logs ficam em `~/.local/state/kanivet-cli/runs/<execução>/`. Corrija a causa
  e rode `kanivet-cli install --resume`.
- **Uma integração do herdr foi pulada**: o agente não foi encontrado no `PATH`.
  Instale o agente e rode `kanivet-cli install herdr-plugins` de novo.
- **Algo estranho no ambiente**: `kanivet-cli doctor`.
