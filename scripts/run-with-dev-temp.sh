#!/usr/bin/env bash
set -euo pipefail

if (( $# == 0 )); then
    printf '%s\n' "usage: scripts/run-with-dev-temp.sh <command> [args...]" >&2
    exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
temp_dir="${repo_root}/.devtmp"

mkdir -p "${temp_dir}"

cleanup() {
    "${repo_root}/scripts/clean-dev-temp.sh"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP

export TMPDIR="${temp_dir}/"
export TMP="${TMPDIR}"
export TEMP="${TMPDIR}"

"$@"
