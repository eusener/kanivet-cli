# Proposal

## Why

Hoje o kanivet-cli só pode ser usado por quem clona o repositório e compila com
Go 1.24+, o que contradiz a proposta do projeto ("um terminal pronto em qualquer
máquina nova": a máquina nova ainda não tem Go). Não existe tag, release nem forma
de saber qual versão está instalada. Uma release pública com binários e pacotes
nativos permite instalar no macOS, Ubuntu e Fedora com um comando.

## What Changes

- Publicação automática de uma GitHub Release a cada tag `vX.Y.Z`, começando por
  `v0.1.0`, com:
  - arquivos `.tar.gz` para `darwin` e `linux`, em `amd64` e `arm64`;
  - pacote `.deb` (Ubuntu/Debian) e pacote `.rpm` (Fedora) para `amd64` e `arm64`;
  - arquivo de checksums SHA-256 de todos os artefatos;
  - notas da release geradas a partir dos commits desde a tag anterior.
- Script `install.sh` para `curl -fsSL … | sh`: detecta SO e arquitetura, baixa o
  `.tar.gz` da release (a mais recente ou uma versão fixada), confere o checksum e
  instala o binário em `~/.local/bin` sem sudo.
- `kanivet-cli --version` passa a mostrar a versão, o commit e a data do build;
  `kanivet-cli doctor` passa a mostrar a versão também.
- Testes de fumaça antes de publicar: o `.deb` instalado num container Ubuntu, o
  `.rpm` num container Fedora e o `install.sh` no macOS, todos rodando
  `kanivet-cli --version` e `kanivet-cli catalog lint`. A release só é publicada
  se os três passarem.
- O CI valida a configuração de release em todo push, com um build de snapshot
  sem publicar nada.
- O README ganha a seção "Instalação" com os três caminhos (script, `.deb`, `.rpm`)
  e a instrução de `xattr` para quem baixar o binário do macOS pelo navegador.

Fora de escopo nesta mudança: tap do Homebrew, repositórios apt/dnf próprios
(PPA, COPR), assinatura e notarização na Apple, assinatura GPG dos pacotes,
autoatualização do binário, Windows.

## Capabilities

### New Capabilities

- `release-distribution`: quais artefatos cada release pública publica, para
  quais plataformas, como são verificados antes de publicar e como se instala cada
  um.
- `install-script`: o comportamento do instalador `install.sh` (detecção de
  plataforma, escolha de versão, verificação de checksum, destino e erros).
- `cli-version`: como o binário informa a própria versão (`--version` e
  `doctor`), inclusive em builds locais fora da release.

### Modified Capabilities

Nenhuma (o projeto ainda não tem specs em `openspec/specs/`).

## Impact

- **Código**: `internal/cli/root.go` (flag `--version`), `internal/cli/doctor.go`,
  um novo pacote/variável de versão preenchido por `-ldflags`; mensagens novas em
  `internal/i18n/messages.go` (en e pt-BR).
- **Arquivos novos**: `.goreleaser.yaml`, `.github/workflows/release.yml`,
  `install.sh` na raiz do repositório.
- **CI**: `.github/workflows/ci.yml` ganha um job de `goreleaser check` e build de
  snapshot.
- **Dependências**: GoReleaser (com nfpm) só no CI, sem nenhuma dependência nova
  no binário. O build passa a ser `CGO_ENABLED=0`.
- **Repositório**: precisa ser público para o script e os downloads funcionarem
  sem token; o workflow de release precisa de `contents: write`.
- **Docs**: `README.md` (seção de instalação; "Início rápido" deixa de exigir Go).
