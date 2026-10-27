#!/bin/sh
# Install pf (go-port-forward) on Linux or macOS:
#
#   curl -fsSL https://raw.githubusercontent.com/alinemone/go-port-forward/main/install.sh | sh
#
# Options (environment variables):
#   PF_VERSION      release tag to install, e.g. v3.2.1 (default: latest)
#   PF_INSTALL_DIR  where to put pf (default: /usr/local/bin if writable, else ~/.local/bin)
#
# The binary is checked against the release's SHA256SUMS.txt before it is
# installed. Nothing needs root unless you choose a root-owned directory.

set -eu

REPO="alinemone/go-port-forward"
VERSION="${PF_VERSION:-latest}"

say()  { printf '%s\n' "$*"; }
fail() { printf 'pf install: %s\n' "$*" >&2; exit 1; }

# -- platform ----------------------------------------------------------------
case "$(uname -s)" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) fail "unsupported OS $(uname -s) - on Windows use install.ps1 (see the README)" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64)  arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) fail "unsupported CPU $(uname -m) - pf is built for amd64 and arm64" ;;
esac
asset="pf-${os}-${arch}"

if [ "$VERSION" = latest ]; then
  base="https://github.com/${REPO}/releases/latest/download"
else
  base="https://github.com/${REPO}/releases/download/${VERSION}"
fi

# -- download ----------------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL --retry 3 -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -O "$2" "$1"; }
else
  fail "needs curl or wget"
fi

tmp="$(mktemp -d 2>/dev/null || mktemp -d -t pf-install)"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "-> downloading ${asset} (${VERSION})"
fetch "${base}/${asset}" "${tmp}/pf" || fail "download failed: ${base}/${asset}"
fetch "${base}/SHA256SUMS.txt" "${tmp}/SHA256SUMS.txt" || fail "could not download SHA256SUMS.txt"

# -- verify ------------------------------------------------------------------
expected="$(awk -v f="$asset" '$2 == f { print $1 }' "${tmp}/SHA256SUMS.txt")"
[ -n "$expected" ] || fail "${asset} is not listed in SHA256SUMS.txt"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "${tmp}/pf" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "${tmp}/pf" | awk '{ print $1 }')"
else
  fail "needs sha256sum or shasum to verify the download"
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for ${asset} - not installing (expected ${expected}, got ${actual})"
say "[ok] checksum verified"

# -- install -----------------------------------------------------------------
if [ -n "${PF_INSTALL_DIR:-}" ]; then
  dir="$PF_INSTALL_DIR"
elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
  dir=/usr/local/bin
else
  dir="${HOME}/.local/bin"
fi
mkdir -p "$dir" || fail "cannot create ${dir}"
[ -w "$dir" ] || fail "${dir} is not writable - set PF_INSTALL_DIR to a directory you own"

chmod +x "${tmp}/pf"
# Move into place via a temp name so a running pf is replaced atomically.
cp "${tmp}/pf" "${dir}/.pf.new" && mv -f "${dir}/.pf.new" "${dir}/pf" || fail "could not write ${dir}/pf"
if [ "$os" = darwin ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "${dir}/pf" 2>/dev/null || true
fi

say "[ok] installed ${dir}/pf"
"${dir}/pf" version 2>/dev/null || true

# -- PATH --------------------------------------------------------------------
case ":${PATH}:" in
  *":${dir}:"*)
    found="$(command -v pf 2>/dev/null || true)"
    if [ -n "$found" ] && [ "$found" != "${dir}/pf" ]; then
      say ""
      say "! another pf comes first on your PATH: ${found}"
      say "  remove it, or put ${dir} earlier in PATH"
    else
      say ""
      say "Run: pf"
    fi
    ;;
  *)
    case "${SHELL:-}" in
      */zsh)  rc="~/.zshrc" ;;
      */bash) rc="~/.bashrc" ;;
      *)      rc="your shell's startup file" ;;
    esac
    say ""
    say "! ${dir} is not on your PATH. Add it once:"
    say "    echo 'export PATH=\"${dir}:\$PATH\"' >> ${rc}"
    say "  then open a new terminal and run: pf"
    ;;
esac
