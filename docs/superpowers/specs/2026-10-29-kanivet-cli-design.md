# kanivet-cli — design

Data: 2026-10-29 · Status: aprovado

Neste documento, **(decisão do usuário)** indica algo que o dono do projeto
decidiu na entrevista e **(decisão do Claude)** indica uma escolha minha, que
ele ainda pode revisar. O que não tem marca é consequência direta das decisões
anteriores.

---

## 1. Intenção

Montar um ambiente de desenvolvimento focado em terminal numa máquina nova
consome horas. É preciso lembrar de dezenas de ferramentas, cada uma com seu
instalador, e configurar shell, editor, terminal e linguagens. Depois disso,
manter tudo atualizado vira um trabalho manual.

O **kanivet-cli** ("canivete") é uma ferramenta open source com uma TUI de
qualidade profissional. Com ela o dev:

1. escolhe um **perfil** (Laravel/PHP, Web, ML/Data, Backend/Sistemas);
2. ajusta o que vai instalar e configurar, item por item;
3. revisa o plano exato antes de qualquer alteração;
4. acompanha a instalação ao vivo;
5. depois, usa a mesma ferramenta para ver o estado da máquina, adicionar,
   remover e atualizar itens.

Ela nasce a partir deste repositório (`terminal-setup`), que hoje é um script
Python que instala kitty, herdr, plugins, fontes e cerca de 30 ferramentas CLI.

## 2. Objetivos, não-objetivos e anti-objetivos

### Objetivos (v1)
- Instalar e configurar um ambiente completo de dev em terminal no macOS e no
  Linux, com um único binário e sem dependências prévias além de `curl`.
- Ter uma TUI com acabamento profissional: wizard na primeira execução e
  dashboard nas seguintes **(decisão do usuário)**.
- Oferecer uma CLI completa e não interativa com o mesmo motor da TUI
  **(decisão do usuário)**.
- Trazer quatro perfis embutidos que pré-marcam itens; o dev ajusta livremente
  **(decisão do usuário)**.
- Funcionar como gerenciador contínuo: guardar estado, mostrar o que está
  desatualizado, atualizar e desinstalar **(decisão do usuário)**.
- Ter um catálogo declarativo e fácil de estender por qualquer contribuidor
  **(decisão do usuário)**.

### Não-objetivos (v1)
- Windows nativo e WSL ficam fora **(decisão do usuário)**.
- Perfis como arquivos externos ou importados de URL ficam fora; só presets
  embutidos **(decisão do usuário)**.
- Sincronizar dotfiles pessoais do usuário fica fora da v1 **(decisão do
  Claude)**: pode entrar depois como um item "aplicar repo de dotfiles".
- Não é um gerenciador de pacotes: ele orquestra os instaladores existentes
  (brew, mise, rustup, scripts oficiais) e não substitui nenhum deles.
- Não gerencia projetos: ele não cria nem configura apps Laravel/Next; isso é
  papel do lerd, do Flock, do `laravel new` etc.

### Anti-objetivos (é fracasso, mesmo que funcione) **(decisão do usuário)**
1. **Alterar o sistema sem avisar.** Todo arquivo de rc, dotfile ou config que
   for tocado aparece no plano antes e ganha backup.
2. **Exigir sudo sem necessidade.** A instalação fica no `~` do usuário sempre
   que possível. Itens que precisam de sudo são sinalizados no plano e a senha
   é pedida uma vez só, no início da execução.
3. **Quebrar no meio sem conseguir retomar.** Uma falha num item não derruba os
   outros. Rodar de novo retoma a execução e a máquina nunca fica num estado
   que a ferramenta não consiga descrever.
4. **Catálogo difícil de estender.** Adicionar uma ferramenta comum deve exigir
   apenas um arquivo de dados, sem código Go.

## 3. Restrições

- **Linguagem e TUI:** Go + ecossistema Charm (Bubble Tea, Bubbles, Lip Gloss;
  Huh nos formulários) **(decisão do usuário)**.
- **Sistemas:** macOS (Apple Silicon e Intel), Linux Debian/Ubuntu e outras
  distros (Fedora, Arch, openSUSE), em x86_64 e arm64 **(decisão do usuário)**.
- **Público:** open source público **(decisão do usuário)**. Isso exige docs,
  releases, perfis genéricos (sem as preferências pessoais do autor) e testes
  em várias distros.
- **Nome:** `kanivet-cli` para o repositório, a fórmula e o **binário**
  **(decisão do usuário quanto ao nome; o binário com o mesmo nome é decisão do
  Claude)**. Existe um produto "Kanivet" de Kubernetes (`kanivet-ai`) com tap
  Homebrew próprio, então o binário **não** pode se chamar `kanivet`.
- **Licença:** MIT **(decisão do Claude)**. Os itens do catálogo só chamam
  instaladores de terceiros, então licenças como a AGPL do Flock não contaminam
  o projeto.
- **Idioma:** i18n desde a v1, com inglês como padrão e pt-BR escolhido pelo
  `LANG`/`LC_ALL` **(decisão do usuário)**. Tanto as strings da TUI e da CLI
  quanto os campos de texto do catálogo (`name`, `description`, `notes`) nascem
  traduzíveis. No catálogo, os campos de texto aceitam um mapa por idioma
  (`description: {en: ..., pt-BR: ...}`) com fallback para `en`. O lint exige
  `en` em todos os itens.
- **Shells:** zsh e bash **(decisão do usuário)**. Se o usuário estiver em
  bash, a ferramenta oferece o item "zsh como shell padrão", desmarcado por
  padrão, que instala o zsh e roda `chsh`. Esse item pede senha e aparece
  sinalizado no plano. Itens só-zsh (zsh-autosuggestions,
  zsh-syntax-highlighting) ficam desabilitados com o motivo quando o shell
  alvo é bash. O fish fica para depois.
- **Bootstrap:** numa máquina zerada, o único pré-requisito é
  `curl -fsSL <url>/install.sh | sh`, que baixa o binário para `~/.local/bin`.

## 4. Arquitetura

```
┌───────────── TUI (Bubble Tea) ─────────────┐  ┌──── CLI (cobra) ────┐
│ wizard · dashboard · revisão · progresso   │  │ install/status/...  │
└──────────────────────┬─────────────────────┘  └─────────┬───────────┘
                       └──────────────┬───────────────────┘
                                ┌─────▼─────┐
                                │  Planner  │  seleção → plano (DAG) + diff
                                └─────┬─────┘
       ┌──────────────┬───────────────┼───────────────┬──────────────┐
  ┌────▼────┐   ┌─────▼─────┐   ┌─────▼─────┐   ┌─────▼─────┐  ┌─────▼─────┐
  │ Catálogo│   │  Detector │   │ Executor  │   │  Configs  │  │  Estado   │
  │ (YAML)  │   │ (status)  │   │ (backends)│   │ (backups) │  │ (journal) │
  └─────────┘   └───────────┘   └───────────┘   └───────────┘  └───────────┘
```

- **Catálogo:** são os itens, as categorias e os perfis em YAML. Vem embutido
  no binário (`embed.FS`) e pode ser substituído por uma versão remota mais
  nova (seção 10).
- **Detector:** descobre o estado real de cada item na máquina (ausente,
  instalado, desatualizado, instalado por fora do kanivet-cli) e a versão.
- **Planner:** recebe a seleção do usuário e o estado atual e produz um
  **plano**. O plano é um grafo de passos ordenado por dependência, com tudo o
  que será executado: comandos, arquivos alterados, backups e se precisa de
  sudo. A TUI e a CLI exibem esse mesmo plano.
- **Executor:** executa os passos por meio de **backends** (seção 5.3). Roda em
  paralelo o que é independente, com limite de concorrência, e serializa o que
  usa o mesmo gerenciador (por exemplo, `brew` não aceita execuções
  concorrentes). Faz streaming do output de cada passo para a TUI e para um log
  por item.
- **Configs:** aplica as configs opinativas a partir de templates, faz backup
  dos arquivos e gerencia os blocos que a ferramenta insere nos arquivos de rc
  do shell.
- **Estado:** guarda o registro persistente do que foi instalado ou aplicado
  pelo kanivet-cli, além de um journal da execução corrente, que é o que
  permite retomar.

A TUI e a CLI são finas: toda a lógica fica no motor, que é testável sem
terminal.

## 5. Catálogo declarativo

### 5.1 Estrutura

```
catalog/
├── catalog.yaml           # versão do schema, versão do catálogo, ordem das categorias
├── categories/*.yaml      # id, nome, descrição, ícone, exclusive: true|false
├── items/<id>.yaml        # um arquivo por item
├── profiles/<id>.yaml     # perfis embutidos
└── configs/<id>/...       # templates de config opinativa
```

### 5.2 Formato de um item (ilustrativo, a ser finalizado pelo construtor)

```yaml
id: lazygit
name: lazygit
category: git
description: TUI de git — staging por linha, rebase interativo, conflitos.
homepage: https://github.com/jesseduffield/lazygit
platforms: [macos, linux]
depends_on: []
detect:
  command: lazygit --version
  version_regex: 'version=([\d.]+)'
install:                       # o primeiro método compatível com a máquina vence
  - backend: brew
    package: lazygit
    when: { os: macos }
  - backend: mise
    package: aqua:jesseduffield/lazygit
update: auto                   # derivado do backend
uninstall: auto
configs:
  - id: lazygit-theme
    description: Tema e atalhos opinativos
    default: true
    files:
      - template: configs/lazygit/config.yml
        target: "{{.XDG_CONFIG}}/lazygit/config.yml"
shell:
  aliases: { lg: lazygit }
```

Também fazem parte do schema:
- `needs_sudo: true` (por exemplo, o DNS do lerd e pacotes do sistema);
- `notes`, mostradas no detalhe do item e na revisão (por exemplo, "o lerd exige
  systemd e Podman rootless");
- `post_install`, para passos extras como `lerd init`;
- `requires`, para pré-requisitos do sistema (por exemplo, `podman` e `systemd`
  para o lerd);
- `shell.init`, com snippets de init por shell (zoxide, atuin, mise, direnv,
  starship);
- `env`, com variáveis exportadas.

Categorias com `exclusive: true` viram seleção única na TUI: o dev escolhe um
item ou nenhum **(decisão do usuário)**.

### 5.3 Backends

Os backends são o único lugar com código Go por tipo de instalação. Na v1:

| Backend | Uso | Sudo |
|---|---|---|
| `brew` (fórmula/cask) | padrão no macOS; apps GUI (kitty, Zed, VS Code) via cask | não* |
| `mise` | runtimes (node, python, go, java, dotnet, php…) e CLIs via `aqua:`/`github:`/`cargo:` | não |
| `rustup` | Rust **(decisão do usuário: exceção oficial)** | não |
| `script` | instaladores oficiais `curl \| sh` (lerd, herdr, kitty no Linux, Homebrew) com URL e, quando houver, checksum | depende |
| `system` | apt/dnf/pacman/zypper para pré-requisitos do sistema (build-essential, podman, fontconfig) | sim |
| `git` | clonar configs (LazyVim starter, AstroNvim…) | não |
| `font` | Nerd Fonts em `~/Library/Fonts` / `~/.local/share/fonts` | não |
| `herdr-plugin` | plugins do herdr, fixados por commit | não |

\* A primeira instalação do Homebrew no macOS pede sudo.

**Estratégia de instalação das ferramentas CLI (decisão do Claude):** no macOS
o padrão é `brew`. No Linux o padrão é `mise` com backends `aqua`/`github`, que
instala binários no `~` sem sudo. Isso evita o Linuxbrew, que exige sudo para
criar `/home/linuxbrew`, e cumpre o anti-objetivo 2. O script atual usa brew
nas duas plataformas; isso muda. Cada item declara uma lista de métodos e o
primeiro compatível vence. Um item pode forçar `brew` no Linux se for
necessário.

**Linguagens (decisão do usuário):** o `mise` gerencia tudo, exceto o Rust
(`rustup`). O PHP é uma segunda exceção proposta pelo Claude (ver 15.3). A versão global fica em `~/.config/mise/config.toml`, e o dev
escolhe a versão no detalhe do item, com um padrão sensato: a LTS ou a stable
mais recente.

### 5.4 Itens que já existem

Tudo o que o detector encontra instalado por fora (por exemplo, o `starship`
via brew pelo próprio usuário) aparece como "instalado (externo)". O
kanivet-cli **não reinstala** esses itens e não desinstala o que não instalou.
Atualizar um item externo só acontece se o backend dele for identificável, e
sempre com confirmação **(decisão do Claude)**.

## 6. Categorias e itens da v1

| Categoria | Itens | Exclusiva? |
|---|---|---|
| Terminal | kitty (+ config opinativa), herdr (+ plugins + config), JetBrainsMono Nerd Font | não |
| Shell | starship, zsh-autosuggestions, zsh-syntax-highlighting, direnv, atuin, zoxide, fzf | não |
| Distro Neovim | LazyVim, AstroNvim, NvChad, kickstart.nvim, Neovim puro | **sim** |
| Editores | Helix, Zed, VS Code | não |
| Substitutos de comandos clássicos | eza, bat, fd, ripgrep, dust, duf, btop, procs, sd | não |
| Navegação | yazi, television | não |
| Git | lazygit, delta (+ config do git), gh, jj | não |
| Linguagens | PHP + Composer, Node (+ pnpm/bun), Python, Go, Rust (rustup), .NET, Java | não |
| Ambiente PHP local | lerd, Flock | **sim** |
| Python / Data | uv, ruff, Jupyter (via uv), CUDA toolkit (só Linux com GPU NVIDIA; fica de fora se não for detectada) | não |
| Containers e infra | Podman ou Docker, lazydocker, k9s, kubectl | Podman e Docker formam um subgrupo exclusivo |
| Dados e APIs | jq, yq, xh, just, hyperfine, watchexec, tldr (tlrc) | não |
| Gerenciadores | mise (base, sempre instalado quando alguma linguagem é marcada) | — |

O LazyVim recebe extras por perfil: `lang.php` no Laravel, `lang.python` no
ML, `lang.rust`, `lang.go` e `lang.omnisharp` no Backend, `lang.typescript` no
Web **(decisão do Claude)**.

O lerd e o Flock disputam as portas 80/443, por isso a categoria é exclusiva.
Ambos têm requisitos próprios: o lerd precisa de systemd e Podman rootless no
Linux; o Flock suporta macOS 15+ em Apple Silicon e Ubuntu 24.04+/Fedora. Se o
sistema não atende aos requisitos, o item aparece desabilitado com o motivo.

## 7. Perfis embutidos

Os perfis são presets: pré-marcam itens e escolhem o padrão das categorias
exclusivas **(decisão do usuário)**. Todo perfil inclui a base (Terminal,
Shell, Substitutos, Git, LazyVim). O conteúdo detalhado abaixo é uma **decisão
do Claude**:

| Perfil | Além da base |
|---|---|
| **Laravel / PHP** | lerd (padrão no Linux) ou Flock (padrão no macOS), que já fornecem PHP + Composer; Node + pnpm, LazyVim `lang.php`, lazydocker |
| **Web (JS/TS)** | Node + pnpm + bun, LazyVim `lang.typescript`/`tailwind`, xh, just, watchexec |
| **ML / Data** | Python, uv, ruff, Jupyter, CUDA (se houver GPU), LazyVim `lang.python`, jq/yq |
| **Backend / Sistemas** | Rust, Go, .NET, Java, Podman, kubectl, k9s, lazydocker, LazyVim `lang.rust`/`go`/`omnisharp` |

Também há a opção "Começar do zero", sem nada pré-marcado além da base. O
estado registra qual perfil o dev escolheu, apenas para referência.

## 8. Experiência

### 8.1 Primeira execução: wizard **(decisão do usuário)**
1. **Boas-vindas:** mostra o sistema detectado (SO, arquitetura, distro,
   shell) e avisos (por exemplo, "sem systemd: lerd indisponível").
2. **Perfil:** lista de perfis com descrição e prévia dos itens.
3. **Categorias:** uma tela por grupo de categorias, com seleção de itens. As
   categorias exclusivas usam rádio. Cada item tem um painel de detalhe com
   descrição, versão, método de instalação, se precisa de sudo e um toggle da
   config opinativa.
4. **Revisão do plano:** passos agrupados, contadores (instalar, configurar,
   pular), itens que pedem sudo em destaque, arquivos que serão alterados com
   o caminho do backup e um diff das configs sob demanda.
5. **Execução:** lista de passos com spinner e status, painel com o log ao
   vivo do passo selecionado e barra de progresso geral. As falhas aparecem
   sem interromper os demais passos.
6. **Resumo:** o que deu certo, o que falhou (com o caminho do log e um comando
   para tentar de novo) e próximos passos (por exemplo, "abra um terminal
   novo").

### 8.2 Execuções seguintes: dashboard **(decisão do usuário)**
- Dois painéis no estilo lazygit: categorias à esquerda; itens à direita com
  status (✓ instalado, ↑ atualização, ○ ausente, ◇ externo, ✗ falhou) e o
  detalhe embaixo.
- Barra inferior com as mudanças pendentes ("3 a instalar · 2 a atualizar · 1
  a remover") e atalho para revisar e aplicar, que leva às mesmas telas de
  revisão e execução do wizard.
- Ações: marcar e desmarcar, atualizar um ou todos, desinstalar, trocar o
  perfil de referência e reaplicar configs.
- Busca com `/`, ajuda com `?` e atalhos sempre visíveis no rodapé.

### 8.3 Qualidade visual **(decisão do Claude, para o construtor refinar)**
- Tema próprio e coeso via Lip Gloss, que se adapta a fundo claro e escuro.
- Uso de cores ANSI respeitando `NO_COLOR`.
- Ícones Nerd Font apenas quando a fonte estiver presente, com fallback ASCII.
- Layout responsivo a partir de 80×24.
- Navegação por teclado completa (setas e hjkl).
- O mouse é opcional.

### 8.4 CLI **(decisão do usuário)**
```
kanivet-cli                         # abre a TUI (wizard ou dashboard)
kanivet-cli install --profile laravel --add rust --remove flock --yes
kanivet-cli install <item>...       # itens avulsos
kanivet-cli status [--json]
kanivet-cli update [<item>...]
kanivet-cli uninstall <item>...
kanivet-cli plan --profile web      # só mostra o plano (dry-run)
kanivet-cli list [--category X]     # catálogo
kanivet-cli profiles
kanivet-cli catalog update          # busca o catálogo remoto
kanivet-cli doctor                  # diagnóstico do ambiente
```
Toda ação aceita `--dry-run`. Sem TTY, o padrão é falhar pedindo `--yes`, em
vez de travar à espera de resposta.

**Seleção aditiva (decisão do usuário):** `install` só adiciona. Rodar
`install --profile web` numa máquina com o perfil Laravel instala o que falta
do Web e não remove nada. `--remove X` apenas tira X da seleção daquele
comando; não desinstala. A remoção é sempre explícita: `uninstall` na CLI ou
desmarcar um item instalado no dashboard, que aparece como "a remover" na
revisão. Não existe um "conjunto desejado" persistido: o estado registra o que
está instalado e configurado, e não o que deveria estar.

## 9. Estado, retomada e segurança

- **Estado:** fica em `~/.local/state/kanivet-cli/state.json`. Registra os
  itens instalados pelo kanivet-cli (backend, versão e data), as configs
  aplicadas (arquivo, backup e hash do conteúdo aplicado) e o perfil de
  referência.
- **Journal:** fica em `~/.local/state/kanivet-cli/runs/<timestamp>/`, com o
  plano serializado, o status de cada passo e um log por item. Se a execução
  for interrompida, a próxima abertura oferece retomar o plano pendente.
- **Idempotência:** cada passo verifica o estado real antes de agir (pelo
  detector). Por isso, reexecutar é sempre seguro.
- **Backups:** todo arquivo existente que for alterado é copiado para
  `~/.local/state/kanivet-cli/backups/<timestamp>/<caminho>`. Isso centraliza
  os backups, em vez de espalhar arquivos `.bak` ao lado de cada original
  **(decisão do Claude)**. Existe também `kanivet-cli restore <run>`.
- **Configs alteradas pelo usuário:** antes de reaplicar uma config, a
  ferramenta compara o hash. Se o arquivo mudou depois da aplicação, avisa e
  oferece diff, sobrescrever ou manter.
- **Integração com o shell:** os snippets de todos os itens são gerados num
  arquivo gerenciado por shell (`~/.config/kanivet-cli/shell/init.zsh`,
  `init.bash` e `init.fish`, se o fish for adicionado no futuro). Esse arquivo
  é carregado por **um único bloco** com marcadores no fim do `~/.zshrc` ou
  `~/.bashrc`. A ordem de inicialização é definida no catálogo (por exemplo,
  `zsh-syntax-highlighting` por último e atuin depois do fzf), e a regra de não
  iniciar o starship duas vezes continua valendo. O mesmo vale para o
  `cli-tools.sh` atual.
- **Sudo:** se o plano tiver passos com sudo, a senha é pedida uma vez antes
  da execução, com `sudo -v` e keep-alive. Esses passos ficam agrupados no
  início do plano.
- **Scripts remotos:** itens do backend `script` declaram URL HTTPS fixa. Ver
  o script antes de executar é uma ação disponível na revisão.

## 10. Atualização remota do catálogo **(decisão do usuário)**

- O binário traz um catálogo embutido, o que garante funcionamento offline.
- `kanivet-cli catalog update` (e uma checagem leve ao abrir a TUI, no máximo
  uma vez por dia, que pode ser desligada) baixa o catálogo mais recente,
  publicado como asset de release no repositório do projeto. O download fica
  em `~/.local/share/kanivet-cli/catalog/`.
- **Integridade (decisão do Claude):** o catálogo é assinado (minisign ou
  cosign, a definir pelo construtor), e a chave pública fica embutida no
  binário. Um catálogo sem assinatura válida é descartado. Isso é obrigatório,
  porque o catálogo define comandos executados na máquina.
- **Compatibilidade:** o catálogo declara a versão do schema. Se ela for maior
  que a suportada pelo binário, o catálogo é ignorado e a TUI sugere atualizar
  o binário.
- Vale a versão mais nova entre a embutida e a baixada.

## 11. Distribuição

- Releases com GoReleaser: binários darwin/linux × amd64/arm64, checksums e
  assinatura.
- `install.sh`: detecta a plataforma, baixa o binário para `~/.local/bin`,
  confere o checksum e garante o diretório no `PATH`.
- Tap Homebrew próprio (`<owner>/homebrew-tap`, fórmula `kanivet-cli`). A
  fórmula no homebrew-core fica para depois, quando o projeto tiver tração.
- `kanivet-cli self-update` **(decisão do Claude)**.

## 12. Testes

- **Unitários:** planner (ordenação, conflitos, exclusividade, diff de
  estado), detector (parsing de versões), configs (backup, hash, blocos de rc),
  estado e journal.
- **Lint do catálogo:** um comando, também usado no CI, que valida o schema,
  os IDs únicos, as dependências existentes e a ausência de ciclos, os
  templates referenciados e o suporte a pelo menos uma plataforma por item.
  Isso é o que torna o catálogo seguro de receber contribuições.
- **TUI:** testes de golden output com `teatest`.
- **Integração:** containers Ubuntu 24.04, Debian, Fedora, Arch e openSUSE
  rodando `kanivet-cli install --profile <p> --yes` e em seguida
  `kanivet-cli status --json`. Também roda em runner macOS no CI. Itens que
  exigem systemd ou GPU são pulados nos containers.
- Todo backend aceita um modo "fake" para testes rápidos do executor.

## 13. Migração do `terminal-setup`

- Este repositório passa a ser o `kanivet-cli`. O `setup.py`, o
  `shell/cli-tools.sh` e `configs/` são aposentados quando a v1 cobrir o que
  eles fazem **(decisão do Claude)**.
- kitty, herdr, os plugins do herdr (fixados por commit), as integrações de
  agentes, a Nerd Font, a fonte de ícones do herdr-radar e todas as
  ferramentas CLI viram itens do catálogo **(decisão do usuário)**.
- As configs pessoais atuais do kitty e do herdr **deixam de ser o padrão**
  **(decisão do usuário)**. A config opinativa da v1 é uma versão neutra,
  derivada delas: sem caminhos ou atalhos pessoais e com o tema genérico.
- A lógica de detecção de agentes de IA (`AGENT_BINARIES`) é reaproveitada no
  item herdr: a integração só é instalada para agentes presentes.

## 14. Fases sugeridas

1. **Motor + catálogo + CLI:** schema, lint, detector, planner, executor com os
   backends `brew`, `mise`, `rustup`, `script`, `system`, `git`, `font` e
   `herdr-plugin`, estado e journal, configs e shell, e os comandos `plan`,
   `install`, `status`, `update`, `uninstall`. Já é útil sozinho, sem TUI.
2. **TUI:** wizard, dashboard, revisão e execução ao vivo.
3. **Catálogo remoto assinado + distribuição:** GoReleaser, `install.sh`, tap
   e self-update.
4. **Ampliação do catálogo e polimento:** extras do LazyVim por perfil, mais
   distros nos testes e docs.

## 15. Regras de comportamento (respostas da revisão) **(decisões do Claude)**

### 15.1 "Desatualizado" e `update`
- Por padrão, cada item segue a **última versão upstream** do seu backend:
  `brew outdated`, `mise outdated`, a release mais recente no GitHub para
  itens `script`/`aqua`. O detector consulta a rede só quando o usuário pede o
  status de atualizações (ao abrir o dashboard, em background e com cache de
  algumas horas, ou com `status --check-updates`).
- O catálogo **fixa** versão só quando há motivo: os plugins do herdr por
  commit, as configs git (LazyVim starter) por tag. Atualizar o catálogo
  remoto é o que move esses pinos.
- **Linguagens:** `update` só atualiza dentro da linha escolhida (Node 22.x →
  22.y, Python 3.13.x → 3.13.y). Trocar de linha (Node 22 → 24) é uma ação
  explícita no detalhe do item e nunca acontece num `update` geral.

### 15.2 Ferramentas instaladas por fora
- Uma ferramenta externa satisfaz o item se a versão for maior ou igual ao
  `min_version` do item no catálogo. Por exemplo, o fzf exige 0.48 ou mais,
  porque o init usa `fzf --zsh`.
- Se a versão externa for menor, o item fica como "externo desatualizado" e o
  plano oferece instalar a versão do kanivet-cli. O shell init gerado protege o
  uso por versão quando isso é viável e nunca gera init que quebre o shell.
- Configs opinativas e shell init podem ser aplicados sobre itens externos,
  porque são independentes da instalação. Por exemplo, um starship instalado
  pelo brew do usuário recebe init (se ainda não houver um) e config (se
  marcada).
- **Linguagens externas:** o node do nvm, o python do pyenv ou do sistema etc.
  são detectados e exibidos. Se o usuário marcar a linguagem, a revisão avisa
  que a versão global do mise vai ter precedência no `PATH` sobre a externa.

### 15.3 PHP
- **Com o lerd selecionado, o PHP não é instalado na máquina** **(decisão do
  usuário)**. O lerd já traz o PHP (7.4 a 8.6, por projeto), então o item PHP
  fica como "fornecido pelo lerd": o planner não gera nenhum passo de
  instalação de PHP e a revisão mostra isso. **(decisão do Claude)** O mesmo
  vale para o Flock, que também traz o próprio PHP.
- No catálogo, isso é declarado com `provides: [php, composer]` no item lerd e
  no item Flock. Um item fornecido por outro selecionado é satisfeito sem
  instalação.
- Sem lerd nem Flock, o PHP é a **segunda exceção à regra do mise**, ao lado
  do Rust, porque o mise compila PHP a partir do código-fonte (lento, frágil e
  com muitas dependências de sistema) **(decisão do Claude, aprovada pelo
  usuário)**:
  1. no macOS, `brew` (com `shivammathur/php` para várias versões);
  2. no Linux, os binários prebuilt do static-php-cli, instalados no `~`.
- Nesses dois casos, o Composer é instalado junto.

### 15.4 Desinstalação e restore
- `uninstall <item>` remove a instalação, se ela foi feita pelo kanivet-cli. Também
  remove as configs aplicadas daquele item, restaurando o backup se existir
  (com aviso quando o arquivo mudou depois da aplicação), e tira os snippets
  do item do shell init gerado.
- As dependências que ficaram órfãs (por exemplo, o mise sem nenhuma linguagem)
  são **sugeridas** para remoção na revisão, desmarcadas por padrão.
- `restore <run>` restaura **somente arquivos** a partir dos backups daquela
  execução e não desfaz instalações.

### 15.5 Base, herdr e agentes de IA
- As dependências são resolvidas automaticamente pelo planner. Os plugins do
  herdr precisam de Go ≥ 1.24 e Node ≥ 18, que entram via mise como
  dependências do item "plugins do herdr" e aparecem na revisão como
  "dependência de X".
- A base de todos os perfis é Shell + Substitutos + Git + LazyVim. A
  categoria **Terminal** (kitty, herdr, Nerd Font) é pré-marcada só quando há
  sessão gráfica (`DISPLAY`/`WAYLAND_DISPLAY` no Linux; sempre no macOS). Em
  servidores ou por SSH ela fica desmarcada.
- Os agentes de IA (claude, codex…) **não** são itens do catálogo na v1. Eles
  são apenas detectados para instalar as integrações do herdr.

## 16. Deixado para o construtor

- A estrutura de pacotes Go, as bibliotecas auxiliares (cobra ou similar para
  a CLI, yaml) e a escolha entre minisign e cosign.
- O schema YAML final, a partir do ilustrativo da seção 5.2.
- A paleta, o espaçamento e o layout exato das telas, respeitando a seção 8.
- O limite de concorrência e a estratégia de serialização por backend.
- As versões padrão de cada linguagem, se "LTS/stable mais recente" for
  ambíguo.
- O conteúdo exato das configs opinativas neutras.

## 17. Resumo das decisões do Claude (para revisão)

1. No Linux, as ferramentas CLI vão via `mise` (aqua/github) e não via
   Linuxbrew, para evitar sudo. No macOS fica o `brew`.
2. O binário se chama `kanivet-cli` (e não `kanivet`), por causa do conflito
   com o produto de Kubernetes.
3. Licença MIT.
4. O catálogo remoto é assinado, e o binário rejeita catálogos sem assinatura.
5. Os backups ficam centralizados em `~/.local/state/kanivet-cli/backups/`.
6. O kanivet-cli não reinstala nem desinstala itens instalados por fora.
7. O conteúdo dos perfis e os padrões lerd/Flock por SO estão na seção 7.
8. Os extras do LazyVim dependem do perfil.
9. Sincronização de dotfiles pessoais fica fora da v1.
10. Este repositório vira o kanivet-cli e o `setup.py` é aposentado na v1.
11. Existe um comando `self-update`.
12. **PHP fica fora do mise:** com lerd/Flock não se instala PHP (`provides`);
    sem eles, vem do brew no macOS ou do static-php-cli no Linux (seção 15.3).
    Aplicar a regra do lerd também ao Flock é decisão do Claude.
13. `update` segue a versão upstream, mas linguagens só mudam dentro da linha
    escolhida (15.1).
14. Ferramentas externas valem se a versão for maior ou igual a `min_version`;
    configs e init podem ser aplicados sobre elas (15.2).
15. `uninstall` remove configs e init do item; órfãos são só sugeridos;
    `restore` mexe só em arquivos (15.4).
16. A categoria Terminal só é pré-marcada com sessão gráfica; agentes de IA
    não são itens na v1 (15.5).
