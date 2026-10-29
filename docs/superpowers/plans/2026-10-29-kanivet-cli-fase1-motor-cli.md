# kanivet-cli — Fase 1 (motor + catálogo + CLI): plano de implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar o binário `kanivet-cli` sem TUI. Ele precisa de um catálogo
declarativo embutido, detecção de estado, planner, executor retomável, configs
com backup, shell init gerado e a CLI `plan/install/status/update/uninstall/list/profiles/restore`.

**Architecture:** O motor fica em pacotes `internal/` independentes de terminal.
Os backends **não executam nada**: só traduzem um método de instalação do
catálogo em comandos (`runner.Cmd`). Assim o mesmo plano serve para
dry-run, para a revisão e para a execução. O executor roda os comandos por
um `runner.Runner` (real ou fake), registra tudo num journal e continua quando
um item falha. A CLI (cobra) é uma camada fina sobre o motor. A TUI da Fase 2
vai consumir as mesmas interfaces.

**Tech Stack:** Go 1.24+ (`go.mod` com `go 1.24`), `github.com/spf13/cobra v1.9.1`,
`gopkg.in/yaml.v3 v3.0.1`. O resto é biblioteca padrão (`embed`, `text/template`,
`os/exec`, `crypto/sha256`).

**Spec:** `docs/superpowers/specs/2026-10-29-kanivet-cli-design.md`. Leia as
seções 3, 4, 5, 9 e 15 antes de qualquer tarefa.

**Fora desta fase:** TUI (Fase 2), `catalog update` remoto, `self-update`,
GoReleaser/`install.sh`/tap (Fase 3). O `setup.py` continua no repo até a v1.

## Global Constraints

- O módulo Go é `github.com/eusener/kanivet-cli` e o binário é `kanivet-cli`.
  Nunca use `kanivet`.
- Plataformas: `darwin` e `linux`, em `amd64` e `arm64`. Em Windows, sai com
  erro "unsupported OS".
- Os textos de usuário passam por `i18n.T`. O inglês é o padrão e o pt-BR vem
  de `LANG`/`LC_ALL` começando com `pt`. Os campos de texto do catálogo são
  `i18n.Text` (string simples = en, ou um mapa `{en, pt-BR}`), e o lint exige
  `en`.
- Caminhos fixos:
  - estado: `~/.local/state/kanivet-cli/state.json`;
  - journal: `~/.local/state/kanivet-cli/runs/<YYYYMMDD-HHMMSS>/`;
  - backups: `~/.local/state/kanivet-cli/backups/<run>/<caminho absoluto sem a / inicial>`;
  - shell init: `~/.config/kanivet-cli/shell/init.zsh` e `init.bash`.
- Os marcadores do bloco no rc são `# >>> kanivet-cli >>>` e
  `# <<< kanivet-cli <<<`, com uma linha só dentro:
  `[ -f "$HOME/.config/kanivet-cli/shell/init.<shell>" ] && . "$HOME/.config/kanivet-cli/shell/init.<shell>"`.
- O rc é `~/.zshrc` se `$SHELL` termina em `zsh`, senão `~/.bashrc`.
- Seleção aditiva: `install` nunca gera passos de remoção, e `--remove X` só
  tira X da seleção.
- Sem TTY e sem `--yes`, `install`/`update`/`uninstall` saem com código 2 e a
  mensagem `i18n` "non-interactive: pass --yes".
- Uma falha num item não aborta o resto. Os passos que dependem do item que
  falhou ficam `skipped`. Se algum passo falhou, o código de saída é 1.
- Sudo: o executor chama `sudo -v` uma vez antes do primeiro passo com
  `NeedsSudo` e mantém a sessão com `sudo -n -v` a cada 60s. Os passos com
  sudo vêm primeiro na ordenação, desde que as dependências permitam.
- Linguagens via mise, com a versão em linha (`node@22`). `update` usa
  `mise upgrade <tool>`, que respeita a linha. O Rust usa `rustup`.
- PHP: o item `php` tem `provided_by: [lerd, flock]`. Sem eles, usa `brew`
  no macOS e o binário do static-php-cli no Linux.
- Na escolha automática do método de instalação, o brew vence no macOS e o
  mise no Linux. Sempre vence o primeiro método compatível, na ordem em que o
  item lista.

## Review Focus

1. **rc do shell é um symlink (dotfiles com stow/chezmoi):** a escrita segue o
   link e altera o arquivo real, sem trocar o link por um arquivo. O teste fica
   na Task 10.
2. **Interrupção no meio (Ctrl+C ou kill):** o journal fica com o passo em
   `running`. Na próxima vez, `install` oferece retomar, o passo é
   re-detectado e não é reexecutado se já estiver satisfeito. O teste fica na
   Task 12.
3. **Item pedido explicitamente que não suporta a plataforma** (por exemplo,
   `install flock` em Debian 12): o erro nomeia o item e o motivo, e os outros
   itens seguem. O teste fica na Task 9.
4. **Comando de detecção trava ou tem saída inesperada:** há timeout de 5s por
   comando de detecção, e o resultado é `StatusUnknown`, sem pânico. O teste
   fica na Task 7.
5. **Dois itens de categoria exclusiva pedidos juntos** (`--add lerd --add flock`):
   o planner retorna um erro que nomeia os dois itens e a categoria. O teste
   fica na Task 9.

---

## Estrutura de arquivos

```
cmd/kanivet-cli/main.go            # chama cli.Execute()
internal/platform/platform.go      # Detect(): SO, arch, distro, pkg manager, shell, gráfico, systemd
internal/i18n/i18n.go              # T(), Lang, Text
internal/i18n/messages.go          # mapas en / pt-BR
internal/version/version.go        # Compare, Extract
internal/catalog/types.go          # Catalog, Category, Item, Method, ...
internal/catalog/load.go           # Load(fs.FS)
internal/catalog/lint.go           # Lint(*Catalog) []error
internal/runner/runner.go          # Cmd, Result, Runner, ExecRunner
internal/runner/fake.go            # FakeRunner
internal/backend/backend.go        # interface + registro
internal/backend/{brew,mise,rustup,system,script,git,font,herdrplugin}.go
internal/detect/detect.go          # Detector, Status
internal/state/state.go            # State, Load/Save atômico
internal/state/journal.go          # Journal
internal/plan/plan.go              # Selection, Plan, Step, Build
internal/configs/configs.go        # render, backup, hash, rc block
internal/shellinit/shellinit.go    # Generate(items, shell)
internal/executor/executor.go      # Execute
internal/cli/*.go                  # comandos cobra
catalog/embed.go                   # package catalogdata; //go:embed
catalog/catalog.yaml, categories/, items/, profiles/, configs/
test/integration/                  # Dockerfiles + run.sh
.github/workflows/ci.yml
```

---

### Task 1: Esqueleto do módulo + detecção de plataforma

**Files:**
- Create: `go.mod`, `cmd/kanivet-cli/main.go`, `internal/platform/platform.go`
- Test: `internal/platform/platform_test.go`

**Interfaces:**
- Produces:
  ```go
  type OS string // "macos" | "linux"
  type Platform struct {
      OS OS; Arch string            // "amd64" | "arm64"
      Distro string                 // "ubuntu","debian","fedora","arch","opensuse-tumbleweed",... ; "" no macOS
      DistroVersion string          // "24.04"; no macOS, a versão do sistema ("15.1")
      PkgManager string             // "apt" | "dnf" | "pacman" | "zypper" | "" (macOS)
      Shell string                  // "zsh" | "bash"
      Graphical bool                // macOS sempre true; Linux: DISPLAY ou WAYLAND_DISPLAY
      Systemd bool                  // /run/systemd/system existe
      Home string
  }
  func Detect() (Platform, error)
  func detectFrom(env map[string]string, goos, goarch, osRelease string, fileExists func(string) bool, lookPath func(string) bool) (Platform, error)
  ```

- [ ] **Step 1: Write failing tests** em `platform_test.go` com
  `detectFrom`:
  - `TestDetectUbuntu`: osRelease `ID=ubuntu\nVERSION_ID="24.04"`, lookPath só
    acha `apt-get`, env `SHELL=/bin/bash`, sem DISPLAY. Esperado: `Distro="ubuntu"`,
    `DistroVersion="24.04"`, `PkgManager="apt"`, `Shell="bash"`, `Graphical=false`.
  - `TestDetectFedoraWayland`: `ID=fedora`, `dnf`, `WAYLAND_DISPLAY=wayland-0`.
    Esperado: `Graphical=true` e `PkgManager="dnf"`.
  - `TestDetectMacOS`: goos `darwin`. Esperado: `OS="macos"` e
    `Graphical=true`.
  - `TestDetectWindowsFails`: goos `windows`. Esperado: erro contendo
    `unsupported OS`.
  - `TestArchMapping`: `goarch="arm64"` → `"arm64"`; `"386"` → erro.
- [ ] **Step 2:** `go test ./internal/platform/` deve falhar (o pacote ainda
  não existe).
- [ ] **Step 3:** Crie o `go.mod` (`module github.com/eusener/kanivet-cli`,
  `go 1.24`) e implemente `detectFrom`/`Detect`. O `Detect` lê
  `/etc/os-release` e usa `exec.LookPath`. No macOS, a versão vem de
  `sw_vers -productVersion`. Faça o `main.go` com um `package main`
  provisório que imprime a plataforma (a Task 13 substitui).
- [ ] **Step 4:** `go test ./internal/platform/` passa e
  `go run ./cmd/kanivet-cli` imprime a plataforma real.
- [ ] **Step 5:** Commit `feat: esqueleto do módulo e detecção de plataforma`.

### Task 2: i18n e comparação de versões

**Files:**
- Create: `internal/i18n/i18n.go`, `internal/i18n/messages.go`, `internal/version/version.go`
- Test: `internal/i18n/i18n_test.go`, `internal/version/version_test.go`

**Interfaces:**
- Produces:
  ```go
  // i18n
  type Lang string // "en" | "pt-BR"
  func DetectLang(env map[string]string) Lang     // LC_ALL > LANG; prefixo "pt" → pt-BR
  func SetLang(l Lang); func Current() Lang
  func T(key string, args ...any) string          // fmt.Sprintf; chave ausente → a própria chave
  type Text map[Lang]string                       // UnmarshalYAML aceita string (vira en) ou mapa
  func (t Text) Get() string                      // idioma atual → en → ""
  // version
  func Extract(output, regex string) (string, bool) // primeiro grupo de captura
  func Compare(a, b string) int                     // numérico por partes; "1.10" > "1.9"; ignora sufixos "-rc1"
  ```

- [ ] **Step 1: Write failing tests:**
  - `TestDetectLang`: `{"LANG":"pt_BR.UTF-8"}` → `pt-BR`;
    `{"LC_ALL":"en_US.UTF-8","LANG":"pt_BR.UTF-8"}` → `en`; `{}` → `en`.
  - `TestTextYAML`: `"hello"` → `Text{"en":"hello"}`; um mapa
    `{en: hi, pt-BR: oi}` com o idioma em pt-BR faz `Get()` retornar `"oi"`; o
    mapa `{en: hi}` em pt-BR retorna `"hi"`.
  - `TestTMissingKey`: `T("nope")` → `"nope"`.
  - `TestTMessagesParity`: todas as chaves de `en` existem em `pt-BR` e
    vice-versa.
  - `TestCompare`: `("1.10.0","1.9.9")=1`, `("0.48","0.48.0")=0`, `("2.0.0-rc1","2.0.0")=0`,
    `("1.2","1.10")=-1`.
  - `TestExtract`: `("fzf 0.56.3 (brew)", "([\\d.]+)")` → `"0.56.3"`.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente. O `messages.go` começa com as chaves usadas nas
  Tasks 9, 12, 13 e 14, e cada task que cria mensagens adiciona as dela nos
  dois mapas.
- [ ] **Step 4:** `go test ./internal/i18n/ ./internal/version/` passa.
- [ ] **Step 5:** Commit `feat: i18n en/pt-BR e comparação de versões`.

### Task 3: Tipos e loader do catálogo

**Files:**
- Create: `internal/catalog/types.go`, `internal/catalog/load.go`, `catalog/embed.go`, `catalog/catalog.yaml`
- Test: `internal/catalog/load_test.go`, `internal/catalog/testdata/` (catálogo mínimo)

**Interfaces:**
- Consumes: `i18n.Text`, `platform.OS`
- Produces:
  ```go
  const SchemaVersion = 1
  type Catalog struct {
      Schema int; Version string
      Categories []Category          // na ordem de catalog.yaml
      Items map[string]*Item; Profiles []Profile
      Files fs.FS                     // raiz do catálogo, para os templates
  }
  type Category struct { ID string; Name, Description i18n.Text; Exclusive bool; Order int }
  type Item struct {
      ID, Category string; Name, Description, Notes i18n.Text; Homepage string
      Platforms []platform.OS
      Requires Requires              // {Systemd bool; Graphical bool; MinMacOS string; Distros []string; Arch []string}
      DependsOn []string; ProvidedBy []string; Provides []string
      MinVersion string; NeedsSudo bool; ZshOnly bool
      Detect Detect                  // {Command string; VersionRegex string}
      Install []Method; PostInstall []string
      Configs []ConfigSpec; Shell ShellSpec; Env map[string]string
  }
  type Method struct {
      Backend string; Package string; Version string     // Version = linha padrão ("22", "stable")
      When *When                                          // {OS platform.OS; PkgManager string}
      URL, SHA256 string; Args []string                   // backend script
      Repo, Ref, Dest string                               // backend git
  }
  type ConfigSpec struct { ID string; Description i18n.Text; Default bool; Files []ConfigFile }
  type ConfigFile struct { Template, Target string }
  type ShellSpec struct { Aliases map[string]string; Init map[string]string; InitOrder int; Env map[string]string }
  type Profile struct { ID string; Name, Description i18n.Text; Items []string; Exclusive map[string]string; Extras map[string][]string }
  func Load(fsys fs.FS) (*Catalog, error)
  func Embedded() (*Catalog, error)   // em internal/catalog, usa catalogdata.FS
  ```
  O `ShellSpec.Init` é indexado pelo shell (`"zsh"`, `"bash"`, `"any"`), e
  `"any"` aceita o placeholder `{{shell}}`. O `InitOrder` vai de 0 a 100 e
  ordena o init gerado. O padrão é 50; o syntax-highlighting usa 100 e o
  atuin 60 (depois do fzf, que usa 55).

- [ ] **Step 1: Write failing tests** com o catálogo em `testdata/` (2
  categorias, sendo uma exclusiva, 3 itens e 1 perfil):
  - `TestLoadOrdersCategories`: as categorias aparecem na ordem do
    `catalog.yaml`.
  - `TestLoadItemFields`: `Items["fzf"].Install[0].Backend == "brew"` e
    `Shell.InitOrder == 55`.
  - `TestLoadRejectsNewerSchema`: `schema: 2` → erro contendo `schema`.
  - `TestLoadDuplicateID`: dois arquivos com o mesmo `id` → erro com o id.
  - `TestEmbeddedLoads`: `Embedded()` não dá erro. O catálogo real ainda está
    vazio, com zero itens.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente o `Load`: lê `catalog.yaml`, `categories/*.yaml`,
  `items/*.yaml` e `profiles/*.yaml`, com um item por arquivo. O
  `catalog/embed.go` declara `package catalogdata` e
  `//go:embed catalog.yaml categories items profiles configs`. Crie diretórios
  vazios com `.gitkeep` e ajuste o embed para aceitar isso (`all:` prefix).
- [ ] **Step 4:** `go test ./internal/catalog/` passa.
- [ ] **Step 5:** Commit `feat: tipos e loader do catálogo declarativo`.

### Task 4: Lint do catálogo

**Files:**
- Create: `internal/catalog/lint.go`
- Test: `internal/catalog/lint_test.go`

**Interfaces:**
- Produces: `func Lint(c *Catalog, backends []string) []error`. Os erros são
  `LintError{Item, Field, Msg}`, e `Error()` gera `"<item>: <field>: <msg>"`.

- [ ] **Step 1: Write failing tests**, um caso por regra, com catálogos
  montados em código:
  - `Name`/`Description` sem `en` → erro `description: missing en`;
  - `Category` inexistente;
  - `DependsOn`/`ProvidedBy`/`Profile.Items` apontando para um id inexistente;
  - ciclo em `DependsOn` (a→b→a) → erro contendo `cycle`;
  - `Backend` que não está na lista;
  - `ConfigFile.Template` que não existe em `c.Files`;
  - `Platforms` vazio;
  - `Method.Backend == "script"` sem `URL` ou com URL que não começa com
    `https://`;
  - perfil com `Exclusive[cat]` cuja categoria não é exclusiva;
  - `TestLintValidCatalog`: o catálogo de `testdata` retorna zero erros.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente uma função por regra, juntando os resultados.
  A detecção de ciclo usa DFS com estados branco/cinza/preto.
- [ ] **Step 4:** `go test ./internal/catalog/` passa.
- [ ] **Step 5:** Commit `feat: lint do catálogo`.

### Task 5: Runner (exec real e fake)

**Files:**
- Create: `internal/runner/runner.go`, `internal/runner/fake.go`
- Test: `internal/runner/runner_test.go`

**Interfaces:**
- Produces:
  ```go
  type Cmd struct {
      Argv []string; Shell string      // Shell != "" → roda via "sh -c Shell" (instaladores curl|sh)
      Env []string; Dir string; Sudo bool
      Lock string                      // "brew","mise","apt","dnf","pacman","zypper","" — serializa no executor
      Display string                   // texto exibido no plano; padrão: Argv juntado ou Shell
  }
  type Result struct { ExitCode int; Output string; Err error }
  type Runner interface { Run(ctx context.Context, c Cmd, out io.Writer) Result }
  type ExecRunner struct{}
  type FakeRunner struct { mu sync.Mutex; Calls []Cmd; Responses map[string]Result } // chave = c.String()
  func (c Cmd) String() string
  ```
  No `ExecRunner`, com `Sudo` ligado o comando é prefixado com `sudo`. O
  stdout e o stderr vão juntos para `out` e para um buffer, que vira
  `Result.Output`. Ao cancelar o `ctx`, o grupo de processos inteiro é
  encerrado (`Setpgid` + kill `-pid`).
  No `FakeRunner`, um comando sem resposta registrada retorna `ExitCode 0`.

- [ ] **Step 1: Write failing tests:**
  - `TestExecEcho`: `Argv ["sh","-c","echo hi; echo err 1>&2"]` → `Output` contém
    `hi` e `err`, e `ExitCode == 0`.
  - `TestExecExitCode`: `sh -c "exit 3"` → `ExitCode == 3`.
  - `TestExecCancelKillsGroup`: `sh -c "sleep 30 & sleep 30"`, com o ctx
    cancelado após 100ms → retorna em menos de 2s.
  - `TestFakeRecords`: duas chamadas ficam registradas em ordem, e a resposta
    configurada é devolvida.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test ./internal/runner/` passa.
- [ ] **Step 5:** Commit `feat: runner de comandos real e fake`.

### Task 6: Backends (tradução método → comandos)

**Files:**
- Create: `internal/backend/backend.go` e um arquivo por backend: `brew.go`, `mise.go`, `rustup.go`, `system.go`, `script.go`, `git.go`, `font.go`, `herdrplugin.go`
- Test: `internal/backend/backend_test.go`

**Interfaces:**
- Consumes: `catalog.Method`, `platform.Platform`, `runner.Cmd`, `runner.Runner`
- Produces:
  ```go
  type Backend interface {
      Name() string
      Supports(m catalog.Method, p platform.Platform) bool
      Install(m catalog.Method, version string, p platform.Platform) []runner.Cmd   // version "" → m.Version
      Update(m catalog.Method, p platform.Platform) []runner.Cmd                    // nil = sem update
      Uninstall(m catalog.Method, p platform.Platform) []runner.Cmd
      Latest(ctx context.Context, r runner.Runner, ms []catalog.Method) map[string]string // pacote → versão; nil se não suporta
  }
  func Get(name string) (Backend, bool)
  func Names() []string
  func Choose(it *catalog.Item, p platform.Platform) (catalog.Method, Backend, error)  // primeiro compatível; erro i18n "no install method for <os>"
  ```
  Comandos que cada backend precisa gerar (são os valores verificados nos
  testes):
  | Backend | Install | Update | Uninstall | Lock |
  |---|---|---|---|---|
  | brew | `brew install <pkg>` (cask: `Package` prefixado `cask:` → `brew install --cask <pkg>`) | `brew upgrade <pkg>` | `brew uninstall <pkg>` | brew |
  | mise | `mise use -g <pkg>@<version>` (sem version: `<pkg>@latest`) | `mise upgrade <pkg>` | `mise uninstall --all <pkg>` + `mise unuse -g <pkg>` | mise |
  | rustup | `sh -c "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh -s -- -y --no-modify-path"` | `rustup update` | `rustup self uninstall -y` | — |
  | system | apt: `apt-get install -y <pkg>`; dnf: `dnf install -y`; pacman: `pacman -S --needed --noconfirm`; zypper: `zypper install -y`, todos com `Sudo:true`; `Package` aceita mapa por gerenciador no formato `apt=build-essential,dnf=@development-tools` | igual ao install | `remove`/`-R` equivalentes | o próprio gerenciador |
  | script | `sh -c "curl -fsSL <URL> -o $T && echo '<SHA256>  '$T \| shasum -a 256 -c && sh $T <Args>"` (sem SHA256: pula a checagem) | reexecuta o install | nil: o planner avisa "manual uninstall" | — |
  | git | `git clone --depth 1 --branch <Ref> <Repo> <Dest>` (se `Dest` existir: `git -C <Dest> fetch --depth 1 origin <Ref>` + `checkout FETCH_HEAD`) | o caminho do fetch | `rm -rf <Dest>` | — |
  | font | `curl -fsSL <URL> -o $T.zip`, `unzip -o -j $T.zip '*.ttf' -d <fontdir>`; no Linux, também `fc-cache -f <fontdir>` | igual | nil | — |
  | herdr-plugin | `herdr plugin install <Package> --yes --ref <Ref>` + `herdr plugin enable <id>` | `herdr plugin install <Package> --yes` | `herdr plugin uninstall <id>` | herdr |
  O `$T` é um arquivo temporário criado com `mktemp` dentro do mesmo
  `sh -c`. No `Dest` e na pasta de fontes, `~` é expandido para `p.Home`. A
  pasta de fontes é `~/Library/Fonts` no macOS e `~/.local/share/fonts` no
  Linux.
  `Latest`: o brew usa `brew outdated --json=v2` e o mise usa
  `mise outdated --json`. Os demais retornam nil.

- [ ] **Step 1: Write failing tests**, um por linha da tabela, comparando
  `Cmd.String()` e `Lock`/`Sudo`. Inclua também:
  - `TestChooseMacPrefersFirstCompatible`: item com `[brew, mise]` no macOS →
    brew.
  - `TestChooseLinuxSkipsBrewWhenOS`: método brew com `When{OS: macos}` → no
    Linux o escolhido é mise.
  - `TestChooseNone`: só `brew` com `When macos`, no Linux → erro.
  - `TestBrewLatestParses` / `TestMiseLatestParses`: JSON de exemplo via
    `FakeRunner` → mapa esperado.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente, com registro em `init()` de cada arquivo.
- [ ] **Step 4:** `go test ./internal/backend/` passa.
- [ ] **Step 5:** Commit `feat: backends brew, mise, rustup, system, script, git, font e herdr-plugin`.

### Task 7: Detector de estado

**Files:**
- Create: `internal/detect/detect.go`
- Test: `internal/detect/detect_test.go`

**Interfaces:**
- Consumes: `catalog`, `runner.Runner`, `version`, `state.State` (Task 8;
  aqui use uma interface mínima `Owned(id string) bool`)
- Produces:
  ```go
  type Status int
  const ( StatusAbsent Status = iota; StatusInstalled; StatusOutdated; StatusExternal; StatusExternalOutdated; StatusProvided; StatusUnsupported; StatusUnknown )
  type ItemStatus struct { ID string; Status Status; Version, Latest, Reason string }
  type Owner interface { Owned(id string) bool }
  type Detector struct { R runner.Runner; P platform.Platform; Owner Owner; Timeout time.Duration } // Timeout padrão 5s
  func (d *Detector) Detect(ctx context.Context, c *catalog.Catalog, selected map[string]bool) map[string]ItemStatus
  func (d *Detector) CheckUpdates(ctx context.Context, c *catalog.Catalog, st map[string]ItemStatus) // preenche Latest/Outdated via Backend.Latest
  ```
  Regras:
  - Item fora da plataforma ou sem atender a `Requires` → `StatusUnsupported`,
    com `Reason` legível (por exemplo, "requires systemd").
  - `ProvidedBy` contendo um item instalado ou selecionado → `StatusProvided`.
  - O comando de detecção roda via `sh -c`. Saída 0 significa presente.
  - Item presente que não é do kanivet-cli → `External`, ou
    `ExternalOutdated` se `Version < MinVersion`.
  - Timeout do comando de detecção → `StatusUnknown`.
  - Comando sai 0 mas o regex não casa → o item é tratado como presente
    (`Installed` ou `External`) com `Version == ""`. Nesse caso o
    `MinVersion` não é avaliado.
  - `Requires.Distros`: cada entrada é `<id>` ou `<id>>=<versão>` (por exemplo,
    `ubuntu>=24.04`), comparada com `Distro` e `DistroVersion` via
    `version.Compare`.
  - Os itens são detectados em paralelo, com no máximo 8 de cada vez.

- [ ] **Step 1: Write failing tests**, usando `FakeRunner` e um catálogo em
  código:
  - `TestAbsent`: o comando sai com 1 → `StatusAbsent`.
  - `TestInstalledOwned`: sai 0 com `"0.56.3"` e `Owned=true` → `Installed`,
    com `Version=="0.56.3"`.
  - `TestExternalTooOld`: `MinVersion 0.48` e saída `0.44.1` →
    `StatusExternalOutdated`.
  - `TestProvided`: `php` com `ProvidedBy [lerd]` e `selected["lerd"]=true` →
    `StatusProvided`.
  - `TestUnsupportedSystemd`: `Requires.Systemd` com `P.Systemd=false` →
    `StatusUnsupported`, com `Reason` contendo `systemd`.
  - `TestTimeout` (Review Focus 4): o runner fake bloqueia até o ctx cancelar e
    o `Timeout` é de 50ms → `StatusUnknown`, sem pânico, em menos de 1s.
  - `TestCheckUpdates`: `Installed 1.0`, com `Latest` do backend `1.1` →
    `StatusOutdated`.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test ./internal/detect/` passa.
- [ ] **Step 5:** Commit `feat: detector de estado dos itens`.

### Task 8: Estado persistente e journal

**Files:**
- Create: `internal/state/state.go`, `internal/state/journal.go`
- Test: `internal/state/state_test.go`, `internal/state/journal_test.go`

**Interfaces:**
- Produces:
  ```go
  type InstalledItem struct { Backend, Package, Version string; At time.Time }
  type AppliedConfig struct { Item, Target, Backup, SHA256 string; At time.Time }   // Backup "" = arquivo não existia
  type State struct {
      Version int; Profile string
      Items map[string]InstalledItem; Configs map[string]AppliedConfig  // chave Configs = Target absoluto
      Path string `json:"-"`
  }
  func DefaultDir(home string) string                 // ~/.local/state/kanivet-cli
  func Load(path string) (*State, error)              // ausente → State vazio, sem erro
  func (s *State) Save() error                        // escrita atômica: tmp + fsync + rename
  func (s *State) Owned(id string) bool
  type StepStatus string // "pending","running","done","failed","skipped"
  type Journal struct { Dir string; RunID string; Steps map[string]StepStatus }
  func NewJournal(stateDir string, now time.Time, planJSON []byte) (*Journal, error) // cria runs/<id>/plan.json
  func (j *Journal) Set(stepID string, s StepStatus) error   // persiste steps.json atomicamente
  func (j *Journal) LogWriter(itemID string) (io.WriteCloser, error) // runs/<id>/logs/<item>.log
  func LatestUnfinished(stateDir string) (*Journal, []byte, error)   // último run com algum pending/running; plan.json
  func (j *Journal) Finish() error                                   // marca o run como finalizado
  ```

- [ ] **Step 1: Write failing tests** (em `t.TempDir()`):
  - `TestLoadMissing`: estado vazio, sem erro.
  - `TestSaveLoadRoundTrip`.
  - `TestSaveAtomic`: depois de `Save`, não sobra `*.tmp` no diretório.
  - `TestJournalResume`: cria o journal e marca `a=done` e `b=running`; com
    isso, `LatestUnfinished` retorna esse run e o `plan.json` dele.
  - `TestJournalFinishedNotResumed`: depois de `Finish()`, `LatestUnfinished`
    retorna nil.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente com JSON indentado. O RunID usa o formato
  `20061029-150405`. Em caso de colisão, adicione o sufixo `-2`.
- [ ] **Step 4:** `go test ./internal/state/` passa.
- [ ] **Step 5:** Commit `feat: estado persistente e journal de execução`.

### Task 9: Planner

**Files:**
- Create: `internal/plan/plan.go`
- Test: `internal/plan/plan_test.go`

**Interfaces:**
- Consumes: `catalog`, `backend.Choose`, `detect.ItemStatus`, `platform.Platform`
- Produces:
  ```go
  type Selection struct {
      Profile string; Add, Remove, Uninstall []string
      Versions map[string]string          // item → linha ("node" → "24")
      Configs map[string]bool             // configID → ligada; ausente = ConfigSpec.Default
      Update []string; UpdateAll bool
      SwitchToZsh bool
  }
  type StepKind string // "install","update","uninstall","config","shell","post","chsh"
  type FileChange struct { Target, Template, ItemID string; Remove bool }
  type Step struct {
      ID, ItemID string; Kind StepKind; Cmds []runner.Cmd; Files []FileChange
      NeedsSudo bool; DependsOn []string; Note string
  }
  type Plan struct { Steps []Step; Skipped []Skip; Orphans []string; Warnings []string }
  type Skip struct { ItemID, Reason string }   // provided/external/unsupported
  type PlanError struct { Msgs []string }     // erros que bloqueiam o plano
  func Build(c *catalog.Catalog, p platform.Platform, st map[string]detect.ItemStatus, sel Selection) (*Plan, error)
  func (p *Plan) Order() [][]Step   // camadas topológicas; sudo primeiro dentro do que for possível
  func (p *Plan) MarshalJSON() ([]byte, error)
  ```
  Algoritmo:
  1. Monta o conjunto desejado: `Profile.Items` mais `Add`, menos `Remove`.
  2. Fecha o conjunto sob `DependsOn`. As dependências entram com
     `Note = T("dependency of", x)`.
  3. Aplica as exclusivas: dois itens da mesma categoria exclusiva no conjunto
     geram um `PlanError` com os dois ids e a categoria. Quando o conflito vem
     do perfil, o `Exclusive` do perfil vale como padrão e um `Add` explícito o
     substitui.
  4. Para cada item, de acordo com o status:
     - `Absent` → install;
     - `ExternalOutdated` → install, com warning;
     - `Installed`, `External` ou `Provided` → sem install;
     - `Unsupported` → `Skip`. Se o item foi pedido explicitamente em `Add`,
       entra também em `Warnings` (Review Focus 3).
     - Itens com `ZshOnly` e `p.Shell == "bash"`, sem `SwitchToZsh`, viram
       `Skip`.
  5. Configs ligadas geram um passo `config` que depende do install. Se algum
     item tiver `Shell`, entra um único passo `shell` que depende de todos os
     installs.
  6. `Uninstall` gera passos de uninstall. Dependências de itens desinstalados
     que não são usadas por mais nenhum item instalado vão para `Orphans`, só
     como sugestão.
  7. `Update`/`UpdateAll` geram passos `update` para itens `Installed` ou
     `Outdated` do kanivet-cli.
  8. `SwitchToZsh` gera um passo `chsh` (`chsh -s $(command -v zsh)`), que
     depende do install do zsh.

- [ ] **Step 1: Write failing tests:**
  - `TestProfileItemsInstalled`: o perfil com `[fzf, bat]`, ambos `Absent`,
    gera dois passos install.
  - `TestAdditiveNeverRemoves`: um item instalado fora da seleção não gera
    passo.
  - `TestRemoveOnlyDeselects`: `Remove ["bat"]` com `bat` instalado não gera
    nenhum passo para o bat.
  - `TestDependencyPulled`: `herdr-plugins` depende de `go` → o passo de
    `go` vem antes, com a Note.
  - `TestExclusiveConflict` (Review Focus 5): `Add [lerd, flock]` → `PlanError`
    com uma mensagem contendo `lerd`, `flock` e `php-local`.
  - `TestExclusiveProfileDefaultOverridden`: perfil com `php-local: flock` e
    `Add [lerd]` → instala lerd e não instala flock.
  - `TestProvidedSkipsPHP`: `php` com `StatusProvided` não gera install.
  - `TestUnsupportedExplicitWarns` (Review Focus 3): `Add [flock]` com status
    `Unsupported` → `Skip` e `Warnings` contendo `flock`; o resto do plano
    segue.
  - `TestZshOnlySkippedOnBash`.
  - `TestOrphanSuggested`: `Uninstall [node]` com `mise` sem outros dependentes
    → `Orphans == ["mise"]`, sem passo de uninstall do mise.
  - `TestOrderSudoFirst`: um passo `system` com sudo e sem dependências fica
    na primeira camada.
  - `TestVersionsOverride`: `Versions{"node":"24"}` → o comando contém
    `node@24`.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente seguindo o algoritmo acima. O ID dos passos
  segue o formato `<kind>:<item>` (`shell:all` para o passo de shell).
- [ ] **Step 4:** `go test ./internal/plan/` passa.
- [ ] **Step 5:** Commit `feat: planner com dependências, exclusividade e seleção aditiva`.

### Task 10: Configs (templates, backup, hash, bloco no rc)

**Files:**
- Create: `internal/configs/configs.go`
- Test: `internal/configs/configs_test.go`

**Interfaces:**
- Consumes: `state.State`, `catalog.Catalog.Files`, `platform.Platform`
- Produces:
  ```go
  type Vars struct { HOME, XDG_CONFIG, XDG_DATA, OS, Arch string }
  func VarsFor(p platform.Platform, env map[string]string) Vars
  func Render(fsys fs.FS, tmpl string, v Vars) ([]byte, error)
  type Applier struct { St *state.State; BackupDir string; Vars Vars; Files fs.FS }
  type Conflict struct { Target string; Diff string }   // arquivo mudou depois da aplicação
  func (a *Applier) Apply(fc plan.FileChange, force bool) (*Conflict, error)
  func (a *Applier) Remove(target string) error          // restaura backup ou apaga se não existia
  func EnsureRcBlock(rc, shell string) (changed bool, err error)
  func RestoreRun(backupRunDir string) ([]string, error) // restaura todos os arquivos do run; retorna os alvos
  ```
  O `Apply` funciona assim:
  1. Resolve o `Target`, expandindo `{{.XDG_CONFIG}}` e afins.
  2. Se já existe `St.Configs[target]` e o SHA atual do arquivo é diferente do
     SHA registrado, retorna `Conflict` (com diff unificado simples), a menos
     que `force` esteja ligado.
  3. Se o arquivo existe e ainda não tem backup neste run, copia para
     `BackupDir/<target sem a / inicial>`.
  4. Escreve com permissões preservadas, seguindo symlinks: abre com
     `os.OpenFile` no caminho resolvido por `filepath.EvalSymlinks`.
  5. Registra o SHA no estado.
  O `EnsureRcBlock` adiciona o bloco da seção Global Constraints no fim do rc,
  se ainda não existir, e também segue symlinks.

- [ ] **Step 1: Write failing tests** (em `t.TempDir()`, com `HOME` falso):
  - `TestRenderVars`: o template `{{.HOME}}/x` resulta no caminho esperado.
  - `TestApplyBacksUpExisting`: o arquivo existente vai para o backup e o
    conteúdo novo é escrito.
  - `TestApplyNewFileNoBackup`: `AppliedConfig.Backup == ""`.
  - `TestApplyConflictWhenUserEdited`: depois de aplicar, o arquivo é editado
    e reaplicado → `Conflict != nil` e o arquivo continua intacto; com
    `force=true`, ele é sobrescrito.
  - `TestRemoveRestoresBackup` e `TestRemoveDeletesWhenNoBackup`.
  - `TestEnsureRcBlockIdempotent`: chamado duas vezes → o bloco aparece uma
    vez só.
  - `TestRcSymlinkPreserved` (Review Focus 1): `~/.zshrc` é um symlink para
    `dotfiles/zshrc` → depois de `EnsureRcBlock`, o `~/.zshrc` continua
    symlink (`os.Lstat` com `ModeSymlink`) e o alvo contém o bloco. O mesmo
    vale para `Apply` com um target que é symlink.
  - `TestRestoreRun`.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente. O diff pode ser linha a linha simples (sem
  dependência externa).
- [ ] **Step 4:** `go test ./internal/configs/` passa.
- [ ] **Step 5:** Commit `feat: aplicação de configs com backup, hash e bloco no rc`.

### Task 11: Gerador do shell init

**Files:**
- Create: `internal/shellinit/shellinit.go`
- Test: `internal/shellinit/shellinit_test.go`, `internal/shellinit/testdata/*.golden`

**Interfaces:**
- Consumes: `catalog.Item`
- Produces: `func Generate(items []*catalog.Item, shell string, minVersions map[string]string) string`

  O texto gerado tem, nesta ordem:
  1. Um cabeçalho dizendo que o arquivo é gerado e que não deve ser editado.
  2. O bootstrap do Homebrew. É o mesmo bloco de `shell/cli-tools.sh`
     (linhas 15 a 24), adaptado.
  3. `export` dos `Env` de todos os itens, em ordem alfabética.
  4. Para cada item, ordenado por `(InitOrder, ID)`: o `Init[shell]` ou o
     `Init["any"]` (com `{{shell}}` substituído), seguido dos aliases. Tudo vai
     protegido por `if command -v <bin> >/dev/null 2>&1; then ... fi`, onde
     `<bin>` é a primeira palavra do `Detect.Command`.
  5. Itens com `ZshOnly` só entram quando `shell == "zsh"`.
  6. O init do starship fica protegido por `[ -z "$STARSHIP_SHELL" ]`. Isso não
     é um caso especial no código: a guarda vem escrita no `Init` do item
     starship.

- [ ] **Step 1: Write failing tests:**
  - `TestGoldenZsh` / `TestGoldenBash`: um conjunto fixo com fzf (55), atuin
    (60), zsh-syntax-highlighting (100, `ZshOnly`), eza (com aliases) e
    starship, comparado com os arquivos golden. Use `-update` para
    regenerá-los.
  - `TestOrderSyntaxHighlightingLast`: o índice do syntax-highlighting é maior
    que o de todos os outros.
  - `TestBashOmitsZshOnly`.
  - `TestGeneratedParses`: roda `zsh -n` e `bash -n` sobre a saída, com
    `t.Skip` se o shell não estiver instalado.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test ./internal/shellinit/` passa.
- [ ] **Step 5:** Commit `feat: gerador do shell init zsh/bash`.

### Task 12: Executor retomável

**Files:**
- Create: `internal/executor/executor.go`
- Test: `internal/executor/executor_test.go`

**Interfaces:**
- Consumes: `plan.Plan`, `runner.Runner`, `state.Journal`, `state.State`,
  `configs.Applier`, `shellinit.Generate`, `detect.Detector`
- Produces:
  ```go
  type Event struct { StepID, ItemID string; Status state.StepStatus; Line string; Err error } // Line = uma linha de output
  type Options struct { Concurrency int; DryRun bool; ForceConfigs bool; Events chan<- Event } // Concurrency padrão 4
  type Report struct { Done, Failed, Skipped []string; Conflicts []configs.Conflict }
  type Executor struct { R runner.Runner; J *state.Journal; St *state.State; Ap *configs.Applier; Cat *catalog.Catalog; P platform.Platform; Det *detect.Detector }
  func (e *Executor) Execute(ctx context.Context, pl *plan.Plan, o Options) (Report, error)
  ```
  Regras:
  - Executa as camadas de `pl.Order()`. Dentro de cada camada, roda até
    `Concurrency` passos em paralelo, com um `sync.Mutex` por `Cmd.Lock`.
  - Antes do primeiro passo com `NeedsSudo`, chama `sudo -v` (com stdin
    ligado ao terminal) e inicia o keep-alive. Se o `sudo -v` falhar, todos
    os passos com sudo e os que dependem deles ficam `skipped`.
  - Antes de cada passo `install`, re-detecta o item e pula se já estiver
    satisfeito, marcando `done` com a Note `already present`.
  - Passo que falha → `failed`. Os dependentes transitivos → `skipped`.
  - Install `done` → `St.Items[id]` é preenchido. Uninstall `done` →
    `delete(St.Items, id)`. O estado é salvo depois de cada passo.
  - O passo `shell` gera `init.zsh` e `init.bash` com todos os itens que têm
    `Shell`, instalados ou externos, e chama `configs.EnsureRcBlock`.
  - O passo `config` chama `Ap.Apply`. Um `Conflict` não é falha: vai para
    `Report.Conflicts` e o passo fica `skipped`.
  - Com `DryRun`, nada é executado e só são emitidos eventos com
    `Line = Cmd.Display`.
  - Com `ctx` cancelado, o passo em andamento fica `running` no journal e o
    executor retorna `ctx.Err()`.

- [ ] **Step 1: Write failing tests** (`FakeRunner`, diretórios temporários):
  - `TestFailureDoesNotAbortOthers`: os passos a e b são independentes e o a
    falha → b fica `done` e a fica `failed`.
  - `TestDependentSkipped`: b depende de a, que falha → b fica `skipped`.
  - `TestLockSerializes`: dois passos com `Lock "brew"` em paralelo → o
    `FakeRunner` com sleep nunca vê dois ao mesmo tempo (contador atômico de
    concorrência com máximo 1).
  - `TestSudoOnce`: três passos com sudo → `sudo -v` é chamado uma vez.
  - `TestResumeSkipsSatisfied` (Review Focus 2): o journal tem `a=running`, e a
    detecção agora diz que `a` está presente → a não executa nenhum comando e
    fica `done`.
  - `TestCancelLeavesRunning`: o ctx é cancelado durante o passo a → o journal
    fica com `a=running`.
  - `TestStateUpdated`: depois do install, `St.Owned("fzf")`; depois do
    uninstall, não mais.
  - `TestDryRunExecutesNothing`: `FakeRunner.Calls` fica vazio.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test -race ./internal/executor/` passa.
- [ ] **Step 5:** Commit `feat: executor paralelo, retomável e tolerante a falhas`.

### Task 13: CLI — `list`, `profiles`, `status`, `plan`, `install`

**Files:**
- Create: `internal/cli/root.go`, `internal/cli/list.go`, `internal/cli/status.go`, `internal/cli/install.go`, `internal/cli/render.go`
- Modify: `cmd/kanivet-cli/main.go` (passa a chamar `cli.Execute()`)
- Test: `internal/cli/cli_test.go`

**Interfaces:**
- Consumes: todos os pacotes acima.
- Produces:
  ```go
  type Env struct { R runner.Runner; P platform.Platform; Cat *catalog.Catalog; StateDir string; Stdout, Stderr io.Writer; Stdin io.Reader; IsTTY bool; Now func() time.Time }
  func NewRoot(e *Env) *cobra.Command
  func Execute() int   // monta o Env real e devolve o exit code
  ```
  Comandos e flags (os nomes são exatos):
  - `list [--category <id>] [--json]`
  - `profiles [--json]`
  - `status [--json] [--check-updates]`: a tabela tem as colunas `ITEM`,
    `CATEGORY`, `STATUS`, `VERSION`, `LATEST`. A saída `--json` é uma lista
    de `{id, category, status, version, latest, reason}`, com o status em
    string minúscula (`absent`, `installed`…).
  - `plan --profile <id> [--add x]... [--remove x]... [--version item=linha]... [--no-config <cfgid>]... [--zsh]`
  - `install [<item>...] (mesmas flags do plan) [--yes] [--dry-run] [--force-configs]`
  O root não tem `RunE` na Fase 1: sem subcomando, imprime a ajuda (a TUI
  entra na Fase 2).
  O `install` faz o seguinte, em ordem:
  1. Se existe um run inacabado, pergunta se quer retomar (com `--yes`,
     retoma) e executa o plano salvo.
  2. Sem run inacabado, roda o detect e monta o plano.
  3. Imprime o plano agrupado: sudo, instalar, configurar, pular, avisos,
     arquivos e backups.
  4. Pede confirmação, a menos que `--yes` esteja ligado.
  5. Executa, mostrando eventos como `✓/✗/↷ item`.
  6. Imprime o resumo: falhas, com o caminho do log e o comando para tentar de
     novo, e conflitos de config.
  Os ícones têm fallback ASCII (`[ok]`, `[x]`, `[skip]`) quando
  `NO_COLOR` está definido ou quando `!IsTTY`.

- [ ] **Step 1: Write failing tests** (com `Env` fake: `FakeRunner`, catálogo
  de testdata e `t.TempDir()`):
  - `TestListJSON`: o JSON contém os ids do catálogo.
  - `TestStatusJSONShape`: as chaves são exatamente as listadas acima.
  - `TestPlanPrintsGroups`: a saída contém as seções de instalar e de pular.
  - `TestInstallNonTTYNeedsYes`: `IsTTY=false`, sem `--yes` → exit 2 e stderr
    com a mensagem i18n.
  - `TestInstallYesRuns`: os comandos esperados aparecem no
    `FakeRunner.Calls` e `state.json` tem os itens.
  - `TestInstallDryRun`: nenhum comando no Fake e o plano impresso.
  - `TestInstallExitCodeOnFailure`: um passo falha → exit 1 e o resumo mostra
    o caminho do log.
  - `TestInstallResumePrompt`: com um run inacabado e `--yes`, retoma sem
    re-planejar (os passos `done` não são reexecutados).
  - `TestPtBRMessages`: com `LANG=pt_BR.UTF-8`, o erro de non-TTY sai em
    português.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente.
- [ ] **Step 4:** `go test ./internal/cli/` passa. Depois, um teste manual:
  `go run ./cmd/kanivet-cli install --profile laravel --dry-run` (após a Task
  15) imprime o plano real desta máquina.
- [ ] **Step 5:** Commit `feat: CLI list, profiles, status, plan e install`.

### Task 14: CLI — `update`, `uninstall`, `restore`, `catalog lint`, `doctor`

**Files:**
- Create: `internal/cli/update.go`, `internal/cli/uninstall.go`, `internal/cli/restore.go`, `internal/cli/catalogcmd.go`, `internal/cli/doctor.go`
- Test: `internal/cli/cli2_test.go`

**Interfaces:**
- Consumes: `cli.Env`, `plan.Selection{Update, UpdateAll, Uninstall}`, `configs.RestoreRun`, `catalog.Lint`
- Produces:
  - `update [<item>...] [--yes] [--dry-run]`: sem argumentos, atualiza todos
    os itens do kanivet-cli.
  - `uninstall <item>... [--yes] [--dry-run]`: imprime os órfãos sugeridos
    sem removê-los. Com `--with-orphans`, eles entram na remoção.
  - `restore [<run-id>]`: sem argumento, lista os runs que têm backups. Com
    argumento, chama `RestoreRun` e imprime os arquivos restaurados.
  - `catalog lint [--dir <path>]`: o padrão é o catálogo embutido. Imprime os
    erros e sai com 1 se houver algum.
  - `doctor`: mostra a plataforma, o rc alvo, se o bloco existe, se o
    `~/.local/bin` está no `PATH`, se brew e mise estão presentes, e o run
    inacabado, se houver.

- [ ] **Step 1: Write failing tests:**
  - `TestUpdateOnlyOwned`: itens externos não aparecem no plano de update.
  - `TestUpdateLanguageStaysInLine`: o update do node gera `mise upgrade node`,
    nunca `node@latest`.
  - `TestUninstallRemovesConfigsAndShell`: depois do uninstall do item com
    config, o arquivo é restaurado do backup e o init gerado não contém mais
    o alias do item.
  - `TestUninstallOrphansNotRemovedByDefault`.
  - `TestUninstallScriptItemWarnsManual`: item do backend `script` → o aviso
    "manual uninstall" e o estado não é removido.
  - `TestRestoreLists` e `TestRestoreRun`.
  - `TestCatalogLintEmbedded`: exit 0. `TestCatalogLintBrokenDir`: exit 1.
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Implemente. No uninstall, o passo `shell` é regenerado sem
  os itens removidos.
- [ ] **Step 4:** `go test ./internal/cli/` passa.
- [ ] **Step 5:** Commit `feat: CLI update, uninstall, restore, catalog lint e doctor`.

### Task 15: Catálogo — base (Terminal, Shell, Substitutos, Navegação, Git, Dados e APIs)

**Files:**
- Create: `catalog/categories/{terminal,shell,replacements,navigation,git,data-apis,managers}.yaml`, um arquivo `catalog/items/<id>.yaml` por item listado, `catalog/configs/...`
- Test: `internal/catalog/embedded_test.go`

Itens (ids exatos): `kitty`, `herdr`, `herdr-plugins`, `nerd-font-jetbrains`,
`herdr-icon-font`, `starship`, `zsh`, `zsh-autosuggestions`,
`zsh-syntax-highlighting`, `direnv`, `atuin`, `zoxide`, `fzf`, `eza`, `bat`,
`fd`, `ripgrep`, `dust`, `duf`, `btop`, `procs`, `sd`, `yazi`, `television`,
`lazygit`, `delta`, `gh`, `jj`, `jq`, `yq`, `xh`, `just`, `hyperfine`,
`watchexec`, `tldr`, `mise`.

Fontes da migração:
- Os aliases e o init vêm de `shell/cli-tools.sh`: as linhas de eza, bat e
  MANPAGER, btop, zoxide, fzf com fd/bat/eza, atuin com `--disable-up-arrow`,
  a função `y` do yazi, lg/lzd, direnv, mise e starship com a guarda
  `STARSHIP_SHELL`. Os valores de `InitOrder` são os da Task 3.
- O `herdr-plugins` usa os 4 plugins e commits de `configs/manifest.json`. Os
  `DependsOn` são `go` e `node`, que vêm da Task 16, por isso os itens `go` e
  `node` precisam existir antes do lint passar: crie stubs mínimos aqui e
  complete-os na Task 16. O pós-install `herdr integration install <agente>`
  só roda para os agentes presentes, conforme `AGENT_BINARIES` em
  `setup.py:45`. Isso é gerado como `PostInstall` com `command -v` em shell.
- A config opinativa do kitty e do herdr deriva de `configs/kitty/*.conf` e
  `configs/herdr/config.toml`, **neutralizadas**: sem os atalhos pessoais do
  herdr da tabela "Atalhos personalizados" do README, e com o `{{HOME}}`
  trocado por `{{.HOME}}`. Inclua os mapas `new_tab_with_cwd`.
- O `delta` tem um `PostInstall` que roda o `git config --global` com os
  quatro valores de `GIT_DELTA_CONFIG` (`setup.py`), só se `core.pager`
  estiver vazio.
- `kitty`, `herdr`, `nerd-font-jetbrains` e `herdr-icon-font` têm
  `Requires.Graphical: true`.
- O `zsh` usa o backend `system` (o próprio pacote `zsh`) no Linux e o `brew`
  no macOS.
- `zsh-autosuggestions` e `zsh-syntax-highlighting` têm `ZshOnly: true`. No
  Linux, o método é `git` em `~/.local/share/kanivet-cli/zsh/<nome>`, e o
  `Init` faz `source` desse caminho.
- Todo item de CLI segue o padrão `[brew (When macos), mise aqua:<owner>/<repo>]`.

- [ ] **Step 1: Write failing test** `TestEmbeddedCatalogLints`:
  `catalog.Lint(Embedded(), backend.Names())` não retorna erros, e cada id da
  lista acima existe.
- [ ] **Step 2:** O teste falha porque os itens estão ausentes.
- [ ] **Step 3:** Escreva os YAML com descrições `en` e `pt-BR`.
- [ ] **Step 4:** `go test ./internal/catalog/` e
  `go run ./cmd/kanivet-cli catalog lint` passam. O
  `go run ./cmd/kanivet-cli status` lista os itens com o estado real desta
  máquina.
- [ ] **Step 5:** Commit `feat(catalog): itens base de terminal, shell, cli e git`.

### Task 16: Catálogo — Linguagens, PHP local, Python/Data, Containers, Editores, Neovim e perfis

**Files:**
- Create: `catalog/categories/{languages,php-local,python-data,containers,editors,neovim}.yaml`, os itens, `catalog/profiles/{laravel,web,ml,backend,blank}.yaml`
- Test: `internal/catalog/embedded_test.go` (com casos novos)

Itens (ids exatos) e regras:
- **languages:**
  - `node` (mise `node`, linha `22`);
  - `pnpm` e `bun` (mise);
  - `python` (mise, `3.13`);
  - `go` (mise, `1.25`, `MinVersion 1.24`);
  - `rust` (rustup, linha `stable`);
  - `dotnet` (mise, `9`);
  - `java` (mise, `temurin-21`);
  - `php`: `ProvidedBy [lerd, flock]`; métodos brew `shivammathur/php/php@8.4`
    (macOS) e `script` com o static-php-cli em `~/.local/bin` (Linux); o
    `composer` fica como item separado, também com `ProvidedBy [lerd, flock]`.
- **php-local** (exclusiva):
  - `lerd`: `script https://lerd.sh/install.sh`; `Requires.Systemd` no Linux;
    `DependsOn [podman]` no Linux; `NeedsSudo` (DNS/certificados);
    `Provides [php, composer]`; Notes sobre as portas 80/443.
  - `flock`: `Requires.MinMacOS 15` com arch `arm64` no macOS e `Distros`
    `[ubuntu>=24.04, fedora]`; `Provides [php, composer]`. O método de
    instalação vem de `flockphp.com/download`: confira o comando atual nessa
    página na hora de implementar e registre a URL no item.
- **python-data:**
  - `uv` (brew/mise);
  - `ruff` (mise `aqua:astral-sh/ruff`);
  - `jupyter` (`uv tool install jupyterlab`, como `PostInstall` do item com
    `DependsOn [uv]` e o método `script` local, sem URL; para isso, adicione ao
    backend `script` um campo `Inline` que dispensa o `URL`, e o lint aceita
    `URL` **ou** `Inline`);
  - `cuda`: só Linux; `Requires` exige o `nvidia-smi` presente (estenda
    `Requires` com `Commands []string`); usa o backend `system`.
- **containers:**
  - `podman` e `docker` (exclusivos dentro de uma categoria
    `container-engine`);
  - `lazydocker`, `k9s`, `kubectl`.
- **editors:** `helix`, `zed` (`cask:zed` no macOS; `script` com
  `https://zed.dev/install.sh` no Linux, `Requires.Graphical`) e `vscode`
  (cask no macOS; no Linux, backend `system` com o repositório da Microsoft,
  documentado em Notes).
- **neovim** (exclusiva): `lazyvim`, `astronvim`, `nvchad`, `kickstart` e
  `neovim-plain`.
  - Todos têm `DependsOn [neovim-bin]`. O `neovim-bin` fica na categoria
    `editors`, mas não é selecionável sozinho no perfil.
  - Os starters usam o backend `git` com destino `~/.config/nvim`. Antes do
    clone, um passo `config` com `FileChange{Target: "~/.config/nvim", Dir: true}`
    move o diretório existente para o backup. Para isso, adicione o campo
    `Dir bool` em `plan.FileChange` e o método
    `configs.Applier.ApplyDir(fc plan.FileChange) error` (move para o backup,
    sem copiar). Essa extensão faz parte desta task, com o teste
    `TestApplyDirMovesToBackup` em `internal/configs`.
  - Os extras do LazyVim por perfil ficam em `Profile.Extras["lazyvim"]`. O
    item `lazyvim` tem uma config que renderiza
    `lua/plugins/kanivet-extras.lua` com os extras do perfil. Para isso, o
    `Vars` ganha um campo `Extras []string`.
- **Perfis:** exatamente os da seção 7 da spec, com `Exclusive`:
  - `laravel`: `php-local`=`lerd` no Linux e `flock` no macOS. O perfil
    aceita a chave `exclusive_by_os: {macos: {php-local: flock}, linux: {php-local: lerd}}`;
    adicione isso ao tipo `Profile` e ao planner, com o teste
    `TestProfileExclusiveByOS` em `internal/plan`.
  - `neovim`: `lazyvim` em todos os perfis.
  - `container-engine`: `podman` no perfil backend.
  - `blank`: só a base.
- **Base** (em todos os perfis): os itens de Shell, Substitutos, Git e LazyVim
  da Task 15. A categoria Terminal não entra direto: ela entra pelo
  `Requires.Graphical`, que faz esses itens virarem `Unsupported` com o motivo
  "no graphical session" quando não há sessão gráfica.

- [ ] **Step 1: Write failing tests:**
  - `TestEmbeddedCatalogLints` cobrindo os novos ids.
  - `TestLaravelProfileProvidesPHP`: faz o plano do perfil laravel numa
    plataforma Linux fake com systemd → não há passo `install:php` e há
    `install:lerd`.
  - `TestLaravelMacUsesFlock`.
  - `TestBackendProfileHasRust`: há o passo `install:rust`, com o comando do
    rustup.
  - Mais os testes de extensão citados acima (`TestApplyDirMovesToBackup` e
    `TestProfileExclusiveByOS`).
- [ ] **Step 2:** Os testes falham.
- [ ] **Step 3:** Escreva os YAML e as extensões (`Inline`,
  `Requires.Commands`, `ApplyDir`, `exclusive_by_os`, `Vars.Extras`).
- [ ] **Step 4:** `go test ./...` passa, e
  `go run ./cmd/kanivet-cli plan --profile laravel` mostra o plano coerente
  desta máquina (macOS: flock, sem php).
- [ ] **Step 5:** Commit `feat(catalog): linguagens, PHP local, data, containers, editores e perfis`.

### Task 17: Testes de integração em containers + CI

**Files:**
- Create: `test/integration/Dockerfile.{ubuntu,debian,fedora,arch,opensuse}`, `test/integration/run.sh`, `.github/workflows/ci.yml`
- Modify: `README.md` (adicionar uma seção "kanivet-cli (em desenvolvimento)", com build e uso da CLI)

- [ ] **Step 1:** Crie o `run.sh <distro> <profile>`. Ele faz o build do binário
  para `linux/$(uname -m)`, sobe o container com um usuário não-root `dev`
  com sudo sem senha e roda
  `kanivet-cli install --profile <p> --yes --no-config kitty-config`.
  Depois roda `kanivet-cli status --json` e falha se algum item selecionado,
  que não seja `unsupported`, ficar como `absent`. Também roda
  `zsh -n ~/.config/kanivet-cli/shell/init.zsh` e o equivalente em bash.
- [ ] **Step 2:** `./test/integration/run.sh ubuntu blank` passa localmente
  (precisa de Docker ou Podman).
- [ ] **Step 3:** No `ci.yml`, configure os jobs:
  - `go vet`, `go test -race ./...` e `kanivet-cli catalog lint` no
    ubuntu-latest e no macos-latest;
  - matriz de integração: distros × perfis `blank` e `web`, além de `laravel`
    só no Ubuntu (o lerd pulado por falta de systemd é o esperado:
    `unsupported`);
  - no macOS, `install --profile blank --yes` direto no runner.
- [ ] **Step 4:** O CI passa num push para um branch.
- [ ] **Step 5:** Commit `test: integração em containers e CI`.

---

## Self-review (feito)

- **Cobertura da spec na Fase 1:**
  - seções 3 a 5 → Tasks 1 a 6, 15 e 16;
  - seção 7 → Task 16;
  - seção 8.4 → Tasks 13 e 14;
  - seção 9 → Tasks 8, 10, 11 e 12;
  - seção 12 → Tasks 4, 11 e 17;
  - seção 13 (itens migrados) → Tasks 15 e 16;
  - seção 15 → Tasks 7, 9, 12, 14 e 16.
- **Fora da Fase 1, de propósito:** seções 8.1 a 8.3 (TUI), 10 (catálogo
  remoto) e 11 (distribuição), além de `self-update` e da aposentadoria do
  `setup.py`.
- **Tipos conferidos entre tasks:** `runner.Cmd.Lock`, `plan.FileChange`,
  `detect.ItemStatus`, `state.StepStatus`, `catalog.Method.Version` e
  `Selection.Versions`.
