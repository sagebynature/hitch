#!/bin/sh
set -eu

HITCH_REPO_URL=${HITCH_REPO_URL:-https://github.com/sagebynature/hitch.git}
HITCH_REF=${HITCH_REF:-main}
HITCH_INSTALL_DIR=${HITCH_INSTALL_DIR:-$HOME/.local/bin}
HITCH_SOURCE_DIR=${HITCH_SOURCE_DIR:-}
HITCH_SKIP_HOOK_INSTALL=${HITCH_SKIP_HOOK_INSTALL:-0}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'error: %s is required to install Hitch.\n' "$1" >&2
    exit 1
  fi
}

path_is_first() {
  cmd_path=$(command -v hitch 2>/dev/null || true)
  [ "$cmd_path" = "$HITCH_INSTALL_DIR/hitch" ]
}

shell_config_file() {
  current_shell=$(basename "${SHELL:-sh}")
  case "$current_shell" in
    fish) printf '%s/.config/fish/config.fish' "$HOME" ;;
    zsh) printf '%s/.zshrc' "${ZDOTDIR:-$HOME}" ;;
    bash)
      if [ -f "$HOME/.bashrc" ]; then
        printf '%s/.bashrc' "$HOME"
      else
        printf '%s/.profile' "$HOME"
      fi
      ;;
    *) printf '%s/.profile' "$HOME" ;;
  esac
}

path_command() {
  current_shell=$(basename "${SHELL:-sh}")
  if [ "$HITCH_INSTALL_DIR" = "$HOME/.local/bin" ]; then
    bin_expr='${HOME}/.local/bin'
  else
    bin_expr=$HITCH_INSTALL_DIR
  fi
  case "$current_shell" in
    fish) printf 'fish_add_path "%s"' "$bin_expr" ;;
    *) printf 'export PATH="%s:$PATH"' "$bin_expr" ;;
  esac
}

maybe_update_path() {
  if path_is_first; then
    return 0
  fi

  command_text=$(path_command)
  config_file=$(shell_config_file)
  printf '\nHitch was installed at %s/hitch, but that directory is not first on PATH.\n' "$HITCH_INSTALL_DIR"
  if [ -t 0 ]; then
    printf 'Add it to %s now? [Y/n] ' "$config_file"
    IFS= read -r answer || answer=
    case "$answer" in
      n|N|no|NO) ;;
      *)
        mkdir -p "$(dirname "$config_file")"
        touch "$config_file"
        if ! grep -Fxq "$command_text" "$config_file" 2>/dev/null; then
          printf '\n# Hitch\n%s\n' "$command_text" >> "$config_file"
          printf 'Added PATH update to %s.\n' "$config_file"
        fi
        ;;
    esac
  fi
  printf 'Restart your shell or run:\n\n  %s\n\n' "$command_text"
}

main() {
  require_command go
  if [ -z "$HITCH_SOURCE_DIR" ]; then
    require_command git
  fi

  tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/hitch-install.XXXXXX")
  trap 'rm -rf "$tmp_dir"' EXIT INT TERM

  if [ -n "$HITCH_SOURCE_DIR" ]; then
    src_dir=$HITCH_SOURCE_DIR
  else
    src_dir=$tmp_dir/src
    git clone --depth 1 --branch "$HITCH_REF" "$HITCH_REPO_URL" "$src_dir"
  fi

  printf 'Building Hitch from %s...\n' "$src_dir"
  (cd "$src_dir" && go build -o "$tmp_dir/hitch" ./cmd/hitch)
  (cd "$src_dir" && go build -o "$tmp_dir/hitch-client" ./cmd/hitch-client)

  mkdir -p "$HITCH_INSTALL_DIR"
  cp "$tmp_dir/hitch" "$HITCH_INSTALL_DIR/hitch"
  cp "$tmp_dir/hitch-client" "$HITCH_INSTALL_DIR/hitch-client"
  chmod 755 "$HITCH_INSTALL_DIR/hitch" "$HITCH_INSTALL_DIR/hitch-client"

  "$HITCH_INSTALL_DIR/hitch" --version
  "$HITCH_INSTALL_DIR/hitch-client" --version
  printf 'Installed Hitch to %s/hitch and %s/hitch-client.\n' "$HITCH_INSTALL_DIR" "$HITCH_INSTALL_DIR"
  maybe_update_path

  if [ "$HITCH_SKIP_HOOK_INSTALL" = 1 ]; then
    return 0
  fi

  if [ -t 0 ]; then
    "$HITCH_INSTALL_DIR/hitch" install < /dev/tty
  else
    printf 'Run hook setup with:\n\n  %s/hitch install\n\n' "$HITCH_INSTALL_DIR"
  fi
}

main "$@"
