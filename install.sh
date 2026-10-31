#!/bin/sh
# Instala o kanivet-cli a partir de uma release pública do GitHub.
#
#   curl -fsSL https://raw.githubusercontent.com/eusener/kanivet-cli/master/install.sh | sh
#
# Variáveis:
#   KANIVET_VERSION      versão a instalar (0.1.0 ou v0.1.0); padrão: a release mais recente
#   KANIVET_INSTALL_DIR  pasta de destino; padrão: ~/.local/bin
#   KANIVET_BASE_URL     de onde baixar (<base>/<tag>/<arquivo>); para testes e mirrors.
#                        Padrão: https://github.com/eusener/kanivet-cli/releases/download
#
# Não usa sudo e não edita arquivos de rc.
set -eu

REPO_URL="https://github.com/eusener/kanivet-cli"
BASE_URL="${KANIVET_BASE_URL:-$REPO_URL/releases/download}"
INSTALL_DIR="${KANIVET_INSTALL_DIR:-$HOME/.local/bin}"

case "${LC_ALL:-${LANG:-}}" in
pt*) PT=1 ;;
*) PT= ;;
esac

# msg <en> <pt>
msg() {
	if [ -n "$PT" ]; then printf '%s\n' "$2"; else printf '%s\n' "$1"; fi
}

die() {
	if [ -n "$PT" ]; then printf 'kanivet-cli: %s\n' "$2" >&2; else printf 'kanivet-cli: %s\n' "$1" >&2; fi
	exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }

# fetch <url> <arquivo>
fetch() {
	if have curl; then
		curl -fsL --retry 3 -o "$2" "$1"
	else
		wget -q --tries=3 -O "$2" "$1"
	fi
}

# effective_url <url>: URL final depois dos redirects.
effective_url() {
	if have curl; then
		curl -fsLI -o /dev/null -w '%{url_effective}' "$1"
	else
		wget --spider -S "$1" 2>&1 | sed -n 's/^ *[Ll]ocation: *//p' | tr -d '\r' | tail -n 1
	fi
}

detect_platform() {
	os=$(uname -s)
	arch=$(uname -m)
	case "$os" in
	Darwin) OS=darwin ;;
	Linux) OS=linux ;;
	*) OS= ;;
	esac
	case "$arch" in
	x86_64 | amd64) ARCH=amd64 ;;
	arm64 | aarch64) ARCH=arm64 ;;
	*) ARCH= ;;
	esac
	if [ -z "$OS" ] || [ -z "$ARCH" ]; then
		die "$os/$arch is not supported (supported: darwin and linux on amd64 or arm64)" \
			"$os/$arch não é suportado (suportados: darwin e linux em amd64 ou arm64)"
	fi
	# Shell x86_64 sob Rosetta num Mac Apple Silicon: prefere o binário nativo.
	if [ "$OS" = darwin ] && [ "$ARCH" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || true)" = 1 ]; then
		ARCH=arm64
	fi
}

resolve_tag() {
	if [ -n "${KANIVET_VERSION:-}" ]; then
		TAG="v${KANIVET_VERSION#v}"
		return
	fi
	url=$(effective_url "$REPO_URL/releases/latest") || url=
	case "$url" in
	*/releases/tag/v*) TAG=${url##*/} ;;
	*) die "could not find the latest release at $REPO_URL/releases" \
		"não foi possível achar a release mais recente em $REPO_URL/releases" ;;
	esac
}

sha256() {
	if have sha256sum; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

main() {
	have curl || have wget || die "curl or wget is required" "é preciso ter curl ou wget"
	have tar || die "tar is required" "é preciso ter tar"
	have sha256sum || have shasum ||
		die "sha256sum or shasum is required to verify the download" \
			"é preciso ter sha256sum ou shasum para conferir o download"

	detect_platform
	resolve_tag
	version=${TAG#v}
	file="kanivet-cli_${version}_${OS}_${ARCH}.tar.gz"

	tmp=$(mktemp -d 2>/dev/null || mktemp -d -t kanivet-cli)
	trap 'rm -rf "$tmp"' EXIT
	trap 'exit 1' HUP INT TERM

	msg "Downloading kanivet-cli $version ($OS/$ARCH)..." "Baixando o kanivet-cli $version ($OS/$ARCH)..."
	fetch "$BASE_URL/$TAG/checksums.txt" "$tmp/checksums.txt" ||
		die "version $TAG not found ($BASE_URL/$TAG)" "a versão $TAG não existe ($BASE_URL/$TAG)"
	fetch "$BASE_URL/$TAG/$file" "$tmp/$file" ||
		die "could not download $file" "não foi possível baixar $file"

	expected=$(awk -v f="$file" '$2 == f || $2 == "*" f {print $1}' "$tmp/checksums.txt")
	[ -n "$expected" ] || die "$file is not listed in checksums.txt" "$file não está no checksums.txt"
	actual=$(sha256 "$tmp/$file")
	[ "$expected" = "$actual" ] ||
		die "checksum mismatch for $file (expected $expected, got $actual)" \
			"checksum de $file não confere (esperado $expected, obtido $actual)"

	mkdir -p "$tmp/x"
	tar -xzf "$tmp/$file" -C "$tmp/x" kanivet-cli
	mkdir -p "$INSTALL_DIR" || die "could not create $INSTALL_DIR" "não foi possível criar $INSTALL_DIR"
	# Copia para o mesmo sistema de arquivos do destino e troca com um rename atômico.
	cp "$tmp/x/kanivet-cli" "$INSTALL_DIR/.kanivet-cli.tmp" ||
		die "cannot write to $INSTALL_DIR" "sem permissão de escrita em $INSTALL_DIR"
	chmod 755 "$INSTALL_DIR/.kanivet-cli.tmp"
	mv -f "$INSTALL_DIR/.kanivet-cli.tmp" "$INSTALL_DIR/kanivet-cli"

	msg "Installed: $("$INSTALL_DIR/kanivet-cli" --version) in $INSTALL_DIR/kanivet-cli" \
		"Instalado: $("$INSTALL_DIR/kanivet-cli" --version) em $INSTALL_DIR/kanivet-cli"

	case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*)
		case "${SHELL:-}" in
		*/zsh) rc="$HOME/.zshrc" ;;
		*/bash) if [ "$OS" = darwin ]; then rc="$HOME/.bash_profile"; else rc="$HOME/.bashrc"; fi ;;
		*) rc="$HOME/.profile" ;;
		esac
		dir=$INSTALL_DIR
		# shellcheck disable=SC2016 # o usuário copia a linha com $HOME e $PATH literais
		[ "$dir" = "$HOME/.local/bin" ] && dir='$HOME/.local/bin'
		msg "" ""
		msg "$INSTALL_DIR is not in your PATH. Add this line to $rc:" \
			"$INSTALL_DIR não está no PATH. Adicione esta linha ao $rc:"
		# shellcheck disable=SC2016
		printf '\n  export PATH="%s:$PATH"\n\n' "$dir"
		;;
	esac
}

main "$@"
