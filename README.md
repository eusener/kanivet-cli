# terminal-setup

Script que instala e configura o ambiente de terminal **kitty + herdr** em uma
máquina nova, com a mesma configuração desta: plugins do herdr, integrações com
agentes de IA, fontes (JetBrainsMono Nerd Font e a fonte de ícones do herdr) e um
conjunto de [ferramentas CLI](#ferramentas-cli) via Homebrew.

Funciona no **macOS** e no **Linux** (x86_64 e arm64). Precisa só do Python 3.8+,
sem pacotes extras.

## Início rápido

```sh
# 1. Copie a pasta terminal-setup para a máquina nova (git clone, scp, pendrive...)
cd terminal-setup

# 2. Veja o que vai ser feito, sem alterar nada
python3 setup.py install --dry-run

# 3. Instale e aplique a configuração
python3 setup.py install
```

No fim, **feche e abra o kitty de novo** e rode `herdr`.

## Comandos

| Comando | O que faz |
|---|---|
| `python3 setup.py install` | Instala tudo que falta e aplica as configs de `configs/` |
| `python3 setup.py` | Mesmo que `install` |
| `python3 setup.py install --dry-run` | Só mostra os passos e comandos, sem executar |
| `python3 setup.py install --latest` | Instala os plugins na versão mais recente, em vez do commit salvo |
| `python3 setup.py install --no-cli-tools` | Pula o Homebrew e as ferramentas CLI |
| `python3 setup.py snapshot` | Copia a configuração **desta** máquina para `configs/` |

O `install` pode ser rodado várias vezes: o que já estiver instalado é pulado.

## O que o `install` faz

1. **Detecta o sistema**: macOS ou Linux, e a arquitetura.
2. **Ferramentas básicas**: confere `curl`, `git`, `tar` e `unzip`.
   - No Linux, instala o que faltar (e o `fontconfig`) com `apt`, `dnf`,
     `pacman` ou `zypper`. Pode pedir a senha do `sudo`.
   - No macOS, se faltar o `git`, abre a instalação das Command Line Tools do
     Xcode. Termine essa instalação e rode o script de novo.
3. **go (>= 1.24) e node (>= 18)**: os plugins precisam deles para compilar e
   rodar. Se faltarem ou estiverem antigos, baixa as versões oficiais em
   `~/.local/share/terminal-setup/` e adiciona ao `PATH` no `~/.zshrc` ou
   `~/.bashrc`.
4. **JetBrainsMono Nerd Font**: baixa do repositório oficial do nerd-fonts e
   instala em `~/Library/Fonts` (macOS) ou `~/.local/share/fonts` (Linux).
5. **kitty**: usa o instalador oficial. No Linux, também cria os links `kitty` e
   `kitten` em `~/.local/bin` e o atalho no menu de aplicativos.
6. **herdr**: usa o instalador oficial (`herdr.dev/install.sh`).
7. **Plugins do herdr**: instala os plugins listados em `configs/manifest.json`,
   cada um no commit salvo, e respeita se estava ativado ou desativado.
8. **Configs**: copia os arquivos de `configs/` para `~/.config/kitty` e
   `~/.config/herdr`. Antes, faz backup de cada arquivo existente com o sufixo
   `.bak-setup-AAAAMMDD-HHMMSS`.
9. **Fonte de ícones do herdr**: instala a fonte do plugin herdr-radar e mapeia os
   ícones no kitty.
10. **Integrações**: roda `herdr integration install` para cada agente da lista
    (claude, codex, opencode...), mas **só para os agentes instalados** na
    máquina.
11. Recarrega a configuração do herdr, se ele estiver rodando.
12. **Ferramentas CLI** (pule com `--no-cli-tools`):
    - instala o Homebrew, se faltar (macOS e Linux; pode pedir a senha do `sudo`);
    - instala com `brew` as [ferramentas](#ferramentas-cli) que ainda não estão no
      `PATH`;
    - configura o `delta` como pager do git, **só se** o `core.pager` ainda não
      estiver definido;
    - copia `shell/cli-tools.sh` para `~/.config/terminal-setup/` e adiciona uma
      linha no fim do `~/.zshrc` (ou `~/.bashrc`) para carregá-lo.

## Estrutura

```
terminal-setup/
├── setup.py                 # o script
├── README.md
├── shell/
│   └── cli-tools.sh         # aliases e hooks das ferramentas CLI
└── configs/                 # gerado pelo "snapshot"
    ├── manifest.json        # plugins (repo + commit) e integrações
    ├── kitty/
    │   ├── kitty.conf
    │   ├── dark-theme.auto.conf
    │   └── light-theme.auto.conf
    └── herdr/
        ├── config.toml
        └── plugins-config/  # configs individuais de cada plugin
```

Nos arquivos de `configs/`, o caminho da sua pasta pessoal fica como `{{HOME}}`.
Na instalação, ele é trocado pela pasta pessoal da máquina de destino, então
funciona com qualquer nome de usuário.

## Atualizar a configuração salva

Mudou algo no kitty ou no herdr e quer levar para as outras máquinas? Rode na
máquina de origem:

```sh
python3 setup.py snapshot
```

Isso **apaga e recria** a pasta `configs/` a partir de:

- `~/.config/kitty/kitty.conf` e os temas `*.auto.conf`
- `~/.config/herdr/config.toml` e `~/.config/herdr/plugins/config/`
- a lista de plugins instalados (`~/.config/herdr/plugins.json`)
- as integrações ativas (`herdr integration status`)

Arquivos de backup (`.bak*`), logs e sockets ficam de fora.

## Configuração incluída

Resumo do que a configuração atual aplica:

**kitty**
- Fonte JetBrainsMono Nerd Font Mono, tamanho 14
- Fundo com transparência (0.85) e desfoque
- Barra de título com a cor do fundo e o título da janela (estilo iTerm2)
- Abas no topo, estilo `separator`, com margem `8 8`
- Nova aba (`cmd+t`), janela (`cmd+enter`) e janela do sistema (`cmd+n`) abrem no
  diretório atual (no Linux: `ctrl+shift+t`, `ctrl+shift+enter`, `ctrl+shift+n`)

**herdr**
- Tema `vesper`
- Notificações dentro do próprio herdr, no canto superior direito
- Plugins: herdr-bar, herdr-auto-title, herdr-radar, herdr-reviewr

## Ferramentas CLI

| Categoria | Ferramentas |
|---|---|
| Substitutos | `eza`, `bat`, `fd`, `ripgrep`, `zoxide`, `dust`, `duf`, `btop`, `procs`, `sd` |
| Navegação e busca | `fzf`, `atuin`, `yazi`, `television` |
| Git | `lazygit`, `delta`, `gh`, `jj` |
| Shell e prompt | `starship`, `zsh-autosuggestions`, `zsh-syntax-highlighting`, `direnv`, `mise` |
| Dados, APIs e dev | `jq`, `yq`, `xh`, `just`, `hyperfine`, `watchexec`, `tldr` (tlrc), `uv`, `lazydocker`, `k9s` |

O `shell/cli-tools.sh` só ativa o que estiver instalado:

| Comando / atalho | O que faz |
|---|---|
| `ls`, `ll`, `lt` | `eza` com ícones; `ll` com status do git, `lt` em árvore |
| `cat` | `bat` sem paginação; o `man` também usa o `bat` |
| `top` | `btop` |
| `z <pasta>` / `zi` | pula para pastas frequentes (zoxide) |
| `ctrl+r` | histórico com o atuin (a seta para cima continua normal) |
| `ctrl+t` / `alt+c` | fzf: arquivos (preview com bat) / entrar numa pasta |
| `y` | abre o yazi e, ao sair, fica na pasta onde você parou |
| `lg` / `lzd` | lazygit / lazydocker |
| — | hooks do `direnv` e do `mise`; `starship` (se o rc ainda não iniciou) e plugins do zsh |

`grep`, `find`, `sed`, `du`, `df` e `ps` **não** ganham alias, porque as
ferramentas novas usam flags diferentes e quebrariam scripts e o costume. Use
`rg`, `fd`, `sd`, `dust`, `duf` e `procs` direto.

Para mudar algo, edite `shell/cli-tools.sh` e rode o `install` de novo: a cópia em
`~/.config/terminal-setup/` é sobrescrita a cada execução.

### Atalhos úteis

| Ação | kitty (macOS) | herdr |
|---|---|---|
| Próxima aba | `cmd+shift+]` ou `ctrl+tab` | `prefix+n` |
| Aba anterior | `cmd+shift+[` ou `ctrl+shift+tab` | `prefix+p` |
| Ir para a aba N | `cmd+1` … `cmd+9` | `prefix+1` … `prefix+9` |
| Nova aba | `cmd+t` | `prefix+c` |
| Recarregar config | `ctrl+cmd+,` | `prefix+shift+r` |

O `prefix` do herdr é `ctrl+b`. Atalhos personalizados do herdr incluídos na
configuração:

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

## Desfazer

Cada arquivo substituído ganha uma cópia ao lado, por exemplo:

```sh
ls ~/.config/kitty/*.bak-setup-*
# restaurar:
cp ~/.config/kitty/kitty.conf.bak-setup-20261029-095840 ~/.config/kitty/kitty.conf
```

Para desligar as ferramentas CLI no shell, apague o bloco
`# >>> terminal-setup cli-tools >>>` do `~/.zshrc`. Se o script configurou o delta,
desfaça com `git config --global --unset core.pager` (e o mesmo para
`interactive.diffFilter`, `delta.navigate` e `merge.conflictStyle`).

O script não remove o kitty, o herdr, os plugins, as fontes nem as ferramentas CLI. Para tirar um
plugin, use `herdr plugin uninstall <id>`.

## Problemas comuns

- **`herdr não ficou no PATH`**: abra um terminal novo (para carregar o
  `~/.local/bin` no `PATH`) e rode o script de novo.
- **Ícones aparecem como quadrados**: feche e abra o kitty. Se continuar, rode
  `herdr plugin action invoke hhdebb.herdr-radar.install-font`.
- **A barra de título não mudou de cor**: o kitty precisa ser reiniciado; só
  recarregar a config não basta para essa opção.
- **Um plugin falhou ao instalar**: o script avisa e continua. Veja o log com
  `herdr plugin log <id>` e tente `python3 setup.py install --latest`.
- **Uma ferramenta CLI não instalou**: o script avisa quais falharam e continua.
  Tente `brew install <fórmula>` para ver o erro. No Linux arm64, algumas
  fórmulas podem não ter binário pronto e precisar compilar.
- **Uma integração foi pulada**: o agente não foi encontrado no `PATH`. Instale o
  agente e rode o script de novo.
