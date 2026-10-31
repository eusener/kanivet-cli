# Spec Delta

## Purpose

Define o comportamento do instalador `install.sh`, que baixa e instala o
kanivet-cli de uma release pública com um único comando, sem Go e sem sudo, no
macOS e no Linux.

## ADDED Requirements

### Requirement: Instalação com um comando
O repositório SHALL ter um `install.sh` na raiz, acessível em `https://raw.githubusercontent.com/eusener/kanivet-cli/master/install.sh`, que roda com `curl -fsSL <url> | sh`. O script MUST funcionar em `sh` POSIX (dash no Ubuntu, bash no Fedora, sh do macOS) e precisar só de `curl` ou `wget`, `tar`, `uname` e uma ferramenta de SHA-256.

#### Scenario: Instalação no Ubuntu
- **WHEN** o usuário roda `curl -fsSL https://raw.githubusercontent.com/eusener/kanivet-cli/master/install.sh | sh` num Ubuntu
- **THEN** o kanivet-cli da release mais recente fica em `~/.local/bin/kanivet-cli` e o script imprime a versão instalada

#### Scenario: Instalação no macOS
- **WHEN** o usuário roda o mesmo comando num Mac com Apple Silicon
- **THEN** o script baixa o arquivo `darwin_arm64` e o binário roda sem bloqueio do Gatekeeper

### Requirement: Detecção de plataforma
O script SHALL escolher o arquivo pelo SO (`Darwin` → `darwin`, `Linux` → `linux`) e pela arquitetura (`x86_64`/`amd64` → `amd64`, `arm64`/`aarch64` → `arm64`). Combinação não suportada MUST encerrar com código diferente de zero e uma mensagem com o SO e a arquitetura detectados, sem baixar nada.

#### Scenario: Linux aarch64
- **WHEN** o script roda num Fedora em `aarch64`
- **THEN** ele baixa `kanivet-cli_<versão>_linux_arm64.tar.gz`

#### Scenario: Arquitetura não suportada
- **WHEN** o script roda num Linux `armv7l`
- **THEN** ele sai com erro dizendo que `linux/armv7l` não é suportado

### Requirement: Escolha de versão
Por padrão o script SHALL instalar a release "latest" (pré-releases não contam). A variável `KANIVET_VERSION` MUST fixar uma versão, com ou sem o `v` (`0.1.0` ou `v0.1.0`). Uma versão que não existe MUST encerrar com erro dizendo qual versão não foi encontrada.

#### Scenario: Versão fixada
- **WHEN** o usuário roda `curl -fsSL …/install.sh | KANIVET_VERSION=0.1.0 sh`
- **THEN** a `0.1.0` é instalada mesmo que exista uma versão mais nova

#### Scenario: Versão inexistente
- **WHEN** `KANIVET_VERSION=9.9.9`
- **THEN** o script sai com erro informando que a versão `v9.9.9` não existe e não altera nada no disco

### Requirement: Verificação de checksum obrigatória
O script MUST baixar o `checksums.txt` da mesma release e conferir o SHA-256 do arquivo antes de extrair, usando `sha256sum` ou `shasum -a 256`. Se o checksum não bater, ou se não houver ferramenta de SHA-256, o script MUST encerrar com erro sem instalar.

#### Scenario: Checksum divergente
- **WHEN** o arquivo baixado não corresponde ao SHA-256 do `checksums.txt`
- **THEN** o script sai com erro, apaga os temporários e não toca no binário já instalado

### Requirement: Destino sem sudo
O script SHALL instalar em `~/.local/bin` por padrão, criando a pasta se preciso, e MUST NOT pedir sudo. `KANIVET_INSTALL_DIR` MUST trocar o destino. A troca do binário MUST ser atômica (um binário existente é substituído só depois que o novo foi verificado e extraído).

#### Scenario: Destino personalizado
- **WHEN** `KANIVET_INSTALL_DIR=/opt/bin` e a pasta tem permissão de escrita
- **THEN** o binário fica em `/opt/bin/kanivet-cli`

#### Scenario: Reinstalação atualiza
- **WHEN** a `0.1.0` já está em `~/.local/bin` e o script instala a `0.2.0`
- **THEN** `~/.local/bin/kanivet-cli --version` mostra `0.2.0`

### Requirement: Aviso de PATH
Depois de instalar, se o destino não estiver no `PATH`, o script SHALL dizer como adicioná-lo para o shell do usuário (zsh ou bash), sem editar arquivos de rc por conta própria.

#### Scenario: ~/.local/bin fora do PATH
- **WHEN** a instalação termina e `~/.local/bin` não está no `PATH`
- **THEN** o script imprime a linha `export PATH="$HOME/.local/bin:$PATH"` e o arquivo de rc onde colocá-la

### Requirement: Script também anexado à release
Cada release SHALL anexar uma cópia do `install.sh` da tag, para que uma versão antiga possa ser instalada com o script daquela época.

#### Scenario: Script da tag
- **WHEN** o usuário baixa `install.sh` dos assets da release `v0.1.0`
- **THEN** o arquivo é idêntico ao `install.sh` do commit da tag `v0.1.0`
