#!/bin/sh
# Testa o install.sh contra um dist/ do GoReleaser servido localmente.
#
#   test/release/install-test.sh [dist] [porta]
#
# Precisa de python3 (servidor HTTP) e curl. Cobre: instalação nova, aviso de PATH,
# reinstalação, versão inexistente, checksum adulterado e arquitetura não suportada.
set -u

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
SCRIPT="$ROOT/install.sh"
DIST=$(cd "${1:-$ROOT/dist}" && pwd)
PORT=${2:-18765}
W=$(mktemp -d)

V=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$DIST/metadata.json")
T="v$V"

sha() { if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi; }

mkdir -p "$W/srv/$T" "$W/srv/vbad"
cp "$DIST"/*.tar.gz "$DIST/checksums.txt" "$W/srv/$T/"
for f in "$DIST"/*.tar.gz; do
	b=$(basename "$f" | sed "s/_${V}_/_bad_/")
	cp "$f" "$W/srv/vbad/$b"
	echo "0000000000000000000000000000000000000000000000000000000000000000  $b" >>"$W/srv/vbad/checksums.txt"
done

(cd "$W/srv" && exec python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1) &
SRV=$!
trap 'kill $SRV 2>/dev/null; rm -rf "$W"' EXIT
i=0
until curl -fsS "http://127.0.0.1:$PORT/$T/checksums.txt" >/dev/null 2>&1; do
	i=$((i + 1))
	[ $i -gt 50 ] && echo "servidor não subiu" && exit 1
	sleep 0.2
done

export LANG=C LC_ALL=C KANIVET_BASE_URL="http://127.0.0.1:$PORT"
fail=0
ok() { echo "ok   - $1"; }
ko() {
	echo "FAIL - $1"
	printf '%s\n' "$2" | sed 's/^/       /'
	fail=1
}
D="$W/bin"

out=$(KANIVET_VERSION=$V KANIVET_INSTALL_DIR=$D PATH=/usr/bin:/bin sh "$SCRIPT" 2>&1)
if "$D/kanivet-cli" --version | grep -qF "kanivet-cli $V "; then ok "instala $V"; else ko "instala $V" "$out"; fi
if echo "$out" | grep -q 'export PATH='; then ok "avisa quando o destino não está no PATH"; else ko "aviso de PATH" "$out"; fi
if "$D/kanivet-cli" catalog lint >/dev/null 2>&1; then ok "catalog lint do binário instalado"; else ko "catalog lint" "$("$D/kanivet-cli" catalog lint 2>&1)"; fi

out=$(KANIVET_VERSION="v$V" KANIVET_INSTALL_DIR=$D PATH="$D:/usr/bin:/bin" sh "$SCRIPT" 2>&1)
if echo "$out" | grep -q 'export PATH='; then ko "não avisa com o destino no PATH" "$out"; else ok "aceita a versão com v e não avisa com o destino no PATH"; fi

printf '#!/bin/sh\necho kanivet-cli 0.0.1-old\n' >"$D/kanivet-cli"
out=$(KANIVET_VERSION=$V KANIVET_INSTALL_DIR=$D sh "$SCRIPT" 2>&1)
if "$D/kanivet-cli" --version | grep -qF "kanivet-cli $V "; then ok "reinstalação troca o binário"; else ko "reinstalação" "$out"; fi
if [ -e "$D/.kanivet-cli.tmp" ]; then ko "não deixa temporário no destino" "$D/.kanivet-cli.tmp"; else ok "não deixa temporário no destino"; fi

before=$(sha "$D/kanivet-cli")
out=$(KANIVET_VERSION=9.9.9 KANIVET_INSTALL_DIR=$D sh "$SCRIPT" 2>&1) && c=0 || c=$?
if [ "$c" -ne 0 ] && echo "$out" | grep -q 'v9.9.9 not found' && [ "$before" = "$(sha "$D/kanivet-cli")" ]; then
	ok "versão inexistente falha sem alterar nada"
else ko "versão inexistente (exit $c)" "$out"; fi

out=$(KANIVET_VERSION=bad KANIVET_INSTALL_DIR=$D sh "$SCRIPT" 2>&1) && c=0 || c=$?
if [ "$c" -ne 0 ] && echo "$out" | grep -q 'checksum mismatch' && [ "$before" = "$(sha "$D/kanivet-cli")" ]; then
	ok "checksum adulterado falha e mantém o binário anterior"
else ko "checksum adulterado (exit $c)" "$out"; fi

mkdir -p "$W/shim"
# shellcheck disable=SC2016 # o $1 é do shim
printf '#!/bin/sh\n[ "$1" = -m ] && echo armv7l || echo Linux\n' >"$W/shim/uname"
chmod +x "$W/shim/uname"
out=$(PATH="$W/shim:$PATH" KANIVET_VERSION=$V KANIVET_INSTALL_DIR="$W/bin2" sh "$SCRIPT" 2>&1) && c=0 || c=$?
if [ "$c" -ne 0 ] && echo "$out" | grep -q 'Linux/armv7l is not supported' && [ ! -e "$W/bin2" ]; then
	ok "arquitetura não suportada falha antes de baixar"
else ko "arquitetura não suportada (exit $c)" "$out"; fi

exit $fail
