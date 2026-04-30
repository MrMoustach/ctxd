#!/bin/sh
set -eu

repo="MrMoustach/ctxd"
bin_name="ctxd"
install_dir="${CTXD_INSTALL_DIR:-}"

info() {
  printf '%s\n' "$*"
}

fail() {
  printf 'ctxd install: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

detect_os() {
  case "$(uname -s)" in
    Linux) printf 'Linux' ;;
    Darwin) printf 'Darwin' ;;
    *) fail "unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) printf 'x86_64' ;;
    arm64 | aarch64) printf 'arm64' ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
}

latest_tag() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "https://api.github.com/repos/${repo}/releases/latest" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1
  else
    fail "missing required command: curl or wget"
  fi
}

download() {
  url="$1"
  dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
  else
    wget -q "$url" -O "$dest"
  fi
}

choose_install_dir() {
  if [ -n "$install_dir" ]; then
    printf '%s' "$install_dir"
    return
  fi

  if [ -w "/usr/local/bin" ]; then
    printf '/usr/local/bin'
    return
  fi

  printf '%s/.local/bin' "$HOME"
}

verify_checksum() {
  archive="$1"
  checksums="$2"
  archive_name="$(basename "$archive")"

  if command -v sha256sum >/dev/null 2>&1; then
    expected="$(grep "  ${archive_name}$" "$checksums" | awk '{print $1}')"
    actual="$(sha256sum "$archive" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    expected="$(grep "  ${archive_name}$" "$checksums" | awk '{print $1}')"
    actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
  else
    info "sha256sum/shasum not found; skipping checksum verification"
    return
  fi

  [ -n "$expected" ] || fail "checksum not found for ${archive_name}"
  [ "$expected" = "$actual" ] || fail "checksum mismatch for ${archive_name}"
}

need tar
os="$(detect_os)"
arch="$(detect_arch)"
tag="${CTXD_VERSION:-$(latest_tag)}"
[ -n "$tag" ] || fail "could not resolve latest release tag"

asset="ctxd_${os}_${arch}.tar.gz"
base_url="https://github.com/${repo}/releases/download/${tag}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

archive="${tmp_dir}/${asset}"
checksums="${tmp_dir}/checksums.txt"

info "Downloading ${asset} from ${repo} ${tag}"
download "${base_url}/${asset}" "$archive"
download "${base_url}/checksums.txt" "$checksums"
verify_checksum "$archive" "$checksums"

tar -xzf "$archive" -C "$tmp_dir"

target_dir="$(choose_install_dir)"
mkdir -p "$target_dir"

if [ -w "$target_dir" ]; then
  install "$tmp_dir/$bin_name" "$target_dir/$bin_name"
else
  command -v sudo >/dev/null 2>&1 || fail "${target_dir} is not writable and sudo is not available"
  sudo install "$tmp_dir/$bin_name" "$target_dir/$bin_name"
fi

info "Installed ctxd to ${target_dir}/${bin_name}"
case ":$PATH:" in
  *":$target_dir:"*) ;;
  *) info "Add ${target_dir} to PATH to run ctxd from any shell." ;;
esac
info "Run: ctxd doctor"
