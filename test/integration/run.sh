#!/usr/bin/env bash
# Teste de integração: instala um perfil num container limpo e confere o resultado.
# Uso: test/integration/run.sh <ubuntu|debian|fedora|arch|opensuse> [perfil]
# Precisa de Docker (ou Podman com CONTAINER=podman). Use GITHUB_TOKEN para evitar
# o limite de requisições da API do GitHub nos downloads do mise.
set -euo pipefail

distro=${1:?uso: run.sh <distro> [perfil]}
profile=${2:-blank}
engine=${CONTAINER:-docker}
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)

case "$(uname -m)" in
  x86_64 | amd64) goarch=amd64 ;;
  arm64 | aarch64) goarch=arm64 ;;
  *) echo "arquitetura não suportada: $(uname -m)" >&2; exit 1 ;;
esac

bin="$root/.build/kanivet-cli-linux-$goarch"
mkdir -p "$root/.build"
(cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH=$goarch go build -o "$bin" ./cmd/kanivet-cli)

image="kanivet-cli-it-$distro"
"$engine" build -q -t "$image" -f "$here/Dockerfile.$distro" "$here" >/dev/null

"$engine" run --rm \
  -e GITHUB_TOKEN="${GITHUB_TOKEN:-}" -e PROFILE="$profile" \
  -v "$bin:/usr/local/bin/kanivet-cli:ro" \
  "$image" bash -c '
    set -uo pipefail
    kanivet-cli install --profile "$PROFILE" --yes --no-config kitty-config,herdr-config
    install_rc=$?
    kanivet-cli status --json > /tmp/status.json

    # Todo item do perfil precisa estar presente, a não ser que não rode aqui (unsupported).
    items=$(kanivet-cli profiles --json | jq -r --arg p "$PROFILE" ".[] | select(.id == \$p) | .items[]")
    # O shell do container é bash: os plugins só-zsh são pulados de propósito.
    items=$(echo "$items" | grep -v "^zsh-")
    missing=$(jq -r --arg items "$items" "
      (\$items | split(\"\n\")) as \$want
      | .[] | select(.id as \$id | \$want | index(\$id))
      | select(.status == \"absent\" or .status == \"external-outdated\")
      | \"\(.id): \(.status)\"" /tmp/status.json)

    echo "--- status ($PROFILE)"
    jq -r ".[] | select(.status != \"absent\") | \"\(.status)\t\(.id) \(.version)\"" /tmp/status.json | sort

    rc=0
    if [ -n "$missing" ]; then echo "--- ausentes:"; echo "$missing"; rc=1; fi
    bash -n "$HOME/.config/kanivet-cli/shell/init.bash" || rc=1
    if command -v zsh >/dev/null 2>&1; then zsh -n "$HOME/.config/kanivet-cli/shell/init.zsh" || rc=1; fi
    # O shell gerado precisa carregar sem erros num bash interativo.
    bash -c ". \"\$HOME/.config/kanivet-cli/shell/init.bash\"" 2>/tmp/bash-err
    if [ -s /tmp/bash-err ]; then echo "--- erros ao carregar o init.bash:"; cat /tmp/bash-err; rc=1; fi
    grep -q "kanivet-cli" "$HOME/.bashrc" || { echo "bloco ausente no .bashrc"; rc=1; }
    if [ $install_rc -ne 0 ]; then echo "install saiu com $install_rc"; rc=1; fi
    exit $rc
  '
