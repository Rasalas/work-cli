#!/usr/bin/env bash
set -euo pipefail

repo="Rasalas/work-cli"
binary_name="work"
install_dir="${WORK_INSTALL_DIR:-${HOME}/.local/bin}"
version="${WORK_VERSION:-latest}"

usage() {
  cat <<'EOF'
Usage: install.sh [install|update|uninstall] [--dir DIR] [--version VERSION]

Commands:
  install      Install work from the latest GitHub release
  update       Replace the installed work binary with the latest release
  uninstall    Remove the installed work binary

Options:
  --dir DIR         Installation directory (default: $HOME/.local/bin)
  --version TAG     Release tag to install, for example v0.1.0 (default: latest)
EOF
}

fail() {
  echo "Error: $*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

resolve_platform() {
  case "$(uname -s)" in
    Linux) goos="linux" ;;
    Darwin) goos="darwin" ;;
    MINGW*|MSYS*|CYGWIN*) goos="windows" ;;
    *) fail "unsupported OS: $(uname -s)" ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64) goarch="amd64" ;;
    arm64|aarch64) goarch="arm64" ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
}

resolve_version() {
  if [[ "${version}" != "latest" ]]; then
    return
  fi

  need curl
  version="$(
    curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" |
      sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' |
      head -n 1
  )"
  [[ -n "${version}" ]] || fail "could not resolve latest release"
}

download() {
  local url="$1"
  local target="$2"
  need curl
  curl -fsSL --retry 3 --retry-delay 1 -o "${target}" "${url}"
}

verify_checksum() {
  local archive="$1"
  local checksums="$2"
  local name
  name="$(basename "${archive}")"

  need sha256sum
  (cd "$(dirname "${archive}")" && grep "  ${name}$" "${checksums}" | sha256sum -c -)
}

install_or_update() {
  resolve_platform
  resolve_version

  local archive="work-cli_${version}_${goos}_${goarch}.tar.gz"
  if [[ "${goos}" == "windows" ]]; then
    archive="work-cli_${version}_${goos}_${goarch}.zip"
  fi

  local base_url="https://github.com/${repo}/releases/download/${version}"
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' EXIT

  download "${base_url}/${archive}" "${tmp}/${archive}"
  download "${base_url}/checksums.txt" "${tmp}/checksums.txt"
  verify_checksum "${tmp}/${archive}" "${tmp}/checksums.txt"

  if [[ "${archive}" == *.zip ]]; then
    need unzip
    unzip -q "${tmp}/${archive}" -d "${tmp}/unpack"
  else
    tar -xzf "${tmp}/${archive}" -C "${tmp}"
    mkdir -p "${tmp}/unpack"
    mv "${tmp}/${binary_name}" "${tmp}/unpack/"
  fi

  mkdir -p "${install_dir}"
  install -m 0755 "${tmp}/unpack/${binary_name}" "${install_dir}/${binary_name}"
  echo "Installed ${binary_name} ${version} to ${install_dir}/${binary_name}"
}

uninstall() {
  local target="${install_dir}/${binary_name}"
  if [[ ! -e "${target}" ]]; then
    echo "${target} is not installed"
    return
  fi
  rm -f "${target}"
  echo "Removed ${target}"
}

command="${1:-install}"
if [[ $# -gt 0 ]]; then
  shift
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)
      [[ $# -ge 2 ]] || fail "--dir requires a value"
      install_dir="$2"
      shift 2
      ;;
    --version)
      [[ $# -ge 2 ]] || fail "--version requires a value"
      version="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

case "${command}" in
  install|update)
    install_or_update
    ;;
  uninstall)
    uninstall
    ;;
  -h|--help)
    usage
    ;;
  *)
    fail "unknown command: ${command}"
    ;;
esac
