#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCAL_BIN_DIR="${HOME}/.local/bin"
APP_INSTALL_DIR="${HOME}/Applications"
APP_NAME="Terminal Agent.app"
APP_PATH="${APP_INSTALL_DIR}/${APP_NAME}"
GUI_BIN_PATH="${LOCAL_BIN_DIR}/agent-gui"
PLIST_TEMPLATE="${REPO_ROOT}/packaging/macos/Info.plist"
ICON_SOURCE_PATH="${REPO_ROOT}/assets/icon.png"
SHORTCUT_LABEL="Ctrl+Shift+Space"

log() {
  printf '[terminal-agent] %s\n' "$1"
}

fail() {
  printf '[terminal-agent] ERROR: %s\n' "$1" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "Missing required command: $1"
}

usage() {
  cat <<'EOF'
Usage: scripts/integ_macos.sh [--uninstall]

Installs or removes the macOS desktop integration for Terminal Agent popup GUI.
EOF
}

ensure_macos() {
  [[ "$(uname -s)" == "Darwin" ]] || fail "This integration script is intended for macOS."
}

ensure_build_deps() {
  if ! xcode-select -p >/dev/null 2>&1; then
    log "Xcode Command Line Tools not found."
    log "Install them by running:  xcode-select --install"
    fail "Xcode Command Line Tools are required to build the GUI."
  fi
  log "Xcode Command Line Tools found at $(xcode-select -p)"
  need_cmd go
}

build_gui_binary() {
  log "Building agent-gui"
  mkdir -p "${REPO_ROOT}/bin"
  go build -o "${REPO_ROOT}/bin/agent-gui" "${REPO_ROOT}/cmd/agent-gui/main.go"
}

create_iconset() {
  local iconset_dir="${REPO_ROOT}/bin/terminal-agent.iconset"
  rm -rf "${iconset_dir}"
  mkdir -p "${iconset_dir}"

  [[ -f "${ICON_SOURCE_PATH}" ]] || fail "Missing icon asset at ${ICON_SOURCE_PATH}"

  local sizes=(16 32 128 256 512)
  local size retina_size
  for size in "${sizes[@]}"; do
    retina_size=$((size * 2))
    sips -z "${size}" "${size}" "${ICON_SOURCE_PATH}" --out "${iconset_dir}/icon_${size}x${size}.png" >/dev/null 2>&1
    sips -z "${retina_size}" "${retina_size}" "${ICON_SOURCE_PATH}" --out "${iconset_dir}/icon_${size}x${size}@2x.png" >/dev/null 2>&1
  done

  iconutil -c icns "${iconset_dir}" -o "${REPO_ROOT}/bin/terminal-agent.icns"
  rm -rf "${iconset_dir}"
  log "Generated terminal-agent.icns from assets/icon.png"
}

create_app_bundle() {
  local bundle_dir="${REPO_ROOT}/bin/${APP_NAME}"
  rm -rf "${bundle_dir}"
  mkdir -p "${bundle_dir}/Contents/MacOS"
  mkdir -p "${bundle_dir}/Contents/Resources"

  [[ -x "${REPO_ROOT}/bin/agent-gui" ]] || fail "Expected built binary at ${REPO_ROOT}/bin/agent-gui"
  cp "${REPO_ROOT}/bin/agent-gui" "${bundle_dir}/Contents/MacOS/agent-gui"
  chmod 0755 "${bundle_dir}/Contents/MacOS/agent-gui"

  [[ -f "${REPO_ROOT}/bin/terminal-agent.icns" ]] || fail "Expected icon at ${REPO_ROOT}/bin/terminal-agent.icns"
  cp "${REPO_ROOT}/bin/terminal-agent.icns" "${bundle_dir}/Contents/Resources/terminal-agent.icns"

  [[ -f "${PLIST_TEMPLATE}" ]] || fail "Missing Info.plist template at ${PLIST_TEMPLATE}"
  local version
  version="$(git -C "${REPO_ROOT}" describe --tags --always --dirty 2>/dev/null || echo "dev")"
  sed "s/__VERSION__/${version}/g" "${PLIST_TEMPLATE}" > "${bundle_dir}/Contents/Info.plist"

  log "Created app bundle at ${bundle_dir}"
}

install_app_bundle() {
  mkdir -p "${APP_INSTALL_DIR}"
  if [[ -d "${APP_PATH}" ]]; then
    rm -rf "${APP_PATH}"
    log "Removed previous installation at ${APP_PATH}"
  fi
  cp -R "${REPO_ROOT}/bin/${APP_NAME}" "${APP_PATH}"
  log "Installed app bundle to ${APP_PATH}"
}

install_cli_symlink() {
  mkdir -p "${LOCAL_BIN_DIR}"
  ln -sf "${APP_PATH}/Contents/MacOS/agent-gui" "${GUI_BIN_PATH}"
  log "Created CLI symlink at ${GUI_BIN_PATH}"
}

register_with_launch_services() {
  local lsregister="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
  if [[ -x "${lsregister}" ]]; then
    "${lsregister}" -f "${APP_PATH}" >/dev/null 2>&1 || true
    log "Registered app with Launch Services for Spotlight indexing"
  fi
}

verify_installation() {
  [[ -d "${APP_PATH}" ]] || fail "App bundle was not installed"
  [[ -x "${APP_PATH}/Contents/MacOS/agent-gui" ]] || fail "App binary is missing or not executable"
  [[ -f "${APP_PATH}/Contents/Info.plist" ]] || fail "Info.plist is missing from app bundle"
  [[ -f "${APP_PATH}/Contents/Resources/terminal-agent.icns" ]] || fail "App icon is missing from app bundle"
  [[ -L "${GUI_BIN_PATH}" ]] || fail "CLI symlink was not created"
  plutil -lint "${APP_PATH}/Contents/Info.plist" >/dev/null 2>&1 || fail "Info.plist validation failed"
  "${GUI_BIN_PATH}" --help >/dev/null 2>&1 || fail "Installed GUI binary is not runnable"
  log "Verified app bundle and CLI symlink"
}

uninstall_integration() {
  rm -rf "${APP_PATH}"
  rm -f "${GUI_BIN_PATH}"
  log "Removed macOS desktop integration files"
}

print_shortcut_instructions() {
  log ""
  log "To set up a global keyboard shortcut (${SHORTCUT_LABEL}):"
  log ""
  log "  Option A: Shortcuts.app (macOS 13+)"
  log "    1. Open Shortcuts.app"
  log "    2. Create a new shortcut"
  log "    3. Add a 'Run Shell Script' action with: ${GUI_BIN_PATH} --show"
  log "    4. Name it 'Terminal Agent Popup'"
  log "    5. Right-click the shortcut (or open its details)"
  log "    6. Click 'Add Keyboard Shortcut' and press ${SHORTCUT_LABEL}"
  log ""
  log "  Option B: Automator Quick Action"
  log "    1. Open Automator and create a new Quick Action"
  log "    2. Add 'Run Shell Script' with: ${GUI_BIN_PATH} --show"
  log "    3. Save as 'Terminal Agent Popup'"
  log "    4. Go to System Settings > Keyboard > Keyboard Shortcuts > Services"
  log "    5. Find 'Terminal Agent Popup' and assign ${SHORTCUT_LABEL}"
}

print_summary() {
  log ""
  log "macOS desktop integration complete"
  log "App bundle installed: ${APP_PATH}"
  log "CLI symlink: ${GUI_BIN_PATH}"
  log "Use 'agent-gui' for a normal launch or 'agent-gui --show' to reopen the popup"
}

main() {
  local uninstall="false"
  case "${1:-}" in
    --uninstall)
      uninstall="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    "")
      ;;
    *)
      usage
      fail "Unknown argument: ${1}"
      ;;
  esac

  [[ $# -eq 0 ]] || fail "Unexpected extra arguments"

  ensure_macos
  if [[ "${uninstall}" == "true" ]]; then
    uninstall_integration
    return
  fi

  ensure_build_deps
  build_gui_binary
  create_iconset
  create_app_bundle
  install_app_bundle
  install_cli_symlink
  register_with_launch_services
  verify_installation
  print_shortcut_instructions
  print_summary
}

main "$@"
