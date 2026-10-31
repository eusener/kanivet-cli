# Spec Delta

## Purpose

Define o que cada release pública do kanivet-cli publica, para quais plataformas,
como os artefatos são verificados antes de chegar ao usuário e como se instala
cada um no macOS, Ubuntu e Fedora.

## ADDED Requirements

### Requirement: Release disparada por tag de versão
O projeto SHALL publicar uma GitHub Release pública quando uma tag no formato `vX.Y.Z` (SemVer, com sufixo de pré-release opcional) for enviada ao repositório. Outras tags e pushes de branch MUST NOT publicar release. A primeira release pública SHALL ser `v0.1.0`.

#### Scenario: Tag de versão publica a release
- **WHEN** a tag `v0.1.0` é enviada para o GitHub
- **THEN** uma release pública chamada `v0.1.0` aparece na página de releases com todos os artefatos

#### Scenario: Push sem tag não publica
- **WHEN** um commit é enviado para `master` sem tag
- **THEN** nenhuma release é criada ou alterada

#### Scenario: Tag de pré-release
- **WHEN** a tag `v0.2.0-rc.1` é enviada
- **THEN** a release é publicada marcada como pré-release e não passa a ser a "latest"

### Requirement: Arquivos binários por plataforma
Cada release SHALL conter um arquivo `.tar.gz` para cada combinação de `darwin` e `linux` com `amd64` e `arm64`, com o nome `kanivet-cli_<versão>_<os>_<arch>.tar.gz` (a versão sem o `v`). Cada arquivo MUST conter o executável `kanivet-cli`, o `README.md` e a licença, se houver uma. O binário MUST ser estático (sem libc dinâmica) e não depender de nada em tempo de execução.

#### Scenario: Quatro arquivos binários
- **WHEN** a release `v0.1.0` é publicada
- **THEN** ela contém `kanivet-cli_0.1.0_darwin_amd64.tar.gz`, `kanivet-cli_0.1.0_darwin_arm64.tar.gz`, `kanivet-cli_0.1.0_linux_amd64.tar.gz` e `kanivet-cli_0.1.0_linux_arm64.tar.gz`

#### Scenario: Binário Linux roda em qualquer distro suportada
- **WHEN** o binário de `linux_amd64` é extraído e executado num Ubuntu ou num Fedora sem Go instalado
- **THEN** `kanivet-cli --version` imprime a versão e sai com código 0

### Requirement: Pacote .deb para Ubuntu
Cada release SHALL conter um pacote `.deb` para `amd64` e outro para `arm64`. O pacote MUST instalar o executável em `/usr/bin/kanivet-cli`, declarar a versão da release e recomendar (sem exigir) `git`, `curl` e `ca-certificates`. Remover o pacote MUST remover o executável e MUST NOT apagar o estado do usuário em `~/.local/state/kanivet-cli`.

#### Scenario: Instalação no Ubuntu
- **WHEN** o usuário roda `sudo apt install ./kanivet-cli_0.1.0_amd64.deb` num Ubuntu 24.04 limpo
- **THEN** a instalação termina sem erro e `kanivet-cli --version` mostra `0.1.0`

#### Scenario: Atualização pelo pacote
- **WHEN** a versão `0.1.0` está instalada e o usuário instala o `.deb` da `0.2.0`
- **THEN** o apt trata como atualização e `kanivet-cli --version` mostra `0.2.0`

#### Scenario: Remoção preserva o estado
- **WHEN** o usuário roda `sudo apt remove kanivet-cli`
- **THEN** `/usr/bin/kanivet-cli` deixa de existir e `~/.local/state/kanivet-cli` continua intacto

### Requirement: Pacote .rpm para Fedora
Cada release SHALL conter um pacote `.rpm` para `x86_64` e outro para `aarch64`, com as mesmas regras do `.deb`: executável em `/usr/bin/kanivet-cli`, versão da release, dependências fracas (`Recommends`) em `git`, `curl` e `ca-certificates`, e remoção sem tocar no estado do usuário.

#### Scenario: Instalação no Fedora
- **WHEN** o usuário roda `sudo dnf install ./kanivet-cli-0.1.0-1.x86_64.rpm` num Fedora limpo
- **THEN** a instalação termina sem erro e `kanivet-cli --version` mostra `0.1.0`

#### Scenario: Remoção no Fedora
- **WHEN** o usuário roda `sudo dnf remove kanivet-cli`
- **THEN** `/usr/bin/kanivet-cli` deixa de existir e `~/.local/state/kanivet-cli` continua intacto

### Requirement: Checksums de todos os artefatos
Cada release SHALL conter um arquivo `checksums.txt` com o SHA-256 de cada `.tar.gz`, `.deb` e `.rpm` publicado, no formato aceito por `sha256sum -c` e `shasum -a 256 -c`.

#### Scenario: Verificação manual
- **WHEN** o usuário baixa um artefato e o `checksums.txt` e roda `sha256sum -c --ignore-missing checksums.txt`
- **THEN** o artefato aparece como `OK`

### Requirement: Verificação antes de publicar
A release MUST ser verificada antes de ficar visível: os testes do projeto passam, o `.deb` é instalado num container Ubuntu, o `.rpm` num container Fedora e o `.tar.gz` de `darwin` é instalado pelo `install.sh` num macOS, e em cada um `kanivet-cli --version` e `kanivet-cli catalog lint` saem com código 0. Se qualquer verificação falhar, a release MUST NOT ser publicada.

#### Scenario: Falha no Fedora bloqueia a release
- **WHEN** o `.rpm` não instala no container Fedora
- **THEN** a release continua como rascunho, invisível para o público, e o workflow termina com falha

#### Scenario: Tudo passa
- **WHEN** as três verificações passam
- **THEN** a release deixa de ser rascunho e passa a ser a "latest"

### Requirement: Notas da release
Cada release SHALL ter notas com a lista de mudanças desde a tag anterior, geradas a partir dos commits, sem os commits de merge, de docs ou de testes, e com os comandos de instalação para macOS, Ubuntu e Fedora.

#### Scenario: Notas com instruções
- **WHEN** a release é publicada
- **THEN** as notas listam os commits `feat` e `fix` desde a tag anterior e incluem o comando do `install.sh`, o do `apt install` e o do `dnf install`

### Requirement: Configuração de release validada no CI
Todo push e pull request SHALL validar a configuração de release e gerar todos os artefatos em modo snapshot, sem publicar nada, para que uma configuração quebrada seja detectada antes da tag.

#### Scenario: Configuração inválida num PR
- **WHEN** um PR deixa a configuração de release inválida
- **THEN** o CI do PR falha e nenhuma release é tocada

### Requirement: Instruções de instalação documentadas
O README SHALL ter uma seção de instalação com os comandos para o script, o `.deb` e o `.rpm`, deixar o build com Go como alternativa para quem desenvolve, e explicar o `xattr -d com.apple.quarantine` para quem baixar o binário do macOS pelo navegador, já que o binário não é assinado.

#### Scenario: Usuário de macOS sem Go
- **WHEN** um usuário de macOS sem Go segue o README
- **THEN** ele instala o kanivet-cli com um único comando, sem compilar
