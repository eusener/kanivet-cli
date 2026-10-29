#!/usr/bin/env python3
"""Instala e configura kitty + herdr (com plugins e fontes) e ferramentas CLI no macOS ou Linux.

Uso:
    python3 setup.py snapshot                # copia a configuração desta máquina para ./configs
    python3 setup.py install                 # instala tudo e aplica ./configs
    python3 setup.py install --dry-run       # mostra o que seria feito, sem alterar nada
    python3 setup.py install --latest        # plugins na versão mais recente (em vez do commit salvo)
    python3 setup.py install --no-cli-tools  # pula as ferramentas CLI (Homebrew)

Só usa a biblioteca padrão do Python 3.8+.
"""

import argparse
import json
import os
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request
import zipfile
from pathlib import Path

HOME = Path.home()
ROOT = Path(__file__).resolve().parent
CONFIGS = ROOT / "configs"
MANIFEST = CONFIGS / "manifest.json"
HOME_PLACEHOLDER = "{{HOME}}"

KITTY_DIR = HOME / ".config" / "kitty"
HERDR_DIR = HOME / ".config" / "herdr"
LOCAL_BIN = HOME / ".local" / "bin"
TOOLS_DIR = HOME / ".local" / "share" / "terminal-setup"

MIN_GO = (1, 24)
MIN_NODE = 18
NERD_FONT = "JetBrainsMono"
NERD_FONT_CHECK = "JetBrainsMonoNerdFontMono-Regular.ttf"

# Nome do binário de cada agente, para só instalar integrações de agentes presentes.
AGENT_BINARIES = {"cursor": ["cursor-agent", "cursor"], "copilot": ["copilot"]}

# Ferramentas CLI instaladas via Homebrew: (fórmula, binário). Sem binário, confere pelo brew.
CLI_TOOLS = [
    # substitutos dos comandos clássicos
    ("eza", "eza"), ("bat", "bat"), ("fd", "fd"), ("ripgrep", "rg"), ("zoxide", "zoxide"),
    ("dust", "dust"), ("duf", "duf"), ("btop", "btop"), ("procs", "procs"), ("sd", "sd"),
    # navegação e busca
    ("fzf", "fzf"), ("atuin", "atuin"), ("yazi", "yazi"), ("television", "tv"),
    # git
    ("lazygit", "lazygit"), ("git-delta", "delta"), ("gh", "gh"), ("jj", "jj"),
    # shell e prompt
    ("starship", "starship"), ("zsh-autosuggestions", None), ("zsh-syntax-highlighting", None),
    ("direnv", "direnv"), ("mise", "mise"),
    # dados, APIs e dev
    ("jq", "jq"), ("yq", "yq"), ("xh", "xh"), ("just", "just"), ("hyperfine", "hyperfine"),
    ("watchexec", "watchexec"), ("tlrc", "tldr"), ("uv", "uv"),
    ("lazydocker", "lazydocker"), ("k9s", "k9s"),
]
BREW_PATHS = ["/opt/homebrew/bin/brew", "/usr/local/bin/brew", "/home/linuxbrew/.linuxbrew/bin/brew"]
SHELL_TOOLS_SRC = ROOT / "shell" / "cli-tools.sh"
SHELL_TOOLS_DEST = HOME / ".config" / "terminal-setup" / "cli-tools.sh"
GIT_DELTA_CONFIG = [
    ("core.pager", "delta"),
    ("interactive.diffFilter", "delta --color-only"),
    ("delta.navigate", "true"),
    ("merge.conflictStyle", "zdiff3"),
]

DRY_RUN = False
ORIGINAL_PATH = os.environ.get("PATH", "").split(os.pathsep)


# ---------------------------------------------------------------- utilidades

def log(msg):
    print(f"\033[1;36m==>\033[0m {msg}", flush=True)


def warn(msg):
    print(f"\033[1;33m[aviso]\033[0m {msg}", flush=True)


def die(msg):
    print(f"\033[1;31m[erro]\033[0m {msg}", file=sys.stderr)
    sys.exit(1)


def run(cmd, check=True, shell=False, capture=False, cwd=None):
    shown = cmd if isinstance(cmd, str) else " ".join(str(c) for c in cmd)
    if DRY_RUN:
        print(f"    [dry-run] {shown}")
        return subprocess.CompletedProcess(cmd, 0, "", "")
    print(f"    $ {shown}", flush=True)
    return subprocess.run(cmd, check=check, shell=shell, cwd=cwd, text=True,
                          capture_output=capture)


def which(name):
    return shutil.which(name)


def download(url, dest):
    print(f"    baixando {url}", flush=True)
    req = urllib.request.Request(url, headers={"User-Agent": "terminal-setup"})
    with urllib.request.urlopen(req, timeout=120) as resp, open(dest, "wb") as out:
        shutil.copyfileobj(resp, out)


def fetch_json(url):
    req = urllib.request.Request(url, headers={"User-Agent": "terminal-setup"})
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.load(resp)


def detect_os():
    system = platform.system()
    if system == "Darwin":
        return "macos"
    if system == "Linux":
        return "linux"
    die(f"sistema não suportado: {system} (apenas macOS e Linux)")


def detect_arch():
    machine = platform.machine().lower()
    if machine in ("arm64", "aarch64"):
        return "arm64"
    if machine in ("x86_64", "amd64"):
        return "amd64"
    die(f"arquitetura não suportada: {machine}")


def prepend_path(directory):
    directory = str(directory)
    parts = os.environ.get("PATH", "").split(os.pathsep)
    if directory not in parts:
        os.environ["PATH"] = os.pathsep.join([directory] + parts)


def backup(path):
    if path.exists():
        dest = path.with_name(f"{path.name}.bak-setup-{time.strftime('%Y%m%d-%H%M%S')}")
        log(f"backup: {path} -> {dest.name}")
        if not DRY_RUN:
            shutil.copy2(path, dest)


def shell_rc():
    shell = os.path.basename(os.environ.get("SHELL", ""))
    return HOME / (".zshrc" if shell == "zsh" else ".bashrc")


def ensure_shell_path(dirs):
    """Garante os diretórios no PATH do shell de login (para o herdr achar node/go depois)."""
    rc = shell_rc()
    marker = "# >>> terminal-setup >>>"
    content = rc.read_text() if rc.exists() else ""
    # Só os diretórios que existem (ex.: go/node só se o script precisou instalá-los).
    dirs = [d for d in dirs if Path(d).is_dir() and str(d) not in ORIGINAL_PATH]
    if marker in content or not dirs:
        return
    lines = [f'export PATH="{d}:$PATH"' for d in dirs]
    block = f"\n{marker}\n" + "\n".join(lines) + "\n# <<< terminal-setup <<<\n"
    log(f"adicionando {', '.join(str(d) for d in dirs)} ao PATH em {rc}")
    if not DRY_RUN:
        with open(rc, "a") as f:
            f.write(block)


# ---------------------------------------------------------------- dependências

def linux_package_install(packages):
    managers = [
        ("apt-get", ["sudo", "apt-get", "install", "-y"], ["sudo", "apt-get", "update"]),
        ("dnf", ["sudo", "dnf", "install", "-y"], None),
        ("pacman", ["sudo", "pacman", "-S", "--needed", "--noconfirm"], None),
        ("zypper", ["sudo", "zypper", "install", "-y"], None),
    ]
    for name, install_cmd, update_cmd in managers:
        if which(name):
            if update_cmd:
                run(update_cmd)
            run(install_cmd + packages)
            return
    die(f"nenhum gerenciador de pacotes conhecido; instale manualmente: {' '.join(packages)}")


def ensure_base_tools(os_name):
    log("verificando curl, git, tar e unzip")
    missing = [t for t in ("curl", "git", "tar", "unzip") if not which(t)]
    if os_name == "linux" and not which("fc-cache"):
        missing.append("fontconfig")
    if not missing:
        return
    if os_name == "macos":
        if "git" in missing:
            run(["xcode-select", "--install"], check=False)
            die("instale as Command Line Tools do Xcode (janela aberta) e rode o script de novo")
        die(f"ferramentas ausentes: {' '.join(missing)}")
    linux_package_install(missing)


def version_tuple(text):
    return tuple(int(n) for n in re.findall(r"\d+", text)[:3])


def go_ok():
    if not which("go"):
        return False
    out = subprocess.run(["go", "version"], capture_output=True, text=True).stdout
    match = re.search(r"go(\d+\.\d+(?:\.\d+)?)", out)
    return bool(match) and version_tuple(match.group(1))[:2] >= MIN_GO


def node_ok():
    if not which("node"):
        return False
    out = subprocess.run(["node", "--version"], capture_output=True, text=True).stdout
    return bool(out) and version_tuple(out)[0] >= MIN_NODE


def install_go(os_name, arch):
    if go_ok():
        log("go já instalado")
        return
    log("instalando go (tarball oficial)")
    if DRY_RUN:
        print("    [dry-run] baixar go mais recente para", TOOLS_DIR / "go")
        return
    version = fetch_json("https://go.dev/dl/?mode=json")[0]["version"]
    goos = "darwin" if os_name == "macos" else "linux"
    url = f"https://go.dev/dl/{version}.{goos}-{arch}.tar.gz"
    with tempfile.TemporaryDirectory() as tmp:
        archive = Path(tmp) / "go.tar.gz"
        download(url, archive)
        shutil.rmtree(TOOLS_DIR / "go", ignore_errors=True)
        TOOLS_DIR.mkdir(parents=True, exist_ok=True)
        with tarfile.open(archive) as tar:
            tar.extractall(TOOLS_DIR)
    prepend_path(TOOLS_DIR / "go" / "bin")


def install_node(os_name, arch):
    if node_ok():
        log("node já instalado")
        return
    log("instalando node LTS (tarball oficial)")
    if DRY_RUN:
        print("    [dry-run] baixar node LTS para", TOOLS_DIR / "node")
        return
    release = next(r for r in fetch_json("https://nodejs.org/dist/index.json") if r["lts"])
    version = release["version"]
    nos = "darwin" if os_name == "macos" else "linux"
    narch = "x64" if arch == "amd64" else "arm64"
    name = f"node-{version}-{nos}-{narch}"
    with tempfile.TemporaryDirectory() as tmp:
        archive = Path(tmp) / "node.tar.gz"
        download(f"https://nodejs.org/dist/{version}/{name}.tar.gz", archive)
        with tarfile.open(archive) as tar:
            tar.extractall(tmp)
        shutil.rmtree(TOOLS_DIR / "node", ignore_errors=True)
        TOOLS_DIR.mkdir(parents=True, exist_ok=True)
        shutil.move(str(Path(tmp) / name), str(TOOLS_DIR / "node"))
    prepend_path(TOOLS_DIR / "node" / "bin")


# ---------------------------------------------------------------- fontes

def font_dir(os_name):
    if os_name == "macos":
        return HOME / "Library" / "Fonts"
    return Path(os.environ.get("XDG_DATA_HOME", HOME / ".local" / "share")) / "fonts"


def install_nerd_font(os_name):
    target = font_dir(os_name)
    if (target / NERD_FONT_CHECK).exists():
        log(f"{NERD_FONT} Nerd Font já instalada")
        return
    log(f"instalando {NERD_FONT} Nerd Font em {target}")
    if DRY_RUN:
        print(f"    [dry-run] baixar {NERD_FONT}.zip do nerd-fonts e extrair os .ttf")
        return
    url = f"https://github.com/ryanoasis/nerd-fonts/releases/latest/download/{NERD_FONT}.zip"
    target.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory() as tmp:
        archive = Path(tmp) / "font.zip"
        download(url, archive)
        with zipfile.ZipFile(archive) as zf:
            for member in zf.namelist():
                if member.lower().endswith(".ttf") and "/" not in member:
                    (target / member).write_bytes(zf.read(member))
    if os_name == "linux":
        run(["fc-cache", "-f", str(target)], check=False)


def install_icon_font(manifest):
    """Fonte de ícones dos agentes, distribuída pelo plugin herdr-radar."""
    if not any(p["id"] == "hhdebb.herdr-radar" for p in manifest["plugins"]):
        return
    log("instalando fonte de ícones do herdr-radar")
    result = run(["herdr", "plugin", "action", "invoke", "hhdebb.herdr-radar.install-font"],
                 check=False)
    if result.returncode == 0:
        return
    # Sem servidor do herdr rodando, executa o instalador direto do checkout do plugin.
    root = installed_plugin_roots().get("hhdebb.herdr-radar")
    if root:
        run(["node", "bin/install-font.js"], check=False, cwd=root)
    else:
        warn("não achei o checkout do herdr-radar; rode depois: "
             "herdr plugin action invoke hhdebb.herdr-radar.install-font")


# ---------------------------------------------------------------- kitty e herdr

def kitty_installed(os_name):
    if which("kitty"):
        return True
    if os_name == "macos":
        return any(p.exists() for p in (Path("/Applications/kitty.app"),
                                        HOME / "Applications" / "kitty.app"))
    return (HOME / ".local" / "kitty.app").exists()


def install_kitty(os_name):
    if kitty_installed(os_name):
        log("kitty já instalado")
        return
    log("instalando kitty (instalador oficial)")
    run("curl -L https://sw.kovidgoyal.net/kitty/installer.sh | sh /dev/stdin launch=n", shell=True)
    if os_name == "linux":
        app = HOME / ".local" / "kitty.app"
        if not DRY_RUN:
            LOCAL_BIN.mkdir(parents=True, exist_ok=True)
            for binary in ("kitty", "kitten"):
                link = LOCAL_BIN / binary
                if link.is_symlink() or link.exists():
                    link.unlink()
                link.symlink_to(app / "bin" / binary)
            # Atalhos no menu de aplicativos.
            apps = HOME / ".local" / "share" / "applications"
            apps.mkdir(parents=True, exist_ok=True)
            for desktop in ("kitty.desktop", "kitty-open.desktop"):
                src = app / "share" / "applications" / desktop
                if src.exists():
                    text = src.read_text()
                    text = text.replace("Icon=kitty", f"Icon={app}/share/icons/hicolor/256x256/apps/kitty.png")
                    text = re.sub(r"^Exec=kitty", f"Exec={app}/bin/kitty", text, flags=re.M)
                    (apps / desktop).write_text(text)


def install_herdr():
    if which("herdr"):
        log("herdr já instalado")
        return
    log("instalando herdr")
    run("curl -fsSL https://herdr.dev/install.sh | sh", shell=True)
    prepend_path(LOCAL_BIN)
    if not DRY_RUN and not which("herdr"):
        die("herdr não ficou no PATH após a instalação; abra um novo terminal e rode de novo")


def installed_plugin_roots():
    path = HERDR_DIR / "plugins.json"
    if not path.exists():
        return {}
    return {p["plugin_id"]: p.get("plugin_root") for p in json.loads(path.read_text())}


def install_plugins(manifest, latest):
    installed = installed_plugin_roots()
    for plugin in manifest["plugins"]:
        pid = plugin["id"]
        if pid in installed:
            log(f"plugin {pid} já instalado")
        else:
            log(f"instalando plugin {pid}")
            spec = f"{plugin['owner']}/{plugin['repo']}"
            if plugin.get("subdir"):
                spec += f"/{plugin['subdir']}"
            cmd = ["herdr", "plugin", "install", spec, "--yes"]
            if not latest and plugin.get("commit"):
                cmd += ["--ref", plugin["commit"]]
            if run(cmd, check=False).returncode != 0:
                warn(f"falha ao instalar {pid}")
                continue
        state = "enable" if plugin.get("enabled", True) else "disable"
        run(["herdr", "plugin", state, pid], check=False, capture=True)


def install_integrations(manifest):
    for name in manifest.get("integrations", []):
        binaries = AGENT_BINARIES.get(name, [name])
        if not any(which(b) for b in binaries):
            log(f"integração {name}: agente não encontrado, pulando")
            continue
        log(f"instalando integração {name}")
        run(["herdr", "integration", "install", name], check=False)


# ---------------------------------------------------------------- ferramentas CLI

def find_brew():
    return which("brew") or next((p for p in BREW_PATHS if os.access(p, os.X_OK)), None)


def install_brew():
    brew = find_brew()
    if brew:
        log("Homebrew já instalado")
    else:
        log("instalando Homebrew (instalador oficial; pode pedir a senha do sudo)")
        run('/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"',
            shell=True, check=False)
        brew = find_brew()
        if not brew:
            if not DRY_RUN:
                warn("Homebrew não foi instalado; pulando as ferramentas CLI")
            return None
    prepend_path(Path(brew).parent)
    return brew


def brew_formulas(brew):
    out = subprocess.run([brew, "list", "--formula", "-1"], capture_output=True, text=True).stdout
    return set(out.split())


def missing_cli_tools(brew):
    installed = brew_formulas(brew) if brew else set()
    return [f for f, binary in CLI_TOOLS
            if f not in installed and not (binary and which(binary))]


def install_cli_tools():
    brew = install_brew()
    if not brew and not DRY_RUN:
        return
    missing = missing_cli_tools(brew)
    if not missing:
        log("ferramentas CLI já instaladas")
    else:
        log(f"instalando ferramentas CLI: {', '.join(missing)}")
        run([brew or "brew", "install", *missing], check=False)
        if not DRY_RUN:
            failed = missing_cli_tools(brew)
            if failed:
                warn(f"não foi possível instalar: {', '.join(failed)}")
    configure_git_delta()
    install_shell_tools()


def configure_git_delta():
    if not DRY_RUN and not which("delta"):
        return
    current = subprocess.run(["git", "config", "--global", "--get", "core.pager"],
                             capture_output=True, text=True).stdout.strip()
    if current:
        log(f"git já usa core.pager={current}; mantendo")
        return
    log("configurando o delta como pager do git")
    for key, value in GIT_DELTA_CONFIG:
        run(["git", "config", "--global", key, value])


def install_shell_tools():
    """Copia os hooks e aliases das ferramentas e carrega o arquivo no fim do rc do shell."""
    log(f"aplicando {SHELL_TOOLS_DEST}")
    if not DRY_RUN:
        SHELL_TOOLS_DEST.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(SHELL_TOOLS_SRC, SHELL_TOOLS_DEST)
    rc = shell_rc()
    marker = "# >>> terminal-setup cli-tools >>>"
    content = rc.read_text() if rc.exists() else ""
    if marker in content:
        return
    dest = str(SHELL_TOOLS_DEST).replace(str(HOME), "$HOME")
    block = (f"\n{marker}\n"
             f'[ -f "{dest}" ] && . "{dest}"\n'
             "# <<< terminal-setup cli-tools <<<\n")
    log(f"carregando {SHELL_TOOLS_DEST.name} no fim de {rc}")
    if not DRY_RUN:
        with open(rc, "a") as f:
            f.write(block)


# ---------------------------------------------------------------- configs

def apply_file(src, dest):
    backup(dest)
    log(f"aplicando {dest}")
    if DRY_RUN:
        return
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text(src.read_text().replace(HOME_PLACEHOLDER, str(HOME)))


def apply_configs():
    for src in sorted((CONFIGS / "kitty").glob("*.conf")):
        apply_file(src, KITTY_DIR / src.name)
    apply_file(CONFIGS / "herdr" / "config.toml", HERDR_DIR / "config.toml")
    plugin_configs = CONFIGS / "herdr" / "plugins-config"
    for src in sorted(p for p in plugin_configs.rglob("*") if p.is_file()):
        apply_file(src, HERDR_DIR / "plugins" / "config" / src.relative_to(plugin_configs))


def is_config_file(path):
    name = path.name
    return path.is_file() and ".bak" not in name and not name.endswith((".log", ".sock"))


def copy_snapshot(src, dest):
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text(src.read_text().replace(str(HOME), HOME_PLACEHOLDER))


def snapshot():
    log(f"salvando configuração atual em {CONFIGS}")
    shutil.rmtree(CONFIGS, ignore_errors=True)

    for src in [KITTY_DIR / "kitty.conf", *KITTY_DIR.glob("*.auto.conf")]:
        if is_config_file(src):
            copy_snapshot(src, CONFIGS / "kitty" / src.name)

    copy_snapshot(HERDR_DIR / "config.toml", CONFIGS / "herdr" / "config.toml")
    plugin_configs = HERDR_DIR / "plugins" / "config"
    for src in plugin_configs.rglob("*"):
        if is_config_file(src):
            copy_snapshot(src, CONFIGS / "herdr" / "plugins-config" / src.relative_to(plugin_configs))

    plugins = []
    for p in json.loads((HERDR_DIR / "plugins.json").read_text()):
        source = p.get("source", {})
        if source.get("kind") != "github":
            warn(f"plugin {p['plugin_id']} não vem do GitHub; não será reinstalado")
            continue
        plugins.append({
            "id": p["plugin_id"],
            "owner": source["owner"],
            "repo": source["repo"],
            "subdir": source.get("subdir"),
            "commit": source.get("resolved_commit"),
            "enabled": p.get("enabled", True),
        })

    status = subprocess.run(["herdr", "integration", "status"], capture_output=True, text=True).stdout
    integrations = [m.group(1) for m in re.finditer(r"^(\S+): current", status, re.M)]

    MANIFEST.write_text(json.dumps({"plugins": plugins, "integrations": integrations}, indent=2) + "\n")
    log(f"{len(plugins)} plugins e {len(integrations)} integrações registrados em {MANIFEST.name}")


# ---------------------------------------------------------------- main

def install(args):
    if not MANIFEST.exists():
        die(f"{MANIFEST} não existe; rode 'python3 setup.py snapshot' na máquina de origem")
    manifest = json.loads(MANIFEST.read_text())

    os_name, arch = detect_os(), detect_arch()
    log(f"sistema: {os_name} ({arch})")
    prepend_path(LOCAL_BIN)
    prepend_path(TOOLS_DIR / "go" / "bin")
    prepend_path(TOOLS_DIR / "node" / "bin")

    ensure_base_tools(os_name)
    install_go(os_name, arch)
    install_node(os_name, arch)
    ensure_shell_path([LOCAL_BIN, TOOLS_DIR / "go" / "bin", TOOLS_DIR / "node" / "bin"])

    install_nerd_font(os_name)
    install_kitty(os_name)
    install_herdr()

    # Plugins antes das configs: o build de alguns plugins mexe no config.toml.
    install_plugins(manifest, args.latest)
    apply_configs()
    install_icon_font(manifest)
    install_integrations(manifest)

    run(["herdr", "server", "reload-config"], check=False, capture=True)

    if not args.no_cli_tools:
        install_cli_tools()
    log("pronto! Abra o kitty (reinicie se já estava aberto) e rode 'herdr'.")


def main():
    global DRY_RUN
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command")
    sub.add_parser("snapshot", help="salva a configuração desta máquina em ./configs")
    inst = sub.add_parser("install", help="instala tudo e aplica ./configs (padrão)")
    inst.add_argument("--dry-run", action="store_true", help="só mostra o que seria feito")
    inst.add_argument("--latest", action="store_true", help="instala plugins na versão mais recente")
    inst.add_argument("--no-cli-tools", action="store_true",
                      help="não instala as ferramentas CLI (Homebrew, eza, fzf, lazygit...)")
    args = parser.parse_args()

    if args.command == "snapshot":
        snapshot()
        return
    if args.command is None:
        args = inst.parse_args([])
    DRY_RUN = args.dry_run
    install(args)


if __name__ == "__main__":
    main()
