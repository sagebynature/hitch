#!/bin/sh
set -eu

: "${HOME:=/var/lib/hitch}"
: "${HITCH_CONFIG:=$HOME/.config/hitch/config.toml}"

has_config_arg() {
	for arg in "$@"; do
		case "$arg" in
			--config|--config=*) return 0 ;;
		esac
	done
	return 1
}

seed_config() {
	mkdir -p "$(dirname "$HITCH_CONFIG")" "$HOME/.config/hitch/extensions" "$HOME/.config/hitch/backups"
	if [ ! -e "$HITCH_CONFIG" ]; then
		hitch config init --path "$HITCH_CONFIG" >/dev/null
	fi
}

if [ "$#" -eq 0 ]; then
	set -- serve
fi

if [ "$1" = "hitch" ]; then
	shift
fi

case "$1" in
	serve)
		seed_config
		if has_config_arg "$@"; then
			exec hitch "$@"
		fi
		exec hitch "$@" --config "$HITCH_CONFIG"
		;;
	-*)
		seed_config
		if has_config_arg "$@"; then
			exec hitch serve "$@"
		fi
		exec hitch serve "$@" --config "$HITCH_CONFIG"
		;;
	*)
		exec hitch "$@"
		;;
esac
