#!/bin/sh
# Liaison CLI installer.
#
# Usage:
#   curl -fsSL https://liaison.cloud/install-cli.sh | sh
#
# Optional environment variables:
#   LIAISON_CLI_VERSION   pin a specific version (default: latest release)
#   LIAISON_CLI_INSTALL_DIR    where to put the binary (default: ~/.local/bin
#                              if writable, else /usr/local/bin via sudo)
#   LIAISON_CLI_RELEASE_BASE   override the release URL base (for staging)
#
# Designed to be safe to pipe into sh: it never sources untrusted code, never
# requires root unless installing into a system path, and verifies the SHA256
# checksum of the downloaded binary against the published SHA256SUMS file.

set -eu

GITHUB_BASE="https://github.com/liaisonio/cli/releases"
CHINA_MIRROR="https://liaison.cloud/releases"
RELEASE_BASE="${LIAISON_CLI_RELEASE_BASE:-${GITHUB_BASE}}"
VERSION="${LIAISON_CLI_VERSION:-latest}"
INSTALL_DIR="${LIAISON_CLI_INSTALL_DIR:-}"
BINARY_NAME="liaison"

# ─── helpers ─────────────────────────────────────────────────────────────────

err() { echo "error: $*" >&2; exit 1; }
info() { echo "→ $*"; }

have() { command -v "$1" >/dev/null 2>&1; }

require() {
  for cmd in "$@"; do
    have "$cmd" || err "required command not found: $cmd"
  done
}

# ─── platform detection ──────────────────────────────────────────────────────

detect_os() {
  case "$(uname -s)" in
    Linux*)   echo "linux" ;;
    Darwin*)  echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) err "unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)   echo "amd64" ;;
    arm64|aarch64)  echo "arm64" ;;
    *) err "unsupported architecture: $(uname -m)" ;;
  esac
}

# ─── install location ────────────────────────────────────────────────────────

resolve_install_dir() {
  if [ -n "${INSTALL_DIR}" ]; then
    # If the user explicitly chose a path, create it. Trust their decision —
    # don't fall through to the sudo branch for an arbitrary user-supplied dir.
    mkdir -p "${INSTALL_DIR}" 2>/dev/null || true
    echo "${INSTALL_DIR}"
    return
  fi
  # Prefer ~/.local/bin if the user can write to it (no sudo needed).
  user_bin="${HOME}/.local/bin"
  if [ -d "${user_bin}" ] && [ -w "${user_bin}" ]; then
    echo "${user_bin}"
    return
  fi
  if mkdir -p "${user_bin}" 2>/dev/null && [ -w "${user_bin}" ]; then
    echo "${user_bin}"
    return
  fi
  echo "/usr/local/bin"
}

needs_sudo() {
  [ "$1" = "/usr/local/bin" ] || [ "$1" = "/usr/bin" ] || [ ! -w "$1" ]
}

# ─── download & verify ───────────────────────────────────────────────────────

download() {
  url="$1"; out="$2"
  if have curl; then
    curl -fsSL "${url}" -o "${out}"
  elif have wget; then
    wget -q "${url}" -O "${out}"
  else
    err "need curl or wget"
  fi
}

resolve_version() {
  if [ "${VERSION}" = "latest" ]; then
    # GitHub redirects /releases/latest to /releases/tag/<vN.N.N>; capture the tag
    if have curl; then
      curl -fsSLI "${RELEASE_BASE}/latest" \
        | awk -F'/' '/^[Ll]ocation:/ { gsub(/[\r\n]+$/, "", $NF); print $NF }' \
        | tail -1
    else
      err "need curl to resolve latest version"
    fi
  else
    echo "${VERSION}"
  fi
}

verify_sha256() {
  file="$1"; expected="$2"
  if have shasum; then
    actual=$(shasum -a 256 "${file}" | awk '{print $1}')
  elif have sha256sum; then
    actual=$(sha256sum "${file}" | awk '{print $1}')
  else
    err "need shasum or sha256sum to verify download"
  fi
  if [ "${actual}" != "${expected}" ]; then
    err "checksum mismatch for ${file} (expected ${expected}, got ${actual})"
  fi
}

# ─── main ────────────────────────────────────────────────────────────────────

main() {
  os=$(detect_os)
  arch=$(detect_arch)
  ext=""
  [ "${os}" = "windows" ] && ext=".exe"

  resolved_version=$(resolve_version)
  [ -n "${resolved_version}" ] || err "could not resolve version (network?)"
  info "version:  ${resolved_version}"
  info "platform: ${os}/${arch}"

  artifact="${BINARY_NAME}-${resolved_version}-${os}-${arch}${ext}"

  tmpdir=$(mktemp -d)
  trap 'rm -rf "${tmpdir}"' EXIT

  # Try liaison.cloud first; on failure try GitHub releases.
  # Users can force a specific source via LIAISON_CLI_RELEASE_BASE.
  fetch_from() {
    _base="$1"; _label="$2"
    _artifact_url="${_base}/download/${resolved_version}/${artifact}"
    _sums_url="${_base}/download/${resolved_version}/SHA256SUMS"

    # China mirror uses a flat layout: /releases/<version>/<file>
    if echo "${_base}" | grep -q 'liaison.cloud'; then
      _artifact_url="${_base}/${resolved_version}/${artifact}"
      _sums_url="${_base}/${resolved_version}/SHA256SUMS"
    fi

    info "[${_label}] fetching ${_artifact_url}"
    download "${_artifact_url}" "${tmpdir}/${artifact}" || return 1

    info "[${_label}] fetching SHA256SUMS"
    download "${_sums_url}" "${tmpdir}/SHA256SUMS" || return 1

    expected=$(awk -v f="${artifact}" '$2 == f { print $1 }' "${tmpdir}/SHA256SUMS")
    [ -n "${expected}" ] || { info "no SHA256 entry for ${artifact}"; return 1; }
    info "[${_label}] verifying SHA256"
    verify_sha256 "${tmpdir}/${artifact}" "${expected}"
  }

  if [ "${RELEASE_BASE}" != "${GITHUB_BASE}" ]; then
    # User overrode the source — use only that.
    fetch_from "${RELEASE_BASE}" "custom" || err "download failed from ${RELEASE_BASE}"
  else
    # Try liaison.cloud first (our own CDN), fall back to GitHub releases.
    fetch_from "${CHINA_MIRROR}" "liaison.cloud" || {
      info "liaison.cloud download failed, trying GitHub releases..."
      fetch_from "${GITHUB_BASE}" "github" || err "download failed from both liaison.cloud and GitHub"
    }
  fi

  install_dir=$(resolve_install_dir)
  info "installing to ${install_dir}/${BINARY_NAME}${ext}"

  chmod +x "${tmpdir}/${artifact}"
  if needs_sudo "${install_dir}"; then
    sudo mv "${tmpdir}/${artifact}" "${install_dir}/${BINARY_NAME}${ext}"
  else
    mv "${tmpdir}/${artifact}" "${install_dir}/${BINARY_NAME}${ext}"
  fi

  # Install the bundled agent skill files into ~/.claude/skills. Best-effort:
  # failure just prints a hint, it doesn't fail the install. Opt out with
  # LIAISON_CLI_SKIP_SKILLS=1.
  if [ "${LIAISON_CLI_SKIP_SKILLS:-0}" != "1" ]; then
    info "installing agent skills to ~/.claude/skills"
    if ! "${install_dir}/${BINARY_NAME}${ext}" skills install -g >/dev/null 2>&1; then
      info "skill install failed — run \`${BINARY_NAME} skills install -g\` manually to retry"
    fi
  fi

  echo
  echo "✓ Liaison CLI installed: ${install_dir}/${BINARY_NAME}${ext}"
  case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *) echo "  (note: ${install_dir} is not in your PATH — add it to your shell rc file)" ;;
  esac
  echo
  echo "Next: liaison login"
}

main "$@"
