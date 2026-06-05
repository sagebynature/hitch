#!/bin/sh
set -eu

HITCH_REPO_URL=${HITCH_REPO_URL:-https://github.com/sagebynature/hitch.git}
HITCH_REF=${HITCH_REF:-main}
HITCH_INSTALL_DIR=${HITCH_INSTALL_DIR:-$HOME/.local/bin}
HITCH_SOURCE_DIR=${HITCH_SOURCE_DIR:-}
HITCH_SKIP_HOOK_INSTALL=${HITCH_SKIP_HOOK_INSTALL:-0}
HITCH_URL=${HITCH_URL:-}
HITCH_INSTALL_MODE=${HITCH_INSTALL_MODE:-}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'error: %s is required to install Hitch.\n' "$1" >&2
    exit 1
  fi
}

path_is_first() {
  cmd_name=$1
  cmd_path=$(command -v "$cmd_name" 2>/dev/null || true)
  [ "$cmd_path" = "$HITCH_INSTALL_DIR/$cmd_name" ]
}

tty_available() {
  [ -c /dev/tty ] || return 1
  (: < /dev/tty) >/dev/null 2>&1
}

prompt_tty() {
  answer=
  printf '%s' "$1" > /dev/tty || return 1
  IFS= read -r answer < /dev/tty || return 1
  return 0
}

normalize_install_mode() {
  case "$1" in
    "") printf 'all' ;;
    all|server|client) printf '%s' "$1" ;;
    *) return 1 ;;
  esac
}

select_install_mode() {
  if [ -n "$HITCH_INSTALL_MODE" ]; then
    if selected_mode=$(normalize_install_mode "$HITCH_INSTALL_MODE"); then
      HITCH_INSTALL_MODE=$selected_mode
      return 0
    fi
    printf 'Invalid HITCH_INSTALL_MODE: %s (expected all, server, or client).\n' "$HITCH_INSTALL_MODE" >&2
    exit 1
  fi

  if tty_available; then
    while :; do
      if ! prompt_tty "Install Hitch mode [all/server/client] (all): "; then
        printf 'Install cancelled.\n' > /dev/tty 2>/dev/null || printf 'Install cancelled.\n' >&2
        exit 1
      fi
      if selected_mode=$(normalize_install_mode "$answer"); then
        HITCH_INSTALL_MODE=$selected_mode
        return 0
      fi
      printf 'Please enter all, server, or client.\n' > /dev/tty
    done
  fi

  HITCH_INSTALL_MODE=all
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

server_url_command() {
  current_shell=$(basename "${SHELL:-sh}")
  case "$current_shell" in
    fish) printf 'set -gx HITCH_URL "%s"' "$HITCH_URL" ;;
    *) printf 'export HITCH_URL="%s"' "$HITCH_URL" ;;
  esac
}

hook_setup_command() {
  if [ -n "$HITCH_URL" ]; then
    printf '%s/hitch-client install --url '\''%s'\''' "$HITCH_INSTALL_DIR" "$HITCH_URL"
  else
    printf '%s/hitch-client install' "$HITCH_INSTALL_DIR"
  fi
}

maybe_update_server_url() {
  if [ -z "$HITCH_URL" ] || [ "$HITCH_URL" = "http://127.0.0.1:8799" ]; then
    return 0
  fi

  command_text=$(server_url_command)
  config_file=$(shell_config_file)
  if tty_available && prompt_tty "Persist HITCH_URL to $config_file for future harness launches? [Y/n] "; then
    case "$answer" in
      n|N|no|NO) ;;
      *)
        mkdir -p "$(dirname "$config_file")"
        touch "$config_file"
        if ! grep -Fxq "$command_text" "$config_file" 2>/dev/null; then
          printf '\n# Hitch\n%s\n' "$command_text" >> "$config_file"
          printf 'Added HITCH_URL to %s.\n' "$config_file"
        fi
        ;;
    esac
  fi
}

configure_server_url() {
  default_url=${HITCH_URL:-http://127.0.0.1:8799}
  if tty_available && prompt_tty "Hitch server URL [$default_url]: "; then
    if [ -n "$answer" ]; then
      HITCH_URL=$answer
    else
      HITCH_URL=$default_url
    fi
    export HITCH_URL
    maybe_update_server_url
  elif [ -n "$HITCH_URL" ]; then
    export HITCH_URL
  fi

}

maybe_update_path() {
  path_binary=$1
  installed_names=$2
  if path_is_first "$path_binary"; then
    return 0
  fi

  command_text=$(path_command)
  config_file=$(shell_config_file)
  printf '\nInstalled %s into %s, but that directory is not first on PATH.\n' "$installed_names" "$HITCH_INSTALL_DIR"
  if tty_available && prompt_tty "Add it to $config_file now? [Y/n] "; then
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
  select_install_mode
  install_hitch=0
  install_client=0
  init_config=0
  configure_url=0
  setup_hooks=0
  path_binary=hitch
  installed_names='Hitch'
  case "$HITCH_INSTALL_MODE" in
    all)
      install_hitch=1
      install_client=1
      init_config=1
      configure_url=1
      setup_hooks=1
      path_binary=hitch
      installed_names='Hitch server and client'
      ;;
    server)
      install_hitch=1
      init_config=1
      path_binary=hitch
      installed_names='Hitch server'
      ;;
    client)
      install_client=1
      configure_url=1
      setup_hooks=1
      path_binary=hitch-client
      installed_names='Hitch client'
      ;;
  esac

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
  if [ "$install_hitch" = 1 ]; then
    (cd "$src_dir" && go build -o "$tmp_dir/hitch" ./cmd/hitch)
  fi
  if [ "$install_client" = 1 ]; then
    (cd "$src_dir" && go build -o "$tmp_dir/hitch-client" ./cmd/hitch-client)
  fi

  mkdir -p "$HITCH_INSTALL_DIR"
  if [ "$install_hitch" = 1 ]; then
    cp "$tmp_dir/hitch" "$HITCH_INSTALL_DIR/hitch"
    chmod 755 "$HITCH_INSTALL_DIR/hitch"
    "$HITCH_INSTALL_DIR/hitch" --version
  fi
  if [ "$install_client" = 1 ]; then
    cp "$tmp_dir/hitch-client" "$HITCH_INSTALL_DIR/hitch-client"
    chmod 755 "$HITCH_INSTALL_DIR/hitch-client"
    "$HITCH_INSTALL_DIR/hitch-client" --version
  fi
  if [ "$init_config" = 1 ]; then
    "$HITCH_INSTALL_DIR/hitch" config init --json
  fi

  case "$HITCH_INSTALL_MODE" in
    all) printf 'Installed Hitch to %s/hitch and %s/hitch-client.\n' "$HITCH_INSTALL_DIR" "$HITCH_INSTALL_DIR" ;;
    server) printf 'Installed Hitch server to %s/hitch.\n' "$HITCH_INSTALL_DIR" ;;
    client) printf 'Installed Hitch client to %s/hitch-client.\n' "$HITCH_INSTALL_DIR" ;;
  esac
  maybe_update_path "$path_binary" "$installed_names"

  if [ "$configure_url" = 1 ]; then
    configure_server_url
  fi

  if [ "$setup_hooks" != 1 ] || [ "$HITCH_SKIP_HOOK_INSTALL" = 1 ]; then
    return 0
  fi

  if tty_available; then
    "$HITCH_INSTALL_DIR/hitch-client" install < /dev/tty
  else
    printf 'Run hook setup with:\n\n  %s\n\n' "$(hook_setup_command)"
  fi
}

main "$@"
