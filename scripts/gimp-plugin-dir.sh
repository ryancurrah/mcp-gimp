#!/usr/bin/env bash
# Prints the plug-ins directory of the newest installed GIMP 3.x.
#
# GIMP's per-user config directory is named after its major.minor version and
# a fresh one is created on each minor upgrade, so this path moves when GIMP
# is updated. The active path is shown in Preferences > Folders > Plug-ins.
set -euo pipefail

case "$(uname -s)" in
  Darwin) base="$HOME/Library/Application Support/GIMP" ;;
  *)
    if [ -d "$HOME/snap/gimp/current/.config/GIMP" ]; then
      base="$HOME/snap/gimp/current/.config/GIMP"
    else
      base="${XDG_CONFIG_HOME:-$HOME/.config}/GIMP"
    fi
    ;;
esac

if [ ! -d "$base" ]; then
  echo "no GIMP config directory found at $base" >&2
  exit 1
fi

version="$(find "$base" -maxdepth 1 -name '3.*' -type d -exec basename {} \; | sort -V | tail -1)"

if [ -z "$version" ]; then
  echo "no GIMP 3.x config directory under $base" >&2
  exit 1
fi

printf '%s/%s/plug-ins\n' "$base" "$version"
