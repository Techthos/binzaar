#!/usr/bin/env bash
#
# binzaar installer.
#
#   curl -fsSL https://raw.githubusercontent.com/Techthos/binzaar/main/scripts/install.sh | bash
#
# Detects the host OS/arch, resolves the latest GitHub release (or a pinned
# version), downloads the matching binary, verifies its SHA-256 against the
# release's `.sha256` sidecar, and installs it as `store` into
# binzaar's managed install directory — alongside the micro-apps it installs
# — so the store can later update itself from its own catalog
# (`binzaar install Techthos/binzaar` overwrites this same file).
#
# Environment overrides:
#   BINZAAR_VERSION       release tag to install (default: latest, e.g. v0.2.0)
#   BINZAAR_INSTALL_DIR   target directory     (default: ~/.local/share/binzaar/bin)
#   BINZAAR_REPO          owner/name           (default: Techthos/binzaar)
#   BINZAAR_GITHUB_TOKEN  token for private repos / higher API rate limits
#                            (GITHUB_TOKEN is also honored)
#
set -euo pipefail

REPO="${BINZAAR_REPO:-Techthos/binzaar}"
BIN="binzaar"            # release asset base name
PLACED="store"          # installed filename (matches the catalog's "bin": "store")
VERSION="${BINZAAR_VERSION:-latest}"
INSTALL_DIR="${BINZAAR_INSTALL_DIR:-${HOME}/.local/share/binzaar/bin}"
TOKEN="${BINZAAR_GITHUB_TOKEN:-${GITHUB_TOKEN:-}}"

# ---- output helpers (everything diagnostic goes to stderr) -------------------
if [ -t 2 ]; then
  C_RESET=$'\033[0m'; C_DIM=$'\033[2m'; C_BLUE=$'\033[34m'
  C_YELLOW=$'\033[33m'; C_RED=$'\033[31m'; C_GREEN=$'\033[32m'
else
  C_RESET=''; C_DIM=''; C_BLUE=''; C_YELLOW=''; C_RED=''; C_GREEN=''
fi
info() { printf '%s==>%s %s\n' "$C_BLUE" "$C_RESET" "$*" >&2; }
warn() { printf '%swarning:%s %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
die()  { printf '%serror:%s %s\n' "$C_RED" "$C_RESET" "$*" >&2; exit 1; }

# ---- preflight --------------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
  DL="curl"
elif command -v wget >/dev/null 2>&1; then
  DL="wget"
else
  die "need either curl or wget installed"
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  die "need either sha256sum or shasum to verify the download"
fi

# fetch URL -> stdout. Adds auth header when a token is present.
fetch() {
  local url="$1"
  if [ "$DL" = "curl" ]; then
    if [ -n "$TOKEN" ]; then
      curl -fsSL -H "Authorization: Bearer ${TOKEN}" "$url"
    else
      curl -fsSL "$url"
    fi
  else
    if [ -n "$TOKEN" ]; then
      wget -qO- --header="Authorization: Bearer ${TOKEN}" "$url"
    else
      wget -qO- "$url"
    fi
  fi
}

# fetch URL -> file path.
fetch_to() {
  local url="$1" out="$2"
  if [ "$DL" = "curl" ]; then
    if [ -n "$TOKEN" ]; then
      curl -fsSL -H "Authorization: Bearer ${TOKEN}" -o "$out" "$url"
    else
      curl -fsSL -o "$out" "$url"
    fi
  else
    if [ -n "$TOKEN" ]; then
      wget -q --header="Authorization: Bearer ${TOKEN}" -O "$out" "$url"
    else
      wget -q -O "$out" "$url"
    fi
  fi
}

# ---- detect host OS / arch (matches GOOS/GOARCH the release matrix builds) ---
os_raw="$(uname -s)"
case "$os_raw" in
  Linux)  GOOS="linux" ;;
  Darwin) GOOS="darwin" ;;
  *) die "unsupported OS '${os_raw}'. Released targets: linux, darwin (Windows users: download the .exe from the Releases page)." ;;
esac

arch_raw="$(uname -m)"
case "$arch_raw" in
  x86_64|amd64)  GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) die "unsupported architecture '${arch_raw}'. Released targets: amd64, arm64." ;;
esac

# ---- resolve the release tag ------------------------------------------------
if [ "$VERSION" = "latest" ]; then
  info "Resolving latest release of ${REPO}…"
  api_json="$(fetch "https://api.github.com/repos/${REPO}/releases/latest")" \
    || die "could not query the GitHub API for ${REPO} (is the repo private? set BINZAAR_GITHUB_TOKEN)"
  # Split on commas so each JSON field is isolated on its own line; this keeps
  # the extraction correct for both pretty-printed and minified API responses.
  TAG="$(printf '%s' "$api_json" \
    | tr ',' '\n' \
    | grep -m1 '"tag_name"' \
    | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')"
  [ -n "$TAG" ] || die "no published release found for ${REPO} yet."
else
  TAG="$VERSION"
fi

ASSET="${BIN}-${TAG}-${GOOS}-${GOARCH}"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

info "Installing ${BIN} ${TAG} (${GOOS}/${GOARCH}) as ${PLACED}"

# ---- download + verify in a self-cleaning temp dir --------------------------
TMP="$(mktemp -d "${TMPDIR:-/tmp}/binzaar-install.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

info "Downloading ${ASSET}…"
fetch_to "${BASE_URL}/${ASSET}" "${TMP}/${ASSET}" \
  || die "download failed: ${BASE_URL}/${ASSET}
The release may not include a ${GOOS}/${GOARCH} build, or the tag '${TAG}' does not exist."

info "Verifying SHA-256…"
if fetch_to "${BASE_URL}/${ASSET}.sha256" "${TMP}/${ASSET}.sha256" 2>/dev/null; then
  want="$(cut -d' ' -f1 < "${TMP}/${ASSET}.sha256")"
  got="$(sha256 "${TMP}/${ASSET}")"
  if [ "$want" != "$got" ]; then
    die "checksum mismatch for ${ASSET}
  expected: ${want}
  actual:   ${got}"
  fi
  info "Checksum OK (${got})"
else
  warn "no ${ASSET}.sha256 sidecar published — skipping checksum verification."
fi

# ---- install ----------------------------------------------------------------
mkdir -p "$INSTALL_DIR" || die "cannot create install directory: ${INSTALL_DIR}"
DEST="${INSTALL_DIR}/${PLACED}"
if command -v install >/dev/null 2>&1; then
  install -m 0755 "${TMP}/${ASSET}" "$DEST" || die "failed to install to ${DEST} (need write permission?)"
else
  mv "${TMP}/${ASSET}" "$DEST" && chmod 0755 "$DEST" || die "failed to install to ${DEST}"
fi

info "${C_GREEN}Installed${C_RESET} ${PLACED} ${TAG} → ${DEST}"

# ---- PATH check -------------------------------------------------------------
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*)
    info "Run ${C_DIM}${PLACED}${C_RESET} to get started."
    ;;
  *)
    warn "${INSTALL_DIR} is not on your PATH."
    case "${SHELL##*/}" in
      zsh)  rc="~/.zshrc" ;;
      fish) rc="~/.config/fish/config.fish" ;;
      *)    rc="~/.bashrc" ;;
    esac
    printf '       Add it with:\n\n         echo '\''export PATH="%s:$PATH"'\'' >> %s\n\n       Or run it directly: %s\n' \
      "$INSTALL_DIR" "$rc" "$DEST" >&2
    ;;
esac
