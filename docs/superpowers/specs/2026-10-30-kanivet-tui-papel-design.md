# kanivet-cli — redesenho visual da TUI ("papel e tinta")

Data: 2026-10-30 · Status: aprovado nos mockups, aguardando revisão do documento

## Intenção

A TUI da Fase 2 funciona, mas tem cara de rascunho: texto corrido, pouca
hierarquia, quase nenhuma estrutura visual. O dono do projeto quer uma interface
**bonita e polida**, à altura de uma ferramenta open source pública, em **todas as
telas**.

A direção visual escolhida é a linha "papel e tinta" de uma referência que ele
trouxe (um design system feito a partir do site da MotherDuck): fundo creme,
tudo contornado com traço grosso grafite, sombras sólidas deslocadas, cores de
giz de cera chapadas nos destaques e títulos em maiúsculas monoespaçadas. Por cima
dessa linha ficam as escolhas feitas nas rodadas anteriores: a estrutura do
estilo "vibrante" (marca no topo, trilha/progresso, badges coloridos, títulos de
seção destacados), o **respiro** do estilo minimalista e **✓ / ○** na seleção.

Os mockups aprovados ("perfeito") estão em
`docs/superpowers/specs/2026-10-30-kanivet-tui-papel/papel-e-tinta.html` (itens e execução) e
`papel-todas-telas.html` (boas-vindas, perfil, revisão, resumo, painel). Eles são
a referência visual deste documento; onde o texto e o mockup divergirem, vale o
texto.

## Objetivos

- Todas as telas (boas-vindas, perfil, itens por categoria, revisão, execução,
  resumo, painel, ajuda, diálogos de confirmação/diff) redesenhadas no estilo
  papel e tinta, com a mesma linguagem visual.
- **Sempre creme**: a TUI pinta a tela inteira com o fundo creme, mesmo em
  terminal escuro (decisão do dono, opção A). Não há variante escura.
- Hierarquia clara em cada tela: cabeçalho de marca + trilha, título da tela,
  conteúdo em caixas, ação principal, rodapé de atalhos.
- Continua cabendo em **80×24** sem quebrar nem cortar o que importa.

## Não objetivos

- Mudar o motor, a CLI, o catálogo ou o fluxo de telas (a máquina de estados da
  Fase 2 fica como está). As poucas funções novas estão listadas em
  "Comportamentos novos".
- Tema configurável pelo usuário, tema escuro, temas alternativos.
- Animações além do spinner e da barra de progresso.

## Anti-objetivos

- **Parecer a MotherDuck.** Só a linha visual vem da referência: nada de pato,
  nome, logo, slogans ou o laranja do logo. A marca é KANIVET.
- Tela "manchada": qualquer célula que mostre o fundo do terminal no meio do
  creme (buracos atrás de espaços, fim de linha, entre caixas) é defeito.
- Gradientes, emoji como ícone, sombras "borradas" (o estilo é chapado).
- Perder legibilidade: texto principal precisa de contraste AA sobre o creme.

## Paleta (valores exatos)

| Token | Hex | Uso |
|---|---|---|
| `paper` | `#f4efea` | fundo de toda a tela |
| `card` | `#ffffff` | interior das caixas |
| `ink` | `#383838` | texto, bordas e sombras |
| `ink-strong` | `#111110` | texto sobre cores chapadas (tags, linha selecionada, cartão selecionado) |
| `ink-muted` | `#666666` | texto secundário, rótulos, atalhos (5:1 sobre o creme) |
| `sky` | `#97d4ff` | linha selecionada, passo atual da trilha, ação principal, cartão selecionado |
| `sun` | `#ffde00` | tag da marca, sudo, "novo", aba ativa do painel, atualização disponível |
| `peach` | `#ffdcbd` | tags de método (mise, script, apt, plugin), backup, externo |
| `mint` | `#b0fff4` | instalado, em dia, configs |
| `coral-tint` | `#ffcbc8` | falha (tag e cartão de falhas) |
| `ok` | `#068475` | ✓ de passo concluído |
| `err` | `#e23f35` | ✗ de passo com falha |
| `log-bg` / `log-fg` | `#2b2a29` / `#e8e2dc` | caixa de log na execução (o único bloco escuro) |

Cores por perfil (tag de contagem no cartão): laravel `sun`, web `peach`, ml
`mint`, backend `coral-tint`, blank sem tag.

## Linguagem visual

- **Caixa**: borda grossa (`┏━┓┃┗┛`) em `ink`, interior `card`, padding de 1
  coluna, e **sombra sólida** deslocada para baixo-esquerda com caracteres de
  bloco em `ink` sobre o creme (coluna de meio bloco à esquerda, linha `▀`
  embaixo). A sombra ocupa 1 coluna e 1 linha a mais; o layout reserva esse espaço.
- **Tag**: texto em MAIÚSCULAS, `ink-strong` sobre cor chapada, 1 espaço de cada
  lado. Status, método, contagens e marca são tags.
- **Títulos** de tela em MAIÚSCULAS com espaçamento normal (o terminal não tem
  tracking); títulos grandes (boas-vindas, resumo) também em MAIÚSCULAS, com uma
  linha em branco antes e depois.
- **Linha selecionada**: a linha inteira, de borda a borda da caixa, em `sky`
  com texto `ink-strong` e o nome em MAIÚSCULAS. Itens não selecionados ficam em
  caixa normal.
- **Seleção**: `✓` marcado, `○` desmarcado (fallback ASCII `[x]`/`[ ]`).
- **Cabeçalho** (todas as telas): uma caixa no topo com a tag `KANIVET` (`sun`),
  a trilha `BOAS-VINDAS ─ PERFIL ─ ITENS ─ REVISÃO ─ INSTALAR`, com o passo atual
  numa tag `sky`, os concluídos com `✓` e os demais em `ink-muted`, e `N/5` à
  direita. No painel, a trilha dá lugar a perfil + contadores (ver abaixo).
- **Ação principal**: uma "tecla-botão" em `sky` (`ENTER  INSTALAR →`) à esquerda
  do rodapé, seguida dos outros atalhos em `ink-muted` com a tecla em MAIÚSCULAS.
  Em telas sem ação principal clara, só o rodapé.
- **Respiro**: linhas em branco entre cabeçalho, título, conteúdo e rodapé, e
  margem lateral de 2 colunas. Em 80×24 o respiro encolhe primeiro (ver Layout).

## Telas

**1 · Boas-vindas.** Título grande em duas linhas ("SEU TERMINAL DE DEV, /
MONTADO EM MINUTOS."), uma frase e a garantia "Nada é executado antes da
revisão." em `ink-muted`; caixa com sistema, shell e gerenciador de pacotes, e
uma tag `sun` "SUDO EM N ITENS" quando a seleção padrão tiver itens com sudo;
botão `ENTER  COMEÇAR →`.

**2 · Perfil.** Cartões em grade de 2 colunas (laravel, web, ml, backend), cada
um com nome, descrição curta em até 2 linhas e uma tag de contagem de itens na
cor do perfil. O cartão sob o cursor fica todo `sky` com `●`; os outros brancos
com `○`. "Começar do zero" fica numa linha só abaixo da grade. Setas nas quatro
direções movem; `enter` escolhe.

**3 · Itens por categoria.** Título da categoria + "CATEGORIA N DE 5 · X DE Y
MARCADOS". Duas caixas lado a lado: a lista (nome + tags de método e status) e o
detalhe (nome, homepage, descrição, status, configs com ✓/○). Contadores em tags
no rodapé (`36 INSTALAR`, `6 CONFIGS`). Busca e painel de configs (`c`) como hoje.

**4 · Revisão.** Título + tags de totais (instalar, configs, sudo). Uma caixa com
os grupos do plano, cada um aberto por uma tag: `PRECISA DE SUDO` (`sun`),
`INSTALAR` (`sky`), `ATUALIZAR`, `REMOVER`, `CONFIGURAR` (`mint`); cada linha com
o comando em `ink-muted`; arquivos que já existem levam a tag `BACKUP` (`peach`).
Botão `ENTER  INSTALAR →`, `d` diff.

**5 · Execução.** Título + barra de progresso (caixa de 1 linha preenchida em
`sky`), `feitos/total` e tempo decorrido. Caixa de passos: `✓` (cor `ok`) com
duração, `✗` (cor `err`) com tag `FALHOU`, spinner nos que estão rodando, e uma
linha "· próximos · +N" para a fila. O passo sob o cursor em `sky`. Abaixo, a
caixa de log escura (`log-bg`) com o nome do passo e as últimas linhas.

**6 · Resumo.** Título grande com o resultado ("PRONTO EM 3M42S", ou equivalente
para falha/interrupção), tags de totais (instalados, configs, falhou). Se houve
falhas: cartão `coral-tint` com cada falha e o motivo, e os atalhos `l` ver log e
`r` tentar de novo. Caixa "PRÓXIMOS PASSOS" numerada. Botão `ENTER  IR PARA O
PAINEL →`. Os conflitos de config (diff + `F`) da Fase 2 continuam, no mesmo
estilo.

**7 · Painel.** Cabeçalho com `KANIVET`, perfil numa tag, e tags de
`N INSTALADOS` / `N ATUALIZAÇÕES` (e externos, se houver). Categorias como
**abas encaixadas** (aba ativa `sun`). Lista com versão instalada e tag de estado
(`⇡ versão nova` em `sun`, `EM DIA` em `mint`, `EXTERNO` em `peach`, `—` para não
instalado); detalhe com "pelo kanivet · data", método e configs. Rodapé com os
atalhos atuais do painel.

**Ajuda (`?`), diálogos e diff**: caixa centralizada com sombra, mesmo cabeçalho
de tags; o diff usa `mint`/`coral-tint` como fundo das linhas +/−.

## Comportamentos novos

Os mockups aprovados trazem algumas coisas que a TUI ainda não faz. O dono
decidiu que **todas entram neste mesmo trabalho**, junto com o visual:

1. **Resumo**: `l` abre o log completo do passo sob o cursor; `r` monta e executa
   um novo plano só com os itens que falharam (mesmo caminho da revisão).
2. **Painel**: categorias em abas; `tab`/`shift+tab` trocam de aba (`←`/`→` continuam trocando a versão do item).
   O foco lateral de categorias da Fase 2 deixa de existir.
3. **Execução**: tempo decorrido total e duração de cada passo concluído.
4. **Painel**: data de instalação (vem de `state.InstalledItem.At`) e versão
   disponível (`detect.ItemStatus.Latest`).
5. **Boas-vindas**: contagem de itens com sudo na seleção padrão.

## Layout e 80×24

- A tela inteira é desenhada numa grade do tamanho exato do terminal, e toda
  célula recebe fundo `paper` (ou o fundo do elemento por cima dela).
- Ordem de corte quando falta altura: 1) linhas em branco extras do respiro;
  2) a linha de subtítulo/contador ao lado do título; 3) altura da lista (rola);
  4) linhas do detalhe/log. Cabeçalho, título, linha selecionada, botão e rodapé
  nunca somem.
- Abaixo de 80 colunas: o detalhe da tela de itens vai para baixo da lista e o
  perfil passa a 1 coluna de cartões; abaixo de 80×24 vale o aviso de tamanho
  mínimo da Fase 2, também pintado de creme.

## Terminais sem cor verdadeira

- **True color**: paleta exata.
- **256 cores**: a mesma paleta, aproximada pela degradação do termenv/lipgloss;
  o resultado precisa continuar legível (verificar à mão em `TERM=xterm-256color`).
- **16 cores ou `NO_COLOR`**: não pinta fundo; volta ao fundo do terminal, sem
  sombra, com bordas normais, seleção em negrito/inverso e tags entre colchetes.
  O comportamento atual de `NO_COLOR` é a base.
- Sem Nerd Font: os mesmos fallbacks ASCII de hoje.

## Restrições

- Bubble Tea v1.3.10, bubbles v1.0.0, lipgloss v1.1.0; nada de novas
  dependências sem necessidade.
- Textos continuam no i18n (en + pt-BR, paridade testada). MAIÚSCULAS são
  aplicadas na renderização (`strings.ToUpper`), não nas mensagens.
- Testes golden continuam sem cor; os goldens mudam de layout e são
  regenerados. Precisa de teste que garanta que nenhuma linha passa da largura e
  que toda linha tem exatamente a largura do terminal (sem buracos no fundo).

## Decisões do dono

- Base "vibrante" + respiro + ✓/○ (rodadas 1–2).
- Linha visual papel e tinta da referência, identidade KANIVET (rodada 3).
- Sempre creme, sem variante escura (rodada 4).
- Mockups de todas as telas aprovados como estão (rodada 5).
- As 5 funções novas entram junto com o visual, num trabalho só.

## Decisões minhas (podem ser revistas)

- Cores por perfil e o mapeamento de cada tag para uma cor da paleta.
- Ordem de corte em 80×24 e a regra de layout abaixo de 80 colunas.
- `r` no resumo reaproveita o fluxo de revisão em vez de executar direto.
- Degradação: 16 cores/`NO_COLOR` desistem do creme em vez de imitar.
- A caixa de log é o único bloco escuro da interface.
- O título grande usa só MAIÚSCULAS (sem fonte em blocos/figlet).
- **Trilha**: sempre os 5 passos do wizard. "CATEGORIA N DE 5" é a página de
  categorias do wizard (as 5 páginas da Fase 2), não o número de categorias do
  catálogo. No resumo a trilha mostra os 5 passos com ✓ e `FIM` à direita; o
  painel tem o cabeçalho próprio; ajuda e diálogos ficam por cima da tela atual,
  que mantém o seu cabeçalho.
- **`r` (tentar de novo)**: monta uma seleção só com os itens que falharam, mesmo
  perfil e versões, e entra na revisão; confirmar inicia uma execução nova, com
  contagem e tempo próprios, e o resumo mostra só essa nova execução.
- **80×24 manda**: o mockup é a referência de estilo, as medidas reais são as de
  80×24. Em 24 linhas o cabeçalho continua em caixa (3 linhas + sombra) e a
  lista da tela de itens precisa mostrar pelo menos 6 itens; se não couber,
  corta-se o respiro antes. A 80 colunas, lista (~40) e detalhe (~32) cabem lado
  a lado com margem e sombra.
- **Largura**: a sombra conta dentro da largura do terminal; nada desenha fora.
  Reescrever a renderização de todas as telas sobre um compositor comum é
  aceitável e esperado.
- **Detecção de cor**: pelo perfil do termenv/lipgloss (que já respeita
  `NO_COLOR`); os testes forçam o perfil true color para checar o fundo de cada
  célula e o perfil sem cor para os goldens.
- **Diff em 16 cores/`NO_COLOR`**: só os marcadores `+`/`-` atuais.

## Fica a cargo de quem implementa

- Como pintar o fundo sem buracos com lipgloss (cada estilo com `Background`,
  `lipgloss.Place` com `WithWhitespaceBackground`, ou um compositor próprio).
- Os glifos exatos da sombra e dos cantos, desde que leiam como bloco sólido
  deslocado para baixo-esquerda.
- A estrutura do código de tema/componentes (caixa, tag, botão, trilha, abas).
