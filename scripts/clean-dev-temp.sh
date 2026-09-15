#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
temp_dir="${repo_root}/.devtmp"

mkdir -p "${temp_dir}"
find "${temp_dir}" -mindepth 1 -maxdepth 1 ! -name '.gitkeep' -exec rm -rf {} +
