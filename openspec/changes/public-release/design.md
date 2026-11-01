# Design

## Context

- Binário único em Go 1.24 (`cmd/kanivet-cli`), com o catálogo embutido via
  `embed`. Nenhuma dependência usa cgo, então o cross-compile com
  `CGO_ENABLED=0` gera binários estáticos para os quatro alvos.
- `platform.Detect` só aceita `darwin`/`linux` em `amd64`/`arm64`, que é
  exatamente a matriz da release.
- Não existe tag, `LICENSE`, `--version`, nem workflow de release. O `ci.yml`
  já roda testes em Ubuntu e macOS, integração em containers
  (`test/integration/run.sh`) e um install real no macOS.
- O `doctor` já verifica se `~/.local/bin` está no `PATH`, então esse é o destino
  natural do `install.sh` (coerente com "sem sudo à toa").
- Não existe `.goreleaser*` nem arquivos de empacotamento.

## Goals / Non-Goals

**Goals:**
- Um pipeline só, disparado por tag, que gera, verifica e publica todos os
  artefatos das specs `release-distribution` e `install-script`.
- O mesmo pipeline roda em modo snapshot no CI, para que a tag nunca seja o
  primeiro teste da configuração.
- A versão do binário vem da tag, sem arquivo de versão para editar à mão.

**Non-Goals:**
- Repositórios de pacotes (apt/dnf), Homebrew, assinatura (Apple ou GPG),
  SBOM, autoatualização. A estrutura escolhida (GoReleaser) permite somar cada
  um depois sem refazer nada.
- Universal binary do macOS: dois arquivos separados mantêm o `install.sh`
  simples e o download menor.

## Decisions

### GoReleaser com nfpm, em vez de scripts próprios
Um `.goreleaser.yaml` (versão 2) cobre build cruzado, `.tar.gz`, `.deb`/`.rpm`
(nfpm embutido), `checksums.txt`, changelog e upload para a release.
- Alternativa: `go build` em matriz + `fpm`/`dpkg-deb`/`rpmbuild` à mão. Mais
  código para manter e nomes de arquivo e checksums fáceis de dessincronizar.
- Alternativa: `nfpm` isolado. Resolveria só os pacotes, e o resto continuaria
  manual.

Pontos da configuração:
- `builds`: `main: ./cmd/kanivet-cli`, `env: [CGO_ENABLED=0]`,
  `goos: [darwin, linux]`, `goarch: [amd64, arm64]`, `flags: [-trimpath]`,
  `ldflags: -s -w -X github.com/eusener/kanivet-cli/internal/buildinfo.Version={{.Version}} -X …Commit={{.ShortCommit}} -X …Date={{.CommitDate}}`.
  `mod_timestamp: {{.CommitTimestamp}}` deixa o build reprodutível.
- `archives`: `name_template: kanivet-cli_{{.Version}}_{{.Os}}_{{.Arch}}`,
  formato `tar.gz`, `files: [README.md, LICENSE*]`.
- `nfpms`: `formats: [deb, rpm]`, `bindir: /usr/bin`, `recommends: [git, curl, ca-certificates]`,
  maintainer e homepage do repositório. O nfpm já mapeia `amd64`/`arm64` para
  `x86_64`/`aarch64` no rpm; o `file_name_template` usa `{{ .ConventionalFileName }}`
  para seguir o padrão de cada formato (`kanivet-cli_0.1.0_amd64.deb`,
  `kanivet-cli-0.1.0-1.x86_64.rpm`). Os pacotes não têm scripts de pós-instalação
  nem tocam no `$HOME`.
- `checksum: name_template: checksums.txt` (sha256).
- `release`: `draft: true`, `prerelease: auto`, `extra_files: [install.sh]`,
  `footer` com os três comandos de instalação.
- `changelog`: `use: github`, filtros que excluem `^docs`, `^test`, `^chore`,
  `Merge`; grupos "Novidades" (`^feat`) e "Correções" (`^fix`), seguindo os
  commits convencionais que o repositório já usa.

### Rascunho → testes de fumaça → publicação
O workflow `release.yml` (em `push: tags: ['v*.*.*']`) tem quatro estágios:
1. `test`: os mesmos `go vet`, `go test -race` e `catalog lint` do CI.
2. `goreleaser`: `goreleaser release --clean` cria a release **como rascunho** e
   envia o `dist/` como artifact do Actions.
3. `smoke` (matriz): `ubuntu` (container `ubuntu:24.04` instala o `.deb` com
   `apt install ./…`), `fedora` (container `fedora:latest` com `dnf install ./…`)
   e `macos` (`macos-latest`, roda o `install.sh` contra o `dist/` local). Cada
   um roda `kanivet-cli --version` (conferindo a versão da tag) e
   `kanivet-cli catalog lint`, depois remove o pacote e confere que o binário sumiu.
4. `publish`: `gh release edit <tag> --draft=false --latest` (a não ser que seja
   pré-release). Só roda se todos os `smoke` passarem.
- Alternativa: publicar direto e testar depois. Um `.rpm` quebrado ficaria
  público até alguém notar.
- O `smoke` do macOS testa o `install.sh` e não só o tar, porque é o caminho
  recomendado no macOS. Para isso o script aceita `KANIVET_BASE_URL` (padrão:
  `https://github.com/eusener/kanivet-cli/releases/download`). Essa variável fica
  documentada só nos comentários do script, como recurso de teste e de mirror.

Permissões: `contents: write` só nos jobs `goreleaser` e `publish`; os outros
usam `contents: read`. O token é o `GITHUB_TOKEN` do workflow, sem PAT.

### Snapshot no CI
Um job novo em `ci.yml`, `release-check`: `goreleaser check` e
`goreleaser release --snapshot --clean --skip=publish`, só no Ubuntu. O job
garante que a configuração e o nfpm continuam válidos a cada PR.

### Pacote `internal/buildinfo`
O nome não é `internal/version` porque esse pacote já existe e extrai e compara
as versões dos itens do catálogo.

Um pacote mínimo com `Version`, `Commit` e `Date` (variáveis `-X`, padrão vazio)
e uma função `String()`:
- se `Version` foi injetada, usa as três variáveis;
- senão, `debug.ReadBuildInfo()`: `Main.Version` quando é uma tag limpa, e não `(devel)`, pseudo-versão ou `+dirty` (caso do
  `go install …@v0.1.0`), senão `dev`, e `vcs.revision`/`vcs.time` das settings
  para o commit e a data;
- campos que faltarem viram `unknown`, nunca string vazia.

O root do cobra recebe `Version: version.String()` e um `SetVersionTemplate`
para imprimir `kanivet-cli <versão> (<commit>, <data>)`. O cobra trata `--version`
antes do `RunE`, então a TUI e a checagem de TTY não rodam. O `doctor` ganha a
linha `doctor.version` em `internal/i18n/messages.go` (en e pt-BR), coberta pelo
teste de cobertura de i18n que já existe.
- Alternativa: subcomando `version`. A flag é a convenção do cobra e de quem
  escreve scripts; um subcomando pode vir depois sem conflito.

### `install.sh` em sh POSIX
- `set -eu`, sem bashismos (validado com `shellcheck -s sh` no CI).
- `uname -s`/`uname -m` → `os`/`arch`, com a tabela da spec.
- Versão: `KANIVET_VERSION` normalizada para `vX.Y.Z`; sem ela, segue o redirect
  de `https://github.com/eusener/kanivet-cli/releases/latest` (`curl -fsSLI -o /dev/null -w '%{url_effective}'`)
  para descobrir a tag, sem usar a API do GitHub, que tem limite de requisições
  anônimas e exigiria `jq`.
- Download com `curl -fsSL` ou, sem curl, `wget -qO`, para um `mktemp -d`
  limpo por `trap` em qualquer saída.
- Checksum: `grep` da linha do arquivo em `checksums.txt` e `sha256sum -c` ou
  `shasum -a 256 -c`.
- Instalação: extrai no temporário, `mv` para `<destino>/.kanivet-cli.tmp` e
  `mv` final para `<destino>/kanivet-cli` (o rename é atômico no mesmo sistema de
  arquivos).
- No macOS, arquivos baixados por `curl` não recebem `com.apple.quarantine`, e o
  linker do Go já aplica assinatura ad-hoc em `darwin/arm64`. Então o binário
  roda sem assinatura da Apple. O `xattr` fica documentado só para download pelo
  navegador.

### Tag `v0.1.0`
A primeira release sai da tag `v0.1.0` em `master`, criada depois que o
workflow já estiver mergeado e o snapshot passando. O checklist manual do README
("Teste manual num terminal real") continua valendo como gate antes da tag.

## Risks / Trade-offs

- [Repositório privado ou downloads com autenticação] → o `install.sh` e os links
  dos pacotes param de funcionar para o público. Mitigação: confirmar a
  visibilidade pública antes da tag (tarefa explícita).
- [Pacotes sem assinatura GPG] → `apt install ./x.deb` aceita, e o `dnf` aceita
  pacote local sem assinatura por padrão (`localpkg_gpgcheck=0`). Se alguma
  configuração exigir, o README mostra `--nogpgcheck`. Assinar fica para uma
  mudança futura.
- [Binário macOS sem notarização] → download pelo navegador é bloqueado pelo
  Gatekeeper. Mitigação: o script (sem quarantine) é o caminho recomendado, e o
  `xattr` está no README.
- [Smoke em container não cobre `arm64`] → só o `amd64` é instalado nos
  containers, e o `arm64` do macOS roda no `macos-latest` (Apple Silicon). O
  Linux `arm64` é coberto pelo cross-compile estático; um job em runner `ubuntu-24.04-arm`
  pode ser somado depois.
- [`/releases/latest` ignora pré-releases e rascunhos] → é o comportamento
  desejado, mas, entre o rascunho e o `publish`, o script ainda instala a versão
  anterior. Aceitável.
- [Tag criada antes do merge do workflow] → nenhuma release é gerada. Mitigação:
  a ordem está nas tarefas; se acontecer, apagar a tag e criá-la de novo.

## Migration Plan

1. Mergear versão, GoReleaser, workflows, `install.sh` e README em `master`,
   com o `release-check` verde.
2. Rodar o checklist manual do README.
3. `git tag -a v0.1.0 -m "v0.1.0" && git push origin v0.1.0`.
4. Acompanhar o `release.yml` até o `publish`.

Rollback: se a release sair com problema, marcar como rascunho de novo
(`gh release edit v0.1.0 --draft`) ou apagá-la, corrigir e lançar `v0.1.1`. Uma
tag publicada não é reaproveitada.

## Open Questions

Nenhuma. A licença foi definida como MIT: `LICENSE` na raiz, incluído nos `.tar.gz`,
em `/usr/share/doc/kanivet-cli/copyright` nos pacotes e no campo `license` do nfpm.
