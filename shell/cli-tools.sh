# Ferramentas de linha de comando do terminal-setup.
# Copiado para ~/.config/terminal-setup/cli-tools.sh e carregado no fim do ~/.zshrc
# (ou ~/.bashrc). É sobrescrito a cada "setup.py install": edite a cópia em shell/.

if [ -n "$ZSH_VERSION" ]; then
  _ts_shell=zsh
elif [ -n "$BASH_VERSION" ]; then
  _ts_shell=bash
else
  return 0
fi

_ts_has() { command -v "$1" >/dev/null 2>&1; }

# Homebrew no PATH, caso o shell ainda não tenha carregado.
if ! _ts_has brew; then
  for _ts_brew in /opt/homebrew/bin/brew /usr/local/bin/brew /home/linuxbrew/.linuxbrew/bin/brew; do
    if [ -x "$_ts_brew" ]; then
      eval "$("$_ts_brew" shellenv)"
      break
    fi
  done
  unset _ts_brew
fi

# Completions do zsh (o oh-my-zsh já chama o compinit).
if [ "$_ts_shell" = zsh ] && ! (( $+functions[compdef] )); then
  autoload -Uz compinit && compinit
fi

# ---------------------------------------------------------------- substitutos

if _ts_has eza; then
  alias ls='eza --icons=auto --group-directories-first'
  alias ll='eza -lah --git --icons=auto --group-directories-first'
  alias lt='eza --tree --level=2 --icons=auto --git-ignore'
fi

if _ts_has bat; then
  alias cat='bat --paging=never'
  export MANPAGER="sh -c 'col -bx | bat -l man -p'"
  export MANROFFOPT="-c"
fi

_ts_has btop && alias top='btop'

# ---------------------------------------------------------------- navegação

_ts_has zoxide && eval "$(zoxide init "$_ts_shell")"

if _ts_has fzf; then
  if _ts_has fd; then
    export FZF_DEFAULT_COMMAND='fd --type f --hidden --exclude .git'
    export FZF_CTRL_T_COMMAND="$FZF_DEFAULT_COMMAND"
    export FZF_ALT_C_COMMAND='fd --type d --hidden --exclude .git'
  fi
  _ts_has bat && export FZF_CTRL_T_OPTS="--preview 'bat --color=always --style=numbers --line-range=:300 {}'"
  _ts_has eza && export FZF_ALT_C_OPTS="--preview 'eza --tree --level=2 --icons=always --color=always {}'"
  eval "$(fzf --"$_ts_shell")"
fi

# Depois do fzf: o atuin assume o ctrl+r; o fzf fica com ctrl+t e alt+c.
_ts_has atuin && eval "$(atuin init "$_ts_shell" --disable-up-arrow)"

# yazi: "y" abre o gerenciador e, ao sair, entra na pasta onde você parou.
if _ts_has yazi; then
  y() {
    local tmp cwd
    tmp="$(mktemp -t yazi-cwd.XXXXXX)"
    yazi "$@" --cwd-file="$tmp"
    cwd="$(command cat -- "$tmp")"
    if [ -n "$cwd" ] && [ "$cwd" != "$PWD" ]; then
      builtin cd -- "$cwd"
    fi
    rm -f -- "$tmp"
  }
fi

# ---------------------------------------------------------------- git e dev

_ts_has lazygit && alias lg='lazygit'
_ts_has lazydocker && alias lzd='lazydocker'

_ts_has direnv && eval "$(direnv hook "$_ts_shell")"
_ts_has mise && eval "$(mise activate "$_ts_shell")"

# ---------------------------------------------------------------- prompt

# Só inicia o starship se o rc ainda não tiver feito isso.
if _ts_has starship && [ -z "$STARSHIP_SHELL" ]; then
  eval "$(starship init "$_ts_shell")"
fi

# Plugins do zsh. O syntax-highlighting precisa ser o último.
if [ "$_ts_shell" = zsh ] && [ -n "$HOMEBREW_PREFIX" ]; then
  _ts_zsh_plugins="$HOMEBREW_PREFIX/share"
  [ -f "$_ts_zsh_plugins/zsh-autosuggestions/zsh-autosuggestions.zsh" ] &&
    source "$_ts_zsh_plugins/zsh-autosuggestions/zsh-autosuggestions.zsh"
  [ -f "$_ts_zsh_plugins/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh" ] &&
    source "$_ts_zsh_plugins/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh"
  unset _ts_zsh_plugins
fi

unset _ts_shell
unset -f _ts_has
