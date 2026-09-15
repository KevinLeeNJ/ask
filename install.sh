#!/bin/sh
set -eu

REPO="KevinLeeNJ/ask"
INSTALL_DIR="${ASK_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${ASK_VERSION:-}"

usage() {
	cat <<'EOF'
Install the latest ask release.

Usage:
  install.sh [--install-dir DIR] [--version TAG]

Environment:
  ASK_INSTALL_DIR  Installation directory (default: $HOME/.local/bin)
  ASK_VERSION      Release tag to install (default: latest)
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--install-dir)
			[ "$#" -ge 2 ] || {
				echo "install.sh: --install-dir requires a value" >&2
				exit 2
			}
			INSTALL_DIR="$2"
			shift 2
			;;
		--install-dir=*)
			INSTALL_DIR="${1#*=}"
			shift
			;;
		--version)
			[ "$#" -ge 2 ] || {
				echo "install.sh: --version requires a value" >&2
				exit 2
			}
			VERSION="$2"
			shift 2
			;;
		--version=*)
			VERSION="${1#*=}"
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			echo "install.sh: unknown argument: $1" >&2
			usage >&2
			exit 2
			;;
	esac
done

if command -v curl >/dev/null 2>&1; then
	download_file() {
		curl -fsSL "$1" -o "$2"
	}
	download_stdout() {
		curl -fsSL "$1"
	}
elif command -v wget >/dev/null 2>&1; then
	download_file() {
		wget -qO "$2" "$1"
	}
	download_stdout() {
		wget -qO- "$1"
	}
else
	echo "install.sh: curl or wget is required" >&2
	exit 1
fi

case "$(uname -s)" in
	Darwin) OS="darwin" ;;
	Linux) OS="linux" ;;
	MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
	*)
		echo "install.sh: unsupported operating system: $(uname -s)" >&2
		exit 1
		;;
esac

case "$(uname -m)" in
	x86_64|amd64) ARCH="amd64" ;;
	arm64|aarch64) ARCH="arm64" ;;
	*)
		echo "install.sh: unsupported architecture: $(uname -m)" >&2
		exit 1
		;;
esac

if [ -z "$VERSION" ]; then
	VERSION="$(
		download_stdout "https://api.github.com/repos/${REPO}/releases/latest" |
			sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' |
			head -n 1
	)"
fi

if [ -z "$VERSION" ]; then
	echo "install.sh: could not determine the latest release version" >&2
	exit 1
fi

ARCHIVE_VERSION="${VERSION#v}"
if [ "$OS" = "windows" ]; then
	ARCHIVE_EXT="zip"
	BINARY_NAME="ask.exe"
else
	ARCHIVE_EXT="tar.gz"
	BINARY_NAME="ask"
fi
ASSET="ask_${ARCHIVE_VERSION}_${OS}_${ARCH}.${ARCHIVE_EXT}"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/ask-install.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT HUP INT TERM

ARCHIVE_PATH="${TMP_DIR}/${ASSET}"
CHECKSUM_PATH="${TMP_DIR}/checksums.txt"

echo "Downloading ask ${VERSION} for ${OS}/${ARCH}..." >&2
download_file "${BASE_URL}/${ASSET}" "$ARCHIVE_PATH"
download_file "${BASE_URL}/checksums.txt" "$CHECKSUM_PATH"

if command -v sha256sum >/dev/null 2>&1; then
	ACTUAL_CHECKSUM="$(sha256sum "$ARCHIVE_PATH" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
	ACTUAL_CHECKSUM="$(shasum -a 256 "$ARCHIVE_PATH" | awk '{print $1}')"
else
	echo "install.sh: sha256sum or shasum is required" >&2
	exit 1
fi
EXPECTED_CHECKSUM="$(
	awk -v asset="$ASSET" '$2 == asset { print $1; exit }' "$CHECKSUM_PATH"
)"

if [ -z "$EXPECTED_CHECKSUM" ]; then
	echo "install.sh: checksum for ${ASSET} was not found" >&2
	exit 1
fi
if [ "$ACTUAL_CHECKSUM" != "$EXPECTED_CHECKSUM" ]; then
	echo "install.sh: checksum verification failed for ${ASSET}" >&2
	exit 1
fi

EXTRACT_DIR="${TMP_DIR}/extract"
mkdir -p "$EXTRACT_DIR"
case "$ARCHIVE_EXT" in
	tar.gz)
		tar -xzf "$ARCHIVE_PATH" -C "$EXTRACT_DIR"
		;;
	zip)
		if ! command -v unzip >/dev/null 2>&1; then
			echo "install.sh: unzip is required on Windows" >&2
			exit 1
		fi
		unzip -q "$ARCHIVE_PATH" -d "$EXTRACT_DIR"
		;;
esac

SOURCE_BINARY="${EXTRACT_DIR}/${BINARY_NAME}"
if [ ! -f "$SOURCE_BINARY" ]; then
	echo "install.sh: ${BINARY_NAME} was not found in ${ASSET}" >&2
	exit 1
fi

mkdir -p "$INSTALL_DIR"
TARGET="${INSTALL_DIR}/${BINARY_NAME}"
cp "$SOURCE_BINARY" "$TARGET"
chmod 0755 "$TARGET"

echo "Installed ask ${VERSION} to ${TARGET}"
case ":${PATH}:" in
	*":${INSTALL_DIR}:"*) ;;
	*)
		echo "Add ${INSTALL_DIR} to PATH:"
		echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
		;;
esac
