# TUI "papel e tinta" — plano de implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesenhar todas as telas da TUI do kanivet-cli no estilo "papel e tinta" (sempre creme) e adicionar as 5 funções novas aprovadas nos mockups.

**Architecture:** O `Theme` passa a carregar um `*lipgloss.Renderer` e um modo de pintura (`Paper`). Um arquivo de componentes (`ui.go`) desenha caixa com sombra, tag, botão, trilha e abas, e um `Canvas` garante que cada linha tem exatamente a largura do terminal com fundo creme. O `Model.View()` vira um quadro fixo (cabeçalho, corpo, rodapé) e cada tela devolve só o corpo, já composto com os componentes. A máquina de estados da Fase 2 continua igual, exceto pelas funções novas do resumo e do painel.

**Tech Stack:** Go 1.24+, bubbletea v1.3.10, bubbles v1.0.0, lipgloss v1.1.0, termenv (já é dependência indireta do lipgloss).

**Spec:** `docs/superpowers/specs/2026-10-30-kanivet-tui-papel-design.md` (mockups em `docs/superpowers/specs/2026-10-30-kanivet-tui-papel/`).

## Global Constraints

- Paleta exata da spec: `paper #f4efea`, `card #ffffff`, `ink #383838`, `ink-strong #111110`, `ink-muted #666666`, `sky #97d4ff`, `sun #ffde00`, `peach #ffdcbd`, `mint #b0fff4`, `coral-tint #ffcbc8`, `ok #068475`, `err #e23f35`, `log-bg #2b2a29`, `log-fg #e8e2dc`.
- Sempre creme: com cor (perfil true color ou 256) a tela inteira tem fundo `paper`; não há tema escuro.
- 16 cores ou `NO_COLOR`: nenhuma sequência de fundo, sem sombra, borda normal, seleção em negrito/inverso, tags entre colchetes.
- Mínimo 80×24 (`minWidth`, `minHeight` em `app.go`); nada desenha fora da largura; a sombra conta dentro dela.
- Sem novas dependências diretas além de `github.com/muesli/termenv` (já no `go.sum`).
- Todo texto novo entra em `internal/i18n/messages.go` em `En` e `PtBR` (os testes `TestAllUsedKeysExist` e `TestTMessagesParity` cobram); MAIÚSCULAS só na renderização (`strings.ToUpper`).
- Goldens continuam sem cor (renderer padrão dos testes, perfil Ascii) e são regenerados com `go test ./internal/tui/ -update`; revise o diff do golden antes de commitar.
- Marca KANIVET; nada de nome, logo ou laranja da MotherDuck.
- Commits sem linha `Co-Authored-By` (regra do dono do repo).
- Verificação de cada task: `go vet ./... && go test -race ./...`.

## Review Focus

1. **Buracos no fundo**: uma célula sem fundo `paper` (fim de linha curta, espaço entre caixas, reset de SGR no meio da linha, glifo largo como ícone Nerd) — esperado: toda linha de toda tela, em true color, tem largura exata `w` e fundo em todas as células. Coberto por `assertPainted` (Task 1) chamado em todas as telas (Tasks 2–9).
2. **Texto longo em pt-BR**: nomes de config, caminhos e traduções maiores que o inglês estourando tags e caixas — esperado: trunca com `…` dentro da caixa, nunca empurra a borda. Testes `*_PtBRLongFits` nas Tasks 4, 5 e 7.
3. **Exatamente 80×24**: com o respiro todo, a tela de itens precisa mostrar ≥ 6 linhas de lista e o cabeçalho/botão/rodapé nunca somem — `TestAllScreensAt80x24` (Task 9).
4. **`NO_COLOR` e 16 cores**: nenhuma sequência `48;` (fundo) em nenhuma tela — `TestPlainModeHasNoBackground` (Task 9).
5. **`r` sem nada para tentar**: run sem falhas, ou falha cujo item não é mais selecionável — esperado: `r` desabilitado no rodapé e sem efeito. `TestRetryDisabledWithoutFailures` (Task 7).

---

### Task 1: Paleta, tema com renderer e componentes base

**Files:**
- Modify: `internal/tui/theme.go`
- Create: `internal/tui/ui.go`, `internal/tui/ui_test.go`, `internal/tui/paint_test.go` (helper de teste)
- Modify: `internal/tui/theme_test.go`

**Interfaces:**
- Produces:
  - `func NewTheme(env map[string]string, nerd bool) Theme` (mantida; usa `lipgloss.DefaultRenderer()`)
  - `func NewThemeWith(r *lipgloss.Renderer, env map[string]string, nerd bool) Theme`
  - `Theme.Paper bool` — true quando `Color` e o perfil do renderer é `termenv.TrueColor` ou `termenv.ANSI256`
  - `Theme.R *lipgloss.Renderer`; `func (t Theme) S() lipgloss.Style` — estilo novo do renderer com fundo `paper` quando `Paper`
  - cores exportadas como constantes `colPaper … colLogFg` (`lipgloss.Color`)
  - `func (t Theme) Box(body string, width int, o BoxOpts) string` com `BoxOpts{Title string; Fill lipgloss.Color; Fg lipgloss.Color; NoShadow bool}`; `width` é a largura total ocupada, sombra incluída
  - `func (t Theme) Tag(text string, bg lipgloss.Color) string`
  - `func (t Theme) Button(key, label string) string` (ex.: `ENTER  INSTALAR →`)
  - `func (t Theme) Trail(step int, done bool) string` (5 passos; `step` 0–4; `done` = todos com ✓)
  - `func (t Theme) Tabs(names []string, active, width int) string`
  - `func (t Theme) Canvas(lines []string, w, h int) string` — completa/corta para `h` linhas de largura exata `w`, preenchendo com `paper`
  - `func (t Theme) Check(marked, radio bool) string` passa a devolver `✓`/`○` (e `●`/`○` com `radio`) com Nerd; ASCII como hoje
- Test helper: `func assertPainted(t *testing.T, view string, w, h int)` em `paint_test.go`: exatamente `h` linhas, cada uma com `lipgloss.Width == w`, e um parser de SGR mínimo que acompanha o fundo ativo (`48;2;…`, `48;5;…` ligam; `49`, `0` e `ESC[m` desligam) e falha em qualquer célula impressa sem fundo ativo
- Test helper: `func paperTheme() Theme` — `NewThemeWith` com renderer `lipgloss.NewRenderer(io.Discard)` e `SetColorProfile(termenv.TrueColor)`, `Nerd: true`

- [ ] **Step 1: Testes que falham** em `ui_test.go`:
  - `TestBoxFootprint`: `paperTheme().Box("a\nb", 20, BoxOpts{})` tem 2+2+1 = 5 linhas e todas com largura 20; última linha começa com `▝` seguido de `▀`; linhas 2..4 começam com `▐`.
  - `TestBoxNoShadowPlain`: com `NewTheme(map[string]string{"NO_COLOR": "1"}, false)` a caixa não tem `▐`/`▀` e tem 4 linhas.
  - `TestTagUpper`: `Tag("mise", colPeach)` contém `MISE`; no modo plain é `[MISE]`.
  - `TestCanvasPainted`: `assertPainted(t, th.Canvas([]string{"x", th.Tag("a", colSun)}, 80, 24), 80, 24)`.
  - `TestTrail`: `Trail(2,false)` contém `ITENS` e `✓` duas vezes; `Trail(4,true)` contém `✓` cinco vezes.
  - `TestThemePaperModes` em `theme_test.go`: TrueColor → `Paper`; ANSI256 → `Paper`; ANSI → `!Paper`; `NO_COLOR` → `!Paper && !Color`.
- [ ] **Step 2:** `go test ./internal/tui/ -run 'TestBox|TestTag|TestCanvas|TestTrail|TestThemePaper'` → FAIL (símbolos não definidos).
- [ ] **Step 3: Implementar.** Bordas: `lipgloss.ThickBorder()` com `Paper`, `NormalBorder()` sem. Sombra: a caixa é desenhada com largura `width-1` deslocada 1 coluna à direita; coluna 0 das linhas 1..h-1 é `▐` (fg `ink`, bg `paper`); a última linha é `▝` + `▀`×(width-2) + 1 espaço `paper`. Todo estilo sai de `t.S()` para herdar o fundo. Os estilos antigos (`Title`, `Subtle`, `Accent`, `Selected`, `Danger`, `Warn`, `OK`, `Border`, `Help`) continuam existindo, recalculados sobre a paleta nova, para as telas ainda não migradas compilarem.
- [ ] **Step 4:** mesmo comando → PASS; depois `go vet ./... && go test -race ./...`. Os goldens usam `nerd=false` (ASCII) e perfil sem cor, então **não** devem mudar nesta task; se mudarem, é bug.
- [ ] **Step 5: Commit** `feat(tui): paleta papel e tinta e componentes base`.

### Task 2: Quadro da tela (cabeçalho, rodapé, canvas)

**Files:**
- Modify: `internal/tui/app.go` (`View`, `title` → `header`, novo `footer`), `internal/tui/run.go` (tema com `lipgloss.DefaultRenderer()`, sem mudança de API)
- Modify: `internal/i18n/messages.go` (chaves `tui.trail.welcome|profile|items|review|install`, `tui.trail.end`)
- Test: `internal/tui/start_test.go`, goldens

**Interfaces:**
- Consumes: Task 1 (`Box`, `Trail`, `Button`, `Tag`, `Canvas`, `paperTheme`, `assertPainted`)
- Produces:
  - `func (m Model) header() []string` — caixa com `Tag("KANIVET", colSun)` + `Trail(...)` + `N/5` (ou `FIM`) à direita; no painel, perfil e contadores (detalhe na Task 8)
  - `func (m Model) footer() string` — `primary` à esquerda (de `m.primaryAction()`), contadores em tags, atalhos em `ink-muted` com a tecla em MAIÚSCULAS
  - `func (m Model) primaryAction() (key, label string, ok bool)` — por tela: welcome `ENTER/COMEÇAR`, review `ENTER/INSTALAR`, summary `ENTER/IR PARA O PAINEL`; demais `ok=false`
  - `func (m Model) bodyHeight() int` — `m.h` menos cabeçalho (4), título/respiro e rodapé, com a regra de corte da spec
  - `View()` sempre termina em `m.th.Canvas(lines, m.w, m.h)`; o aviso de tamanho mínimo também passa pelo canvas
- [ ] **Step 1: Testes que falham**: `TestViewPaintedEveryScreen` — para cada `Screen` com a sessão fake de `start_test.go`, `m.th = paperTheme()`, `m.w, m.h = 100, 30`, `assertPainted(t, m.View(), 100, 30)`; `TestHeaderShowsTrail` — no `ScreenCategories` o view contém `KANIVET`, `ITENS` e `3/5`; `TestTooSmallPainted` — `m.w=60` ainda passa em `assertPainted(..., 60, m.h)`.
- [ ] **Step 2:** rodar → FAIL.
- [ ] **Step 3:** implementar; as telas ainda devolvem as linhas antigas (o canvas pinta por baixo).
- [ ] **Step 4:** PASS; regenere goldens (mudam cabeçalho e rodapé) e revise o diff; suíte completa verde.
- [ ] **Step 5: Commit** `feat(tui): quadro com cabeçalho de marca, trilha e rodapé`.

### Task 3: Boas-vindas e perfil

**Files:**
- Modify: `internal/tui/welcome.go`, `internal/tui/profile.go`, `internal/i18n/messages.go`
- Test: `internal/tui/wizard_test.go`, goldens `welcome.golden`, `profile.golden`

**Interfaces:**
- Consumes: Tasks 1–2
- Produces: `func (m Model) sudoCount() int` — itens marcados em `m.sel` cujo método escolhido (`backend.Choose`) tem `NeedsSudo` (no item ou no método); `func profileColor(id string) lipgloss.Color` (laravel `sun`, web `peach`, ml `mint`, backend `coral-tint`, outros sem cor); `func (m Model) profileItemCount(id string) int` via `app.SelectedFor`
- Navegação do perfil: grade de 2 colunas com `↑↓←→`; `blank` fica fora da grade, numa linha abaixo, alcançada com `↓` da última linha
- [ ] **Step 1: Testes que falham**: `TestWelcomeHeadline` (contém `SEU TERMINAL DE DEV` em pt-BR com `i18n` forçado para pt-BR, e a tag `SUDO EM` só quando `sudoCount()>0`); `TestProfileGridNavigation` (a partir de laravel: `→` vai para web, `↓` vai para ml, `↓↓` chega em blank, `←` em blank não sai); `TestProfileCardCounts` (cada cartão mostra `N ITENS` igual a `len(app.SelectedFor(...))`); `assertPainted` nas duas telas.
- [ ] **Step 2:** FAIL.
- [ ] **Step 3:** implementar (título grande = duas linhas `t.S().Bold(true)` em MAIÚSCULAS com uma linha em branco antes/depois; caixa do sistema com `Box`; cartões = `Box` com `Fill: colSky` no selecionado). Abaixo de 80 colunas não acontece (mínimo), mas com `w ≥ 80` e altura < 30 os cartões perdem a linha de descrição 2.
- [ ] **Step 4:** PASS; goldens regenerados e revisados; suíte verde.
- [ ] **Step 5: Commit** `feat(tui): boas-vindas e perfis em cartões`.

### Task 4: Lista de itens (wizard)

**Files:**
- Modify: `internal/tui/itemlist.go`, `internal/tui/categories.go`, `internal/i18n/messages.go`
- Test: `internal/tui/itemlist_test.go`, goldens `itemlist.golden`, `categories.golden`

**Interfaces:**
- Consumes: Tasks 1–2
- Produces: `func (l ItemList) View(width, height int) string` mantém a assinatura; desenha duas caixas lado a lado (lista ≈ 56% da largura, detalhe o resto, gap de 2) com a linha do título da categoria acima (`TERMINAL E SHELL   CATEGORIA 1 DE 5 · 6 DE 7 MARCADOS`); `func methodTag(it *catalog.Item, p platform.Platform) string` (nome do backend escolhido) e `func (l ItemList) statusTag(id string) string` (`INSTALADO` mint, `NOVO` sun para ausente marcado, `SUDO` sun, `EXTERNO` peach, `INDISPONÍVEL` sem cor)
- Linha selecionada: fundo `sky`, texto `ink-strong`, nome em MAIÚSCULAS, de borda a borda da caixa
- [ ] **Step 1: Testes que falham**: `TestItemListTwoBoxesAt80` (em 80×18 a primeira linha da lista e a do detalhe estão na mesma linha da tela; nenhuma linha passa de 80); `TestItemListMinRows` (em 80×24 de tela cheia, ≥ 6 itens visíveis); `TestItemListPtBRLongFits` (pt-BR, item com descrição e config de 200 caracteres: nenhuma linha passa de 80 e a borda direita `┃` está na mesma coluna em todas as linhas da caixa); `assertPainted` com `paperTheme`. Os testes existentes de navegação, busca, painel de configs e versão continuam passando sem mudança de comportamento.
- [ ] **Step 2:** FAIL.
- [ ] **Step 3:** implementar; o corte em altura segue a ordem da spec (respiro → subtítulo → altura da lista com rolagem → linhas do detalhe).
- [ ] **Step 4:** PASS; goldens; suíte verde.
- [ ] **Step 5: Commit** `feat(tui): lista de itens em caixas com tags`.

### Task 5: Revisão

**Files:**
- Modify: `internal/tui/review.go`, `internal/i18n/messages.go`
- Test: `internal/tui/review_test.go`, golden `review.golden`

**Interfaces:**
- Consumes: Tasks 1–2; `app.Group` (grupos do `GroupPlan`)
- Produces: título `REVISE O PLANO` + tags de totais (`N INSTALAR` sky, `N CONFIGS` mint, `N SUDO` sun quando >0); uma caixa com os grupos, cada um aberto por tag (`PRECISA DE SUDO` sun, `INSTALAR` sky, `ATUALIZAR` sky, `REMOVER` coral-tint, `CONFIGURAR` mint); comando em `ink-muted`; `BACKUP` peach nos arquivos existentes; o modo de retomada mostra `RETOMAR EXECUÇÃO <id>` no título
- O diff (`d`) abre caixa centralizada com linhas `+` em fundo `mint` e `-` em `coral-tint` (sem cor: só os marcadores)
- [ ] **Step 1: Testes que falham**: `TestReviewGroupsTagged` (view contém as tags dos grupos presentes e não contém as dos ausentes); `TestReviewPtBRLongFits` (caminho de 150 caracteres: a tag `BACKUP` continua inteira na linha, o caminho é que trunca com `…`); `assertPainted` na revisão e no diff aberto. Testes existentes (cursor, `d`, retomada, request id) continuam passando.
- [ ] **Step 2:** FAIL. **Step 3:** implementar. **Step 4:** PASS; golden; suíte verde.
- [ ] **Step 5: Commit** `feat(tui): revisão em grupos com tags`.

### Task 6: Execução

**Files:**
- Modify: `internal/tui/execute.go`, `internal/i18n/messages.go`
- Test: `internal/tui/execute_test.go`

**Interfaces:**
- Consumes: Tasks 1–2; `executor.Event`
- Produces: `execState.began map[string]time.Time` e `execState.took map[string]time.Duration` — `onEvent` grava `began` no primeiro evento `state.StepRunning` do passo e `took` quando ele chega a `StepDone`/`StepFailed`/`StepSkipped`; `func fmtDur(d time.Duration) string` (`<10s` → `3.1s`; `<1h` → `1m12s`; senão `1h02m`); `m.exec.now func() time.Time` (padrão `time.Now`, substituível no teste)
- Tela: título `INSTALANDO` + barra (caixa de 1 linha preenchida em `sky` na proporção `finished/len(steps)`) + `feitos/total · tempo`; caixa de passos (✓ `ok` com duração, ✗ `err` com tag `FALHOU`, spinner nos que rodam, linha `· próximos · +N`); caixa de log com `Fill: colLogBg, Fg: colLogFg` mostrando o passo seguido e as últimas linhas
- [ ] **Step 1: Testes que falham**: `TestStepDurations` (eventos Running em t0 e Done em t0+3100ms com `now` fake → linha do passo contém `3.1s`); `TestFmtDur` (casos `3.1s`, `1m12s`, `1h02m`); `TestExecuteLogBoxDark` (com `paperTheme`, a caixa de log contém a sequência de fundo de `#2b2a29`); `assertPainted` com run em andamento. Testes existentes (follow, interrupção, interativos, ring buffer) continuam passando.
- [ ] **Step 2:** FAIL. **Step 3:** implementar. **Step 4:** PASS; suíte verde.
- [ ] **Step 5: Commit** `feat(tui): execução com progresso, durações e log`.

### Task 7: Resumo, `l` e `r`

**Files:**
- Modify: `internal/tui/summary.go`, `internal/tui/keys.go`, `internal/tui/review.go` (entrada da revisão com seleção explícita), `internal/i18n/messages.go`
- Test: `internal/tui/summary_test.go`, golden `summary.golden`

**Interfaces:**
- Consumes: Tasks 1–2, 5; `m.runRep.Failed` (step IDs), `m.exec.steps` (para achar o `ItemID` de cada passo), `m.sel` (perfil e versões atuais)
- Produces:
  - `KeyMap.Log` (`l`) e `KeyMap.Retry` (`r`) em `keys.go`
  - `func (m Model) retrySelection() (plan.Selection, bool)` — `Add` = itens dos passos que falharam e ainda são selecionáveis (`m.sel.Selectable`), `Profile` e `Versions` do `m.sel.ToPlan()` atual, sem `Remove`/`Uninstall`; `ok=false` quando a lista fica vazia
  - `func (m Model) beginReviewWith(from Screen, sel plan.Selection) (Model, tea.Cmd)`; `beginReview(from)` passa a chamar `beginReviewWith(from, m.sel.ToPlan())`
  - Confirmar a revisão vinda do `r` inicia um run novo: `startRun` zera `m.exec`, `m.runRep`, `m.sum`, e o resumo seguinte mostra só esse run
- Tela: título grande (`PRONTO EM 3M42S`, `TERMINOU COM FALHAS`, `INTERROMPIDO`), tags de totais, cartão `coral-tint` com as falhas (motivo + `L VER LOG` `R TENTAR DE NOVO`), caixa `PRÓXIMOS PASSOS` numerada, conflitos de config no mesmo estilo; `enter` num passo com falha continua abrindo o log (como hoje), `l` faz o mesmo
- [ ] **Step 1: Testes que falham**: `TestSummaryLogKey` (`l` numa falha abre `panelLog`); `TestRetryBuildsSelectionOfFailed` (run com 2 falhas e 3 sucessos → `retrySelection().Add` tem só os 2 itens, mesmo `Profile`); `TestRetryEntersReview` (`r` → `ScreenReview` com `review.caller == ScreenSummary`); `TestRetryDisabledWithoutFailures` (sem falhas: binding `Retry` desabilitado e `r` não muda a tela; falha de item agora `Unsupported`: idem); `TestSummaryPtBRLongFits` (motivo de falha de 200 caracteres cabe na caixa); `assertPainted`. Testes existentes (diff, `F`, confirmação) continuam passando.
- [ ] **Step 2:** FAIL. **Step 3:** implementar. **Step 4:** PASS; golden; suíte verde.
- [ ] **Step 5: Commit** `feat(tui): resumo com falhas em destaque, ver log e tentar de novo`.

### Task 8: Painel com abas

**Files:**
- Modify: `internal/tui/dashboard.go`, `internal/tui/keys.go`, `internal/i18n/messages.go`
- Test: `internal/tui/dashboard_test.go`, golden `dashboard.golden`

**Interfaces:**
- Consumes: Tasks 1–2, 4 (caixas da lista); `detect.ItemStatus.Version`/`Latest`; `m.s.St.Items[id].At` (`state.InstalledItem`)
- Produces:
  - `KeyMap.NextTab` (`tab`) e `KeyMap.PrevTab` (`shift+tab`). **Ruling:** a spec citava também `←`/`→`, mas no painel essas teclas já trocam a versão do item (`CycleVersion`); elas continuam com essa função. No painel, `tab` deixa de alternar o foco lista↔configs do `ItemList` (o painel de configs continua em `c`, e `esc` sai dele); no wizard, `tab` não muda
  - `r` continua sendo "recarregar" no painel (`keyRefresh`); `KeyMap.Retry` só vale no resumo
  - `dashState.focusList` deixa de existir; a lista sempre tem o foco; `dashState.cat` vira a aba ativa; `updateCatPane` é removido
  - Cabeçalho do painel: `KANIVET`, `Tag(perfil, colPeach)`, `N INSTALADOS` mint, `N ATUALIZAÇÕES` sun (quando >0), `N EXTERNOS` peach (quando >0)
  - Linha do item: versão instalada e tag de estado (`⇡ <latest>` sun, `EM DIA` mint, `EXTERNO` peach, `—` ausente); detalhe com `PELO KANIVET · dd/mm` (data local de `At`), método e configs
  - Abas: `Tabs(nomes das categorias, cat, largura)`; quando não cabem, rolam mantendo a ativa visível com `…` nas pontas
- [ ] **Step 1: Testes que falham**: `TestDashTabsSwitch` (`tab` avança a aba, `shift+tab` volta, a lista troca de categoria; na última aba `tab` volta à primeira); `TestDashTabsScrollKeepsActive` (com 14 categorias em 80 colunas, a aba ativa sempre aparece); `TestDashShowsVersionAndLatest` (status `Outdated` com `Version: "0.9.2"`, `Latest: "0.10.0"` → linha contém `0.9.2` e `⇡ 0.10.0`); `TestDashInstalledDate` (`At` em 29/10 → detalhe contém `29/10`); `assertPainted` com o seletor de perfil aberto e fechado. Os testes existentes de `a`, `u`, `U`, `p`, busca global e atualização continuam passando (adapte só os que dependiam do foco lateral, e registre isso no commit).
- [ ] **Step 2:** FAIL. **Step 3:** implementar. **Step 4:** PASS; golden; suíte verde.
- [ ] **Step 5: Commit** `feat(tui): painel com abas, versões e data de instalação`.

### Task 9: Ajuda, diálogos, modo sem cor e acabamento

**Files:**
- Modify: `internal/tui/app.go` (ajuda e confirmação de saída em caixa centralizada), `internal/tui/theme.go`, `README.md` (seção Interface: atalhos novos `l`, `r`, `tab`/`shift+tab`, e a observação de que a TUI pinta o fundo de creme)
- Test: `internal/tui/start_test.go` (ou `polish_test.go` novo)

**Interfaces:**
- Consumes: todas as anteriores
- Produces: `func (m Model) overlay(body string, title string) []string` — caixa centralizada sobre o corpo atual (cabeçalho e rodapé da tela continuam)
- [ ] **Step 1: Testes que falham**:
  - `TestAllScreensAt80x24`: para cada tela (com dados representativos, inclusive execução em andamento, resumo com falhas e painel com atualizações), `paperTheme`, 80×24: `assertPainted`; o view contém `KANIVET`; quando `primaryAction().ok`, contém o rótulo do botão; última linha não vazia (rodapé).
  - `TestAllScreensAt120x40`: idem em 120×40.
  - `TestPlainModeHasNoBackground`: com `NO_COLOR=1` e com renderer em `termenv.ANSI` (16 cores), nenhuma tela contém `\x1b[48;` nem `▐`/`▀`.
  - `TestHelpOverlayPainted`: `?` em cada tela → `assertPainted` e o título da ajuda visível.
- [ ] **Step 2:** FAIL (ajuda e confirmação ainda no formato antigo; possíveis buracos remanescentes).
- [ ] **Step 3:** implementar e corrigir o que os testes apontarem.
- [ ] **Step 4:** PASS; `go vet ./... && go test -race ./...` verde; `go build -o .build/k ./cmd/kanivet-cli`.
- [ ] **Step 5: Commit** `feat(tui): ajuda e diálogos em caixa, modo sem cor e README`.
- [ ] **Step 6: Teste manual (registre o resultado no PR)**: `.build/k` num kitty escuro e num claro (fundo creme em toda a tela, sem faixas), `TERM=xterm-256color .build/k` (legível), `NO_COLOR=1 .build/k`, redimensionar até 80×24 e abaixo.
