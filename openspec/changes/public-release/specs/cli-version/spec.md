# Spec Delta

## Purpose

Define como o binário do kanivet-cli informa a própria versão, para que usuários,
scripts e relatos de bug saibam exatamente qual build está rodando.

## ADDED Requirements

### Requirement: Flag --version
`kanivet-cli --version` SHALL imprimir numa linha a versão, o commit curto e a data do build, no formato `kanivet-cli <versão> (<commit>, <data AAAA-MM-DD>)`, e sair com código 0 sem abrir a TUI, sem detectar estado e sem precisar de TTY.

#### Scenario: Binário da release
- **WHEN** o usuário roda `kanivet-cli --version` com o binário da release `v0.1.0`
- **THEN** a saída é `kanivet-cli 0.1.0 (<commit da tag>, <data do build>)` e o código de saída é 0

#### Scenario: Sem TTY
- **WHEN** `kanivet-cli --version | cat` roda num pipe
- **THEN** a saída é a mesma linha de versão, e não a ajuda

### Requirement: Versão em builds locais
Um binário compilado sem as informações da release SHALL mostrar a versão do módulo quando instalado por `go install …@vX.Y.Z`, e `dev` com o commit do VCS, quando disponível, num `go build` local. Nunca MUST imprimir uma versão vazia.

#### Scenario: go build local
- **WHEN** o binário é compilado com `go build ./cmd/kanivet-cli` dentro do clone
- **THEN** `kanivet-cli --version` começa com `kanivet-cli dev`

#### Scenario: go install de uma tag
- **WHEN** o binário é instalado com `go install github.com/eusener/kanivet-cli/cmd/kanivet-cli@v0.1.0`
- **THEN** `kanivet-cli --version` começa com `kanivet-cli 0.1.0`

### Requirement: Versão no doctor
`kanivet-cli doctor` SHALL mostrar a versão do kanivet-cli como primeira linha, traduzida conforme o idioma (en e pt-BR).

#### Scenario: Doctor em pt-BR
- **WHEN** o usuário roda `LANG=pt_BR.UTF-8 kanivet-cli doctor`
- **THEN** a primeira linha é `Versão: 0.1.0 (<commit>, <data>)`
