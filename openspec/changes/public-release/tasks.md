# Tasks

## 1. Versão no binário

- [x] 1.1 Criar `internal/buildinfo` com `Version`, `Commit`, `Date` e `String()` (ldflags → build info → `dev`/`unknown`), com testes de tabela para os três casos, e verificar com `go test ./internal/buildinfo`
- [x] 1.2 Ligar `Version` e `SetVersionTemplate` no root em `internal/cli/root.go` e testar em `internal/cli` que `--version` imprime `kanivet-cli <versão> (<commit>, <data>)`, sai com 0 e não chama `RunTUI` mesmo com `IsTTY` verdadeiro
- [x] 1.3 Adicionar a primeira linha `doctor.version` ao `doctor` com mensagens en e pt-BR em `internal/i18n/messages.go`, e verificar com `go test ./internal/cli ./internal/i18n` (incluindo o teste de cobertura de i18n)
- [x] 1.4 Verificar à mão que `go build ./cmd/kanivet-cli && ./kanivet-cli --version` começa com `kanivet-cli dev` e que um build com `-ldflags "-X …Version=0.1.0"` mostra `0.1.0`

## 2. Configuração do GoReleaser

- [x] 2.1 Criar `.goreleaser.yaml` (v2) com builds `CGO_ENABLED=0` para darwin/linux × amd64/arm64, `-trimpath`, ldflags de `internal/buildinfo` e `mod_timestamp`, e verificar com `goreleaser check`
- [x] 2.2 Configurar `archives` (`kanivet-cli_{{.Version}}_{{.Os}}_{{.Arch}}.tar.gz` com `README.md` e `LICENSE*`) e `checksum` (`checksums.txt`), e verificar com `goreleaser release --snapshot --clean` que `dist/` tem os quatro `.tar.gz` e que `sha256sum -c checksums.txt --ignore-missing` passa dentro de `dist/`
- [x] 2.3 Configurar `nfpms` (deb e rpm, `/usr/bin`, `recommends: git, curl, ca-certificates`, `ConventionalFileName`) e verificar no snapshot que existem `.deb` amd64/arm64 e `.rpm` x86_64/aarch64 e que `dpkg-deb -c`/`rpm -qlp` listam só `/usr/bin/kanivet-cli` (além da doc)
- [x] 2.4 Configurar `release` (`draft: true`, `prerelease: auto`, `extra_files: install.sh`, footer com os comandos de instalação) e `changelog` (grupos feat/fix, filtros de docs/test/chore/merge), e verificar com `goreleaser release --snapshot --clean` sem erros
- [x] 2.5 Adicionar `dist/` ao `.gitignore` e verificar que `git status` não mostra `dist/` depois do snapshot

## 3. Script de instalação

- [x] 3.1 Escrever `install.sh` em sh POSIX: detecção de SO/arch, `KANIVET_VERSION` (com ou sem `v`), descoberta da latest pelo redirect, `KANIVET_BASE_URL`, download com curl ou wget num `mktemp -d` com `trap`, e verificar com `shellcheck -s sh install.sh` sem avisos
- [x] 3.2 Implementar a verificação de checksum obrigatória (`sha256sum` ou `shasum -a 256`, erro sem ferramenta) e a instalação atômica em `KANIVET_INSTALL_DIR` (padrão `~/.local/bin`), com o aviso de PATH para zsh/bash
- [x] 3.3 Testar o script contra o `dist/` do snapshot servido localmente (`python3 -m http.server` + `KANIVET_BASE_URL`, com a pasta no layout `<tag>/<arquivo>`) e verificar: instalação nova, reinstalação, `KANIVET_INSTALL_DIR` personalizado, versão inexistente (erro, nada muda), checksum adulterado (erro, binário antigo intacto) e arquitetura falsa não suportada (erro com `os/arch`)
- [x] 3.4 Rodar o mesmo teste em `dash` (`docker run ubuntu:24.04`) para confirmar compatibilidade POSIX, e verificar que `kanivet-cli --version` do binário instalado mostra a versão do snapshot

## 4. Workflows

- [x] 4.1 Adicionar ao `.github/workflows/ci.yml` o job `release-check` (`goreleaser check`, snapshot com `--skip=publish` e `shellcheck -s sh install.sh`) e verificar que ele passa num PR
- [x] 4.2 Criar `.github/workflows/release.yml` em `push: tags: ['v*.*.*']` com os jobs `test` → `goreleaser` (rascunho + upload do `dist/`) → `smoke` (ubuntu `.deb`, fedora `.rpm`, macos `install.sh` com `KANIVET_BASE_URL` local) → `publish` (`gh release edit --draft=false`, `--latest` só fora de pré-release), com `contents: write` só em `goreleaser` e `publish`, e verificar com `actionlint`
- [x] 4.3 Fazer cada job de `smoke` conferir que `kanivet-cli --version` contém a versão da tag, rodar `kanivet-cli catalog lint`, remover o pacote e conferir que `/usr/bin/kanivet-cli` sumiu, e verificar com um ensaio numa tag de pré-release descartável (`v0.1.0-rc.1`) num fork ou no próprio repositório, apagando a release e a tag depois

## 5. Documentação

- [x] 5.1 Reescrever o "Início rápido" do `README.md` com a seção "Instalação" (comando do `install.sh`, `apt install ./…deb`, `dnf install ./…rpm`, `KANIVET_VERSION`/`KANIVET_INSTALL_DIR`, `xattr -d com.apple.quarantine` para download pelo navegador, `--nogpgcheck` se o dnf recusar) e manter o `go build` como "Compilar do código", e verificar seguindo os comandos do README contra a release de ensaio da 4.3
- [x] 5.2 Documentar no README, na seção de testes, como fazer uma release (checklist manual, tag anotada, acompanhar o workflow, rollback) e verificar que os comandos citados existem no `release.yml`

## 6. Primeira release pública

- [x] 6.1 Confirmar que o repositório `eusener/kanivet-cli` é público e que os workflows têm permissão de escrita, e verificar que `curl -fsSI https://github.com/eusener/kanivet-cli/releases/latest` responde sem autenticação
- [ ] 6.2 Com tudo mergeado e o `release-check` verde, rodar o checklist manual do README e criar e enviar a tag anotada `v0.1.0`, e verificar que o `release.yml` termina com o job `publish` verde
- [ ] 6.3 Verificar a release publicada de ponta a ponta: `curl -fsSL …/install.sh | sh` num Mac, `apt install ./kanivet-cli_0.1.0_amd64.deb` num Ubuntu 24.04 e `dnf install ./kanivet-cli-0.1.0-1.x86_64.rpm` num Fedora, com `kanivet-cli --version` mostrando `0.1.0` nos três
