# kanivet-cli — Fase 2 (TUI): plano de implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rodar `kanivet-cli` sem subcomando num terminal abre uma TUI de
qualidade profissional. Na primeira vez é um wizard (boas-vindas → perfil →
categorias → revisão → execução → resumo). Nas próximas é um dashboard de dois
painéis que mostra o estado, deixa adicionar, remover e atualizar itens, e
passa pelas mesmas telas de revisão e execução.

**Architecture:** O fluxo que hoje vive dentro de `internal/cli` (carregar
estado, detectar, planejar, executar, retomar) sai para um pacote
`internal/app`, usado igualmente pela CLI e pela TUI.

A TUI (`internal/tui`) é um modelo Bubble Tea com uma máquina de telas.
Toda decisão de seleção fica num modelo puro (`tui/selection.go`), testado sem
terminal. A execução roda o executor numa goroutine, e os eventos chegam à TUI
como `tea.Msg`. Comandos que leem o terminal (`sudo -v`, `chsh`) passam por um
hook do executor que a TUI atende com `tea.ExecProcess`, o que libera o
terminal enquanto o prompt roda.

**Tech Stack:**
- Go 1.24+.
- `github.com/charmbracelet/bubbletea v1.3.10`, `github.com/charmbracelet/bubbles v1.0.0` e `github.com/charmbracelet/lipgloss v1.1.0`.
- Dependências que já existem: cobra, yaml.v3.
- Sem `teatest`: os testes chamam `Update`/`View` dos modelos direto, com tamanho fixo e perfil de cor ASCII, o que é determinístico e dispensa uma dependência pseudo-versionada.

**Spec:** `docs/superpowers/specs/2026-10-29-kanivet-cli-design.md`, seções 8.1
a 8.3 (TUI), 9 (sudo, backups, conflitos) e 15. O plano da Fase 1
(`docs/superpowers/plans/2026-10-29-kanivet-cli-fase1-motor-cli.md`) define
os tipos que esta fase consome.

**Fora desta fase:** catálogo remoto assinado, `self-update`, GoReleaser,
`install.sh` e tap (Fase 3). Também ficam de fora os 13 achados menores da
revisão da Fase 1, que estão listados no PR #1. A exceção é o "Unknown descarta
item explícito sem aviso": a TUI mostra Unknown explicitamente (Task 6).

## Global Constraints

- O binário continua se chamando `kanivet-cli`. A TUI abre só quando **stdin e
  stdout são TTY** e não há subcomando. Sem TTY, `kanivet-cli` imprime a ajuda,
  como hoje.
- **Tamanho mínimo é 80×24.** Abaixo disso, a TUI mostra só a mensagem i18n
  `tui.too_small` com o tamanho atual e o mínimo. Ela não quebra, e volta
  sozinha quando o terminal é aumentado.
- **Todo texto passa por `i18n.T`** (inglês padrão, pt-BR por `LANG`). O teste
  `TestAllUsedKeysExist` já existente cobre as chaves novas.
- **Cores:** com `NO_COLOR` definido, nada de cor ANSI. Os estilos vêm de
  `lipgloss.AdaptiveColor`, que funciona em fundo claro e escuro.
- **Ícones Nerd Font** só quando a JetBrainsMono Nerd Font estiver instalada,
  ou seja, quando o item `nerd-font-jetbrains` está presente. Nos outros casos
  vale o fallback ASCII. Mapa fixo:

  | Status | Ícone Nerd (codepoint, Font Awesome do Nerd Fonts) | Ícone ASCII |
  |---|---|---|
  | instalado | U+F00C (check) | `[x]` |
  | desatualizado | U+F062 (arrow-up) | `[^]` |
  | externo | U+F08E (external-link) | `[e]` |
  | externo velho | U+F071 (exclamation-triangle) | `[!]` |
  | ausente | U+F10C (circle-o) | `[ ]` |
  | fornecido | U+F0C1 (link) | `[p]` |
  | sem suporte | U+F05E (ban) | `[-]` |
  | desconhecido | U+F128 (question) | `[?]` |

  Marcadores de seleção:
  - Nerd: caixa marcada U+F14A, caixa vazia U+F096, rádio ligado U+F192, rádio desligado U+F10C;
  - ASCII: `[x]`, `[ ]`, `(•)`, `( )`.

- **Teclado completo:** setas e `hjkl`, `space` marca, `enter` confirma ou
  avança, `esc` volta, `/` busca, `?` ajuda, `q` sai (com confirmação se houver
  mudanças pendentes) e `ctrl+c` sai ou interrompe. O rodapé sempre mostra os
  atalhos da tela atual (bubbles/help). O mouse é opcional
  (`tea.WithMouseCellMotion`), só para rolagem e clique em itens.
- **Seleção aditiva, como na CLI.** No dashboard, desmarcar um item instalado
  **pelo kanivet-cli** vira "a remover". Desmarcar um item externo não faz
  nada, e o detalhe explica isso com a mensagem `tui.external_untouched`.
- **Nada roda sem a tela de revisão.** A execução só começa com `enter` na
  revisão. A revisão mostra o mesmo conteúdo agrupado da CLI: sudo, instalar,
  atualizar, remover, configurar (com `(backup)` e o rc), pular, órfãos e
  avisos.
- **Logs na TUI:** cada passo guarda no máximo as últimas 500 linhas em
  memória. O log completo continua no arquivo do journal.
- **Ctrl+C durante a execução** cancela o contexto. O runner manda SIGTERM,
  espera 10s e só então SIGKILL (o comportamento da Fase 1). A TUI mostra
  "interrompido" e sai restaurando o terminal, e a mensagem final traz
  `kanivet-cli install --resume`.

## Review Focus

1. **Resize ou terminal pequeno no meio da execução:** sem pânico. Abaixo de
   80×24 aparece a tela `tui.too_small`, e o layout volta ao crescer. O teste
   fica na Task 9 (`TestExecutionSurvivesResize`).
2. **Prompt de senha (sudo/chsh) com a TUI dona do terminal:** o pedido sai
   pelo hook e vira `tea.ExecProcess`. A resposta volta para o executor e a TUI
   redesenha. O teste fica na Task 9 (`TestInteractiveRequestUsesExecProcess`).
3. **Ctrl+C na execução:** cancela, o journal fica em `running`, o modelo
   entra no estado `interrupted` e devolve `tea.Quit` depois que o executor
   retorna. O teste fica na Task 9 (`TestCtrlCDuringExecution`).
4. **Output enorme e rápido** (por exemplo, `brew` com milhares de linhas): o
   buffer é limitado a 500 linhas por passo e o `View` não fica lento. O teste
   fica na Task 9 (`TestLogBufferBounded`).
5. **No dashboard, desmarcar um item externo:** não gera uninstall e mostra o
   aviso. O teste fica na Task 4 (`TestDeselectExternalDoesNotUninstall`).

---

## Estrutura de arquivos

```
internal/app/session.go        # Session: estado, detecção, plano, execução, retomada (extraído da cli)
internal/app/groups.go         # GroupPlan: plano agrupado (usado pela CLI e pela TUI)
internal/app/updates.go        # cache de verificação de atualizações (6h)
internal/cli/*.go              # passa a usar internal/app (comportamento idêntico)
internal/executor/executor.go  # + Options.RunInteractive
internal/catalog/types.go      # + Method.Versions
internal/tui/
  app.go          # modelo raiz: máquina de telas, tamanho, too-small, sair
  theme.go        # estilos, ícones (nerd/ascii), NO_COLOR
  keys.go         # keymaps por tela + help
  selection.go    # modelo puro de seleção (perfil, exclusivas, configs, versões, dashboard)
  itemlist.go     # componente: lista de itens com checkbox/rádio + painel de detalhe + filtro
  welcome.go      # tela 1 do wizard
  profile.go      # tela 2 do wizard
  categories.go   # tela 3 do wizard (uma página por grupo de categorias)
  review.go       # revisão do plano (viewport + diff sob demanda)
  execute.go      # execução ao vivo (passos, log, progresso, hook interativo)
  summary.go      # resumo final
  dashboard.go    # dashboard de dois painéis
  testdata/*.golden
```

Os grupos de páginas da tela 3 são uma **decisão do Claude**:

| Página | Categorias |
|---|---|
| 1. Terminal e shell | terminal, shell |
| 2. Ferramentas | replacements, navigation, git, data-apis |
| 3. Editores | neovim, editors |
| 4. Linguagens | languages, php-local, python-data |
| 5. Containers | container-engine, containers |

`managers` não aparece como página: o mise e o homebrew entram como
dependência.

---

### Task 1: Extrair `internal/app` (Session) da CLI

**Files:**
- Create: `internal/app/session.go`, `internal/app/groups.go`
- Modify: `internal/cli/install.go`, `internal/cli/update.go`, `internal/cli/render.go`, `internal/cli/root.go`
- Test: `internal/app/session_test.go`, `internal/app/groups_test.go` (os testes atuais de `internal/cli` precisam continuar verdes sem alteração)

**Interfaces:**
- Consumes: `cli.Env` (os campos atuais), `plan`, `detect`, `executor`, `state`, `configs`
- Produces:
  ```go
  package app
  type Session struct {
      R runner.Runner; P platform.Platform; Cat *catalog.Catalog; StateDir string
      Getenv map[string]string; Now func() time.Time
      St *state.State // carregado por Open
  }
  func Open(s Session) (*Session, error)                                  // carrega o estado
  func (s *Session) Statuses(ctx context.Context, sel plan.Selection) map[string]detect.ItemStatus
  func (s *Session) Plan(ctx context.Context, sel plan.Selection) (*plan.Plan, map[string]detect.ItemStatus, error)
  func (s *Session) Unfinished() (*state.Journal, *plan.Plan, error)     // plano pendente (sem passos done) ou nil
  type RunOptions struct { ForceConfigs bool; Events chan<- executor.Event; RunInteractive func(context.Context, runner.Cmd) runner.Result; Profile string }
  func (s *Session) Run(ctx context.Context, pl *plan.Plan, j *state.Journal, o RunOptions) (executor.Report, *state.Journal, error) // j nil → cria journal novo
  func (s *Session) RcPath() string; func (s *Session) ShellDir() string; func (s *Session) ExpandTarget(string) string
  func (s *Session) ExtrasFor(profile string) []string
  func SelectedFor(c *catalog.Catalog, p platform.Platform, sel plan.Selection) map[string]bool

  type GroupKind string // "sudo","install","update","uninstall","config","skip","orphans","warnings"
  type Line struct { StepID, Text string; Cmds []string; Files []FileLine } // Cmds já compactados (1ª linha + " …")
  type FileLine struct { Path string; Remove, Exists, Rc bool }
  type Group struct { Kind GroupKind; Title string; Lines []Line }
  func (s *Session) GroupPlan(pl *plan.Plan) []Group
  ```
  As funções `selectedFor`, `extrasFor`, `expandTarget`, `rcPath`, `shellDir`,
  `pendingSteps`, `compact` e a lógica de grupos de `printPlan` **se mudam**
  para `app`. O `printPlan` da CLI passa a só formatar `[]Group`, sem lógica
  própria.

- [ ] **Step 1: Write failing tests** em `internal/app`, com o catálogo de
  `../catalog/testdata`, um `FakeRunner` como o do harness da CLI (a resposta
  de detecção é configurável) e `t.TempDir()` como HOME:
  - `TestSessionPlanAddsProfileItems`: `Plan(Selection{Profile:"laravel"})`
    contém `install:fzf` e `install:lerd`.
  - `TestSessionRunCreatesJournalAndState`: depois do `Run`, `St.Owned("fzf")`
    é verdadeiro e o journal está finalizado (`Unfinished()` retorna nil).
  - `TestSessionUnfinishedReturnsPendingOnly`: um journal com `a=done` e
    `b=pending` faz o plano devolvido ter só o `b`.
  - `TestGroupPlanMarksExistingAndRc`: um arquivo existente produz
    `FileLine.Exists=true`, e o passo de shell inclui um `FileLine{Rc:true}`
    com `RcPath()`.
  - `TestGroupPlanCompactsMultilineCmds`.
- [ ] **Step 2:** `go test ./internal/app/` falha (pacote inexistente).
- [ ] **Step 3:** Implemente o `app` movendo o código da CLI. Na CLI, troque as
  chamadas e transforme o `printPlan(w, groups)` num formatador que repete o
  texto atual exatamente.
- [ ] **Step 4:** `go test ./internal/app/ ./internal/cli/` passa, com os testes
  da CLI **inalterados**, e `go test -race ./...` fica verde.
- [ ] **Step 5:** Commit `refactor: extrai a sessão (internal/app) da CLI`.

### Task 2: Hook interativo no executor

**Files:**
- Modify: `internal/executor/executor.go`
- Test: `internal/executor/executor_test.go`

**Interfaces:**
- Produces: `Options.RunInteractive func(ctx context.Context, c runner.Cmd) runner.Result`.
  Quando o hook está definido, **todo `Cmd` com `Stdin: true`** passa por ele
  em vez de `R.Run`. Isso inclui o `sudo -v` de `startSudo` e os comandos de
  `chsh`. Sem hook, o comportamento da Fase 1 continua igual.

- [ ] **Step 1: Write failing tests:**
  - `TestInteractiveHookHandlesSudo`: um passo com `NeedsSudo` e o hook
    registrando chamadas → o hook recebe `sudo -v` uma vez e o `FakeRunner` não
    recebe `sudo -v`.
  - `TestInteractiveHookHandlesChsh`: o passo `chsh:zsh` passa pelo hook.
  - `TestInteractiveHookFailureSkipsSudoSteps`: o hook devolve `ExitCode 1` para
    o `sudo -v` → os passos com sudo ficam `skipped` (`note.sudo_denied`).
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente um helper `r.exec(ctx, c, out)` que escolhe entre
  o hook e `R.Run`, usado por `startSudo` e `runCmds`.
- [ ] **Step 4:** `go test -race ./internal/executor/` passa.
- [ ] **Step 5:** Commit `feat: hook para comandos interativos no executor`.

### Task 3: Versões selecionáveis no catálogo

**Files:**
- Modify: `internal/catalog/types.go`, `internal/catalog/lint.go`, `catalog/items/{node,python,go,java,dotnet}.yaml`
- Test: `internal/catalog/lint_test.go`, `internal/catalog/embedded_test.go`

**Interfaces:**
- Produces: `Method.Versions []string \`yaml:"versions"\``, que são as linhas
  oferecidas na TUI. A primeira é a padrão, e o lint exige
  `Version == Versions[0]` quando `Versions` não está vazio.
  Valores (**decisão do Claude**):

  | Item | Versões |
  |---|---|
  | node | `["22","24","20"]` |
  | python | `["3.13","3.12","3.11"]` |
  | go | `["1.25","1.24"]` |
  | java | `["temurin-21","temurin-25","temurin-17"]` |
  | dotnet | `["9","10","8"]` |

- [ ] **Step 1: Write failing tests:**
  - `TestLintVersionsDefaultFirst`: `Version "24"` com `Versions ["22","24"]`
    gera erro `versions`.
  - `TestEmbeddedLanguageVersions`: `Items["node"].Install[0].Versions[0] == "22"`
    e o mesmo vale para os outros quatro itens.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente o campo e a regra de lint, e atualize os YAML.
- [ ] **Step 4:** `go test ./internal/catalog/` e `go run ./cmd/kanivet-cli catalog lint` passam.
- [ ] **Step 5:** Commit `feat(catalog): linhas de versão selecionáveis por linguagem`.

### Task 4: Modelo puro de seleção

**Files:**
- Create: `internal/tui/selection.go`
- Test: `internal/tui/selection_test.go`

**Interfaces:**
- Consumes: `catalog`, `detect.ItemStatus`, `platform.Platform`, `plan.Selection`
- Produces:
  ```go
  type Mode int // ModeWizard | ModeDashboard
  type Selection struct { /* privados */ }
  func NewSelection(c *catalog.Catalog, p platform.Platform, st map[string]detect.ItemStatus, mode Mode) *Selection
  func (s *Selection) ApplyProfile(id string)            // marca os itens do perfil (aditivo) + exclusivas (exclusive e exclusive_by_os)
  func (s *Selection) Marked(id string) bool
  func (s *Selection) Toggle(id string) (changed bool, note string) // note: motivo de não mudar (i18n)
  func (s *Selection) Selectable(id string) (bool, string)          // falso para unsupported/provided, com motivo
  func (s *Selection) ConfigOn(cfgID string) bool; func (s *Selection) ToggleConfig(cfgID string)
  func (s *Selection) Version(itemID string) string; func (s *Selection) CycleVersion(itemID string, delta int)
  func (s *Selection) Pending() (install, update, remove int)
  func (s *Selection) MarkUpdate(id string) / ToggleUpdateAll()
  func (s *Selection) ToPlan() plan.Selection
  ```
  Regras:
  - **Categoria exclusiva:** marcar B desmarca A da mesma categoria (rádio).
    Marcar o item já marcado desmarca, ou seja, a escolha passa a ser
    "nenhum".
  - **Wizard:** `ToPlan()` = `Profile` + `Add` com todos os marcados. Itens já
    presentes viram no-op no planner, o que é o comportamento da Fase 1.
  - **Dashboard:** na abertura, os itens `Installed`/`Outdated`/`External` vêm
    marcados. Desmarcar um `Installed` ou `Outdated` entra em `Uninstall`.
    Desmarcar um `External` não muda nada, `Toggle` devolve
    `changed=false, note=T("tui.external_untouched")`, e o item continua
    marcado. Marcar um ausente entra em `Add`.
  - `Unsupported` e `Provided` não são selecionáveis, e o `Selectable` devolve
    o motivo do status.
  - **Versões:** `CycleVersion` percorre `Method.Versions` do método escolhido
    (`backend.Choose`). Quando a versão é diferente da padrão, entra em
    `plan.Selection.Versions`.
  - **Configs:** o padrão vem de `ConfigSpec.Default`. Uma config desligada
    entra em `plan.Selection.Configs[id]=false`.

- [ ] **Step 1: Write failing tests** (catálogo embutido, plataformas fake
  iguais às de `embedded_test.go`):
  - `TestApplyProfileMarksItemsAndExclusiveByOS`: laravel no Linux marca
    `lerd` e não marca `flock`; no macOS arm64 é o contrário.
  - `TestExclusiveIsRadio`: com `lerd` marcado, `Toggle("flock")` desmarca
    `lerd`, e um novo `Toggle("flock")` deixa os dois desmarcados.
  - `TestUnsupportedNotSelectable`: `kitty` sem sessão gráfica faz
    `Selectable` ser falso, e o motivo contém o texto de `reason.graphical`.
  - `TestWizardToPlan`: o perfil web com `bun` desmarcado e
    `node=24` gera `Profile "web"`, `Add` com os marcados, `Remove` contendo
    `bun` e `Versions{"node":"24"}`.
  - `TestDashboardUninstallOwned`: `bat` `Installed`, ao desmarcar, faz
    `ToPlan().Uninstall == ["bat"]`.
  - `TestDeselectExternalDoesNotUninstall` (Review Focus 5): `eza` `External`
    → `Toggle` devolve `changed=false` com a nota, e `Uninstall` fica vazio.
  - `TestPendingCounts`.
  - `TestConfigToggle`: desligar `kitty-config` resulta em
    `Configs{"kitty-config":false}`.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test ./internal/tui/` passa.
- [ ] **Step 5:** Commit `feat(tui): modelo de seleção`.

### Task 5: Tema, ícones e atalhos

**Files:**
- Create: `internal/tui/theme.go`, `internal/tui/keys.go`
- Test: `internal/tui/theme_test.go`

**Interfaces:**
- Produces:
  ```go
  type Theme struct { Title, Subtle, Accent, Selected, Danger, Warn, OK, Border, Help lipgloss.Style; Nerd, Color bool }
  func NewTheme(env map[string]string, nerd bool) Theme    // NO_COLOR → Color=false e estilos sem cor
  func (t Theme) StatusIcon(s detect.Status) string        // tabela da Global Constraints
  func (t Theme) Check(marked, radio bool) string          // marcadores da Global Constraints (nerd ou ASCII)
  func HasNerdFont(st map[string]detect.ItemStatus) bool   // nerd-font-jetbrains presente
  type KeyMap struct { Up, Down, Left, Right, Toggle, Enter, Back, Search, Help, Quit, Apply, Update, UpdateAll, Diff, Profile key.Binding }
  func DefaultKeys() KeyMap                                // ShortHelp/FullHelp por tela vêm das telas
  ```
  A paleta usa `lipgloss.AdaptiveColor` com o par `{Light, Dark}` **(decisão do
  Claude)**:

  | Papel | Light | Dark |
  |---|---|---|
  | Accent | `#5A4FCF` | `#A79BFF` |
  | OK | `#2E7D32` | `#7BD88F` |
  | Warn | `#B26A00` | `#FFC56B` |
  | Danger | `#C62828` | `#FF7B72` |
  | Subtle | `#6B6B76` | `#8B8B99` |
  | Border | `#D0D0DA` | `#3A3A48` |

- [ ] **Step 1: Write failing tests:**
  - `TestStatusIconsASCII` e `TestStatusIconsNerd`, um caso por status.
  - `TestNoColorRendersPlain`: com `NO_COLOR=1`,
    `Theme.Accent.Render("x") == "x"`.
  - `TestHasNerdFont`.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente o tema. Os testes usam `lipgloss.SetColorProfile(termenv.Ascii)`
  num `TestMain`. Se a API do lipgloss v1.1 for outra, use um
  `lipgloss.NewRenderer` com perfil ASCII injetado no `Theme` e registre a
  escolha.
- [ ] **Step 4:** `go test ./internal/tui/` passa.
- [ ] **Step 5:** Commit `feat(tui): tema adaptativo, ícones e atalhos`.

### Task 6: Componente de lista de itens com detalhe e filtro

**Files:**
- Create: `internal/tui/itemlist.go`
- Test: `internal/tui/itemlist_test.go`, `internal/tui/testdata/itemlist*.golden`

**Interfaces:**
- Consumes: `Selection`, `Theme`, `KeyMap`, `catalog`, `detect.ItemStatus`
- Produces:
  ```go
  type ItemList struct { /* privados */ }
  func NewItemList(sel *Selection, th Theme, keys KeyMap, categories []string) ItemList // itens das categorias, na ordem do catálogo
  func (l ItemList) Update(msg tea.Msg) (ItemList, tea.Cmd)
  func (l ItemList) View(width, height int) string   // lista em cima + painel de detalhe embaixo (~40% da altura)
  func (l ItemList) Current() string                  // id sob o cursor
  func (l ItemList) Filtering() bool
  ```
  - **Linha:** `<check> <nome>  <status icon> <versão>`. Itens de categoria
    exclusiva usam o rádio. Itens não selecionáveis ficam com o estilo
    `Subtle`, e a linha não aceita `space`.
  - **Painel de detalhe:**
    - descrição;
    - status (com o texto i18n do status e `Reason`);
    - método que será usado (`backend` + pacote) e `(sudo)` se o método
      precisar;
    - `Notes`;
    - versão, mostrada como `‹ 22 ›` quando há `Versions`, e trocada com
      `←/→` ou `h/l`;
    - configs opinativas, cada uma com toggle por `c` (próxima config) e
      `space` dentro do painel. **Decisão do Claude:** `tab` alterna o foco
      entre a lista e o painel.
  - **`StatusUnknown`** aparece como `[?] desconhecido`, com a nota
    `tui.unknown_hint` ("a detecção não respondeu; o item será tratado como
    presente").
  - **Busca:** `/` abre um `textinput`, que filtra por id, nome e descrição,
    sem diferenciar maiúsculas. `esc` limpa.
  - **Rolagem:** o cursor mantém a janela visível e não renderiza linhas fora
    da tela.

- [ ] **Step 1: Write failing tests** (tamanho 100×30, tema ASCII):
  - `TestItemListGolden`: categorias `shell` e `replacements`, com o cursor no
    segundo item.
  - `TestSpaceTogglesAndRadio`: na categoria `php-local`, `space` em `flock`
    deixa `lerd` desmarcado.
  - `TestUnsupportedLineIgnoresSpace`.
  - `TestVersionCycleInDetail`: com o cursor em `node`, `→` faz
    `Selection.Version("node") == "24"`.
  - `TestFilter`: `/` seguido de `rip` deixa só `ripgrep` visível.
  - `TestScrollKeepsCursorVisible`: com 40 itens em altura 10, depois de `G`
    (fim) o último item está no `View`.
  - `TestUnknownShownWithHint`.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente. A primeira geração dos goldens usa `-update`, e
  depois disso confira o golden a olho e commite junto.
- [ ] **Step 4:** `go test ./internal/tui/` passa.
- [ ] **Step 5:** Commit `feat(tui): lista de itens com detalhe, versões, configs e busca`.

### Task 7: Wizard (boas-vindas, perfil, categorias)

**Files:**
- Create: `internal/tui/welcome.go`, `internal/tui/profile.go`, `internal/tui/categories.go`, `internal/tui/app.go` (modelo raiz, só com o wizard nesta task)
- Test: `internal/tui/wizard_test.go`, goldens

**Interfaces:**
- Consumes: `app.Session`, `Selection`, `ItemList`, `Theme`, `KeyMap`
- Produces:
  ```go
  type Screen int // ScreenWelcome, ScreenProfile, ScreenCategories, ScreenReview, ScreenExecute, ScreenSummary, ScreenDashboard
  type Model struct { /* tela atual, tamanho, sessão, seleção, sub-modelos */ }
  func New(s *app.Session, env map[string]string, start Screen) Model
  func (m Model) Init() tea.Cmd     // dispara a detecção (detectMsg) — mostra spinner "detectando…"
  func (m Model) Update(tea.Msg) (tea.Model, tea.Cmd)
  func (m Model) View() string
  type detectMsg struct{ statuses map[string]detect.ItemStatus }
  ```
  - **Boas-vindas:** sistema (SO, arch, distro, versão), shell, pacote do
    sistema e avisos de plataforma (`sem systemd: lerd indisponível`,
    `sem sessão gráfica: terminal ficará de fora`), derivados dos `Unsupported`
    da detecção. `enter` avança.
  - **Perfil:** os 5 perfis com nome e descrição, e uma prévia dos itens do
    perfil sob o cursor. `enter` chama `Selection.ApplyProfile` e avança.
  - **Categorias:** as 5 páginas da tabela de grupos. O título traz
    `Passo 3/5 · <página> (n/5)`. `enter` vai para a próxima página e, na
    última, para a revisão. `esc` volta para a página anterior e, na primeira,
    para o perfil.
  - **Barra inferior fixa:** `N a instalar · M a configurar`, vindo de
    `Selection.Pending()`, e os atalhos.
  - Enquanto a detecção não termina, a tela mostra um spinner e a tecla `enter`
    fica desabilitada.

- [ ] **Step 1: Write failing tests:**
  - `TestWizardFlowToReview`: `Init` → `detectMsg` → `enter` (boas-vindas) →
    `enter` (perfil web) → 5× `enter` → a tela atual é `ScreenReview`.
  - `TestBackFromFirstCategoryGoesToProfile`.
  - `TestWelcomeWarnsNoSystemd`: numa plataforma Linux sem systemd, o `View`
    contém o aviso do lerd.
  - `TestProfilePreviewGolden`.
  - `TestEnterDisabledWhileDetecting`.
  - `TestTooSmall`: com `tea.WindowSizeMsg{Width:70,Height:20}`, o `View`
    contém o texto de `tui.too_small`. Com `{100,30}`, volta ao normal.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test ./internal/tui/` passa.
- [ ] **Step 5:** Commit `feat(tui): wizard de primeira execução`.

### Task 8: Tela de revisão

**Files:**
- Create: `internal/tui/review.go`
- Modify: `internal/tui/app.go`
- Test: `internal/tui/review_test.go`, goldens

**Interfaces:**
- Consumes: `app.Session.Plan`, `app.Session.GroupPlan`, `Selection.ToPlan`
- Produces: `type planMsg struct{ pl *plan.Plan; groups []app.Group; err error }`.
  - Ao entrar na revisão, um `tea.Cmd` chama `Session.Plan(Selection.ToPlan())`.
  - **Topo:** contadores (instalar, atualizar, remover, configurar, pular) e,
    se houver passos com sudo, a faixa `Warn` com o texto `tui.sudo_banner`.
  - **Corpo:** os `[]app.Group` num `viewport`, com rolagem por `j/k`, `PgUp/PgDn`
    e mouse. Cada arquivo traz `(backup)` quando `Exists`, e o rc vem marcado.
  - `d` sobre uma linha de config abre o diff entre o arquivo atual e o
    template renderizado, num painel com `esc` para fechar. Use o
    `configs.Render` e o `diff` da Fase 1; exporte `configs.Diff`, que hoje é
    privado.
  - `enter` inicia a execução e `esc` volta para a tela anterior (wizard ou
    dashboard).
  - **PlanError** (por exemplo, duas exclusivas no plano via CLI) aparece numa
    caixa `Danger`, sem `enter`.
  - **Plano vazio** mostra `plan.empty`, e o `enter` vira "voltar".

- [ ] **Step 1: Write failing tests:**
  - `TestReviewGolden`: perfil blank numa plataforma Linux fake, com tudo
    ausente.
  - `TestReviewShowsSudoBanner`.
  - `TestReviewDiffOpensAndCloses`: com o cursor na linha do `fzf-defaults` do
    catálogo de testdata, `d` mostra um diff contendo `+--height 40%`, e `esc`
    fecha.
  - `TestReviewEmptyPlan`.
  - `TestReviewEscReturnsToCaller`, um caso para o wizard e outro para o
    dashboard.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test ./internal/tui/` passa.
- [ ] **Step 5:** Commit `feat(tui): revisão do plano com diff sob demanda`.

### Task 9: Tela de execução ao vivo

**Files:**
- Create: `internal/tui/execute.go`
- Modify: `internal/tui/app.go`
- Test: `internal/tui/execute_test.go`

**Interfaces:**
- Consumes: `app.Session.Run`, `executor.Event`, `Options.RunInteractive` (Task 2)
- Produces:
  ```go
  type eventMsg executor.Event
  type runDoneMsg struct{ rep executor.Report; j *state.Journal; err error }
  type interactiveReq struct{ cmd runner.Cmd; reply chan runner.Result }
  func waitEvent(ch <-chan executor.Event) tea.Cmd          // lê um evento e re-agenda
  func (m Model) startRun() (Model, tea.Cmd)               // goroutine com Session.Run; canal de eventos com buffer 256
  ```
  - **Esquerda:** a lista de passos na ordem de `pl.Order()`. Cada passo mostra
    spinner (`bubbles/spinner`) enquanto roda e o ícone `✓/✗/↷` (ASCII
    `[ok]/[x]/[skip]`) quando termina. A nota fica em `Subtle`.
  - **Direita:** o log do passo selecionado, que por padrão segue o passo em
    execução. É um `viewport` com as últimas linhas, e o buffer é um ring de 500
    linhas por passo. `↑/↓` escolhe outro passo e `f` volta a seguir.
  - **Rodapé:** `progress` com `concluídos/total` e o tempo decorrido.
  - **Hook interativo:** o `RunInteractive` da sessão envia `interactiveReq`
    pelo `program.Send`. O `Model` está ligado ao `*tea.Program` por um setter
    em `New`, chamado pelo entrypoint da Task 12. O `Update` responde com
    `tea.ExecProcess(exec.Command(argv...), func(err error) tea.Msg {...})`, e
    o callback escreve o `runner.Result` em `reply`. Para obter o argv, exporte
    `runner.ExecArgv`, que hoje é privado.
  - **`ctrl+c`:** chama o `cancel` do contexto do run, muda para o estado
    `interrupted` e espera o `runDoneMsg`. Quando ele chega, devolve `tea.Quit`,
    e a mensagem final (impressa após sair) é `msg.interrupted` com
    `--resume`.
  - Ao receber `runDoneMsg` sem interrupção, vai para o resumo.

- [ ] **Step 1: Write failing tests** (sessão com `FakeRunner`, plano de 3
  passos, eventos injetados direto como `eventMsg` quando a goroutine não for
  necessária):
  - `TestExecutionMarksStepsFromEvents`: os eventos `running` e `done` fazem o
    `View` mostrar o ícone de ok no passo.
  - `TestLogFollowsRunningStep`.
  - `TestLogBufferBounded` (Review Focus 4): 5000 `eventMsg` com `Line` →
    o buffer do passo tem 500 linhas e `View()` retorna em menos de 50ms.
  - `TestInteractiveRequestUsesExecProcess` (Review Focus 2): um
    `interactiveReq` faz o `Update` devolver um `tea.Cmd` não nulo. Executar o
    callback com `err=nil` escreve `ExitCode 0` no `reply`.
  - `TestCtrlCDuringExecution` (Review Focus 3): `ctrl+c` chama o cancel (o
    teste injeta o cancel), o estado vai para `interrupted` e só depois do
    `runDoneMsg` o comando é `tea.Quit`.
  - `TestExecutionSurvivesResize` (Review Focus 1): no meio da execução, um
    `WindowSizeMsg` 70×20 mostra o too-small e um 120×40 volta à execução, sem
    pânico.
  - `TestRunDoneGoesToSummary`.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test -race ./internal/tui/` passa.
- [ ] **Step 5:** Commit `feat(tui): execução ao vivo com log, progresso e prompts interativos`.

### Task 10: Tela de resumo

**Files:**
- Create: `internal/tui/summary.go`
- Modify: `internal/tui/app.go`
- Test: `internal/tui/summary_test.go`, golden

**Interfaces:**
- Consumes: `executor.Report`, `state.Journal.LogPath`, `configs.Conflict`
- Produces:
  - **Topo:** `summary.counts`, que já existe.
  - **Falhas:** um item por falha, com o caminho do log. `enter` sobre a falha
    abre as últimas 200 linhas do arquivo de log num `viewport`.
  - **Conflitos de config:** caminho mais `d` para o diff. A ação
    `F` reaplica com força só aquela config, com confirmação `y/N` inline,
    chamando `Session.Run` com um plano de um passo `config:<id>` e
    `ForceConfigs:true`.
  - **Próximos passos:** `summary.new_terminal` se houve passo de shell, e as
    `Notes` dos itens instalados que têm notas (por exemplo, `lerd init` e
    `flk install`).
  - **Teclas:** `enter` (sem falha selecionada) vai para o dashboard e `q` sai.

- [ ] **Step 1: Write failing tests:**
  - `TestSummaryGolden`, com 1 falha e 1 conflito.
  - `TestSummaryOpensLog`.
  - `TestSummaryForceConflictAsksConfirmation`: `F` mostra a confirmação, `n`
    cancela e `y` produz um `tea.Cmd` que roda o plano de um passo, verificado
    pelo `FakeRunner`/estado.
  - `TestSummaryShowsItemNotes`.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test ./internal/tui/` passa.
- [ ] **Step 5:** Commit `feat(tui): resumo com logs, conflitos e próximos passos`.

### Task 11: Dashboard

**Files:**
- Create: `internal/tui/dashboard.go`, `internal/app/updates.go`
- Modify: `internal/tui/app.go`
- Test: `internal/tui/dashboard_test.go`, `internal/app/updates_test.go`, golden

**Interfaces:**
- Consumes: `Selection` (`ModeDashboard`), `ItemList`, `app.Session`, `detect.Detector.CheckUpdates`
- Produces:
  ```go
  // app
  func (s *Session) CheckUpdates(ctx context.Context, st map[string]detect.ItemStatus, maxAge time.Duration) // usa ~/.cache/kanivet-cli/updates.json (TTL)
  type updatesMsg struct{ statuses map[string]detect.ItemStatus }
  ```
  - **Layout:**
    - esquerda (cerca de 26 colunas): as categorias na ordem do catálogo, com
      o contador `instalados/total`;
    - direita: o `ItemList` só da categoria selecionada;
    - `tab`, `←` e `→` (com o foco na lista de categorias) trocam o painel em
      foco.
  - **Topo:** `Perfil: <referência>` e a idade da verificação de atualizações
    (`verificado há 2h`, ou um spinner enquanto verifica).
  - **Barra inferior:** `N a instalar · M a atualizar · K a remover`, e `a`
    abre a revisão (Task 8) com `Selection.ToPlan()`.
  - **Ações:**
    - `space` marca ou desmarca, com as regras da Task 4;
    - `u` marca a atualização do item, `U` marca todos os desatualizados;
    - `p` abre o seletor de perfil como modal: escolher aplica o perfil de
      forma aditiva e atualiza `St.Profile` ao executar;
    - `/` faz a busca global, que filtra todas as categorias;
    - `r` re-detecta.
  - **Atualizações:** ao abrir, a verificação roda em background usando o
    cache se ele tiver menos de 6h. Com a variável
    `KANIVET_NO_UPDATE_CHECK=1`, não verifica. É a forma de "desligar" da spec
    seção 10, adiantada aqui.
  - Depois de uma execução, o dashboard re-detecta e mostra o novo estado.

- [ ] **Step 1: Write failing tests:**
  - `TestDashboardGolden` (100×30): mistura de status instalado, externo,
    ausente e sem suporte.
  - `TestDashboardPendingBar`: desmarcar `bat` (owned) e marcar `jq` mostra
    `1 a instalar · 0 a atualizar · 1 a remover`.
  - `TestDashboardUpdateAllMarksOutdated`.
  - `TestDashboardApplyOpensReview`.
  - `TestDashboardProfileModalIsAdditive`: aplicar `web` não desmarca nada que
    já estava marcado.
  - `TestUpdatesCacheTTL` (em app): um cache com 1h não chama o runner; um com
    7h chama; com `KANIVET_NO_UPDATE_CHECK=1`, nunca chama.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test ./internal/tui/ ./internal/app/` passa.
- [ ] **Step 5:** Commit `feat(tui): dashboard com estado, atualizações e perfis`.

### Task 12: Entrypoint, retomada na TUI e documentação

**Files:**
- Modify: `internal/cli/root.go` (o root passa a ter `RunE`), `internal/tui/app.go`, `README.md`
- Create: `internal/tui/run.go`
- Test: `internal/cli/cli_test.go` (casos novos), `internal/tui/start_test.go`

**Interfaces:**
- Produces:
  ```go
  // tui
  func StartScreen(st *state.State) Screen                       // sem itens nem perfil no estado → ScreenWelcome; senão → ScreenDashboard
  func Run(s *app.Session, env map[string]string) error          // cria o programa (AltScreen, mouse), liga o hook interativo, roda; imprime a mensagem final após sair
  ```
  - O `RunE` do root: com `IsTTY`, chama `tui.Run`; sem TTY, `cmd.Help()`, com
    exit 0, que é o comportamento atual.
  - **Retomada:** se `Session.Unfinished()` retorna um plano, a TUI abre direto
    numa revisão com o título `tui.resume_title` mostrando só os passos
    pendentes. `enter` retoma com o journal existente e `esc` descarta,
    abrindo o dashboard ou o wizard. Descartar **não apaga** o journal: um
    novo run o substitui, como na CLI.
  - **README:** a seção `kanivet-cli` ganha um parágrafo "Interface" com os
    atalhos e um checklist de **teste manual num terminal real**:
    1. wizard completo no perfil blank;
    2. senha do sudo pedida e aceita no meio da execução;
    3. `ctrl+c` durante um `brew install` e depois `kanivet-cli` oferecendo a
       retomada;
    4. resize abaixo de 80×24;
    5. `NO_COLOR=1 kanivet-cli`.

- [ ] **Step 1: Write failing tests:**
  - `TestStartScreenFirstRun` e `TestStartScreenDashboard`.
  - `TestRootWithoutTTYPrintsHelp` (cli): `IsTTY=false` faz `Run(env, nil)`
    terminar com exit 0 e a saída conter `Usage`.
  - `TestResumeOpensPendingReview` (tui): uma sessão com journal inacabado gera
    um modelo inicial na revisão com o título de retomada e só os passos
    pendentes.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente e escreva a seção do README.
- [ ] **Step 4:** `go test -race ./...` passa. Depois, um teste manual
  rápido: `go run ./cmd/kanivet-cli` num terminal real abre o dashboard desta
  máquina (que já tem itens) e `q` sai restaurando o terminal. Registre o
  resultado no ledger. Se o ambiente de execução não tiver TTY, registre
  "teste manual pendente para o usuário".
- [ ] **Step 5:** Commit `feat(tui): entrypoint, retomada e documentação`.

---

## Self-review (feito)

**Cobertura da spec:**

| Seção da spec | Tasks |
|---|---|
| 8.1 (wizard: boas-vindas, perfil, categorias, revisão, execução, resumo) | 7, 8, 9, 10 |
| 8.2 (dashboard: dois painéis, status, barra de pendências, atualizar, desinstalar, trocar perfil, reaplicar configs, `/`, `?`) | 11; reaplicar config com força na 10 |
| 8.3 (tema adaptativo, NO_COLOR, ícones com fallback, 80×24, teclado, mouse) | 5, 6, 7 |
| 9 (sudo uma vez, conflitos com diff, revisão de todo arquivo) | 2, 8, 10 |
| 10 (checagem leve de atualização com opção de desligar) | 11 |
| 15.1 (versão na linha) | 3, 4 |
| 15.2 (Unknown/externo) | 4, 6 |

**Fica para a Fase 3, de propósito:** "ver o script antes de executar"
(seção 9). Na Fase 2, a revisão mostra o comando, mas não baixa o script
remoto; isso entra junto com o catálogo assinado.

**Tipos conferidos entre tasks:** `app.Session`, `app.Group`, `app.FileLine`,
`RunOptions.RunInteractive`, `Selection.ToPlan`, `ItemList.Current`, `Screen`,
`eventMsg`, `interactiveReq`, `runDoneMsg` e `updatesMsg`.

**Exports novos exigidos:** `configs.Diff` (Task 8) e `runner.ExecArgv`
(Task 9), ambos citados nas tasks que os usam.
