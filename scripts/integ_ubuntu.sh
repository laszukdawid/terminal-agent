#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCAL_BIN_DIR="${HOME}/.local/bin"
LOCAL_APP_DIR="${HOME}/.local/share/applications"
LOCAL_ICON_DIR="${HOME}/.local/share/icons/hicolor/256x256/apps"
GUI_BIN_PATH="${LOCAL_BIN_DIR}/agent-gui"
DESKTOP_FILE_PATH="${LOCAL_APP_DIR}/terminal-agent-gui.desktop"
ICON_PATH="${LOCAL_ICON_DIR}/terminal-agent.png"
DESKTOP_TEMPLATE_PATH="${REPO_ROOT}/packaging/linux/terminal-agent-gui.desktop"
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

detect_desktop() {
  local current desktop session
  current="${XDG_CURRENT_DESKTOP:-}"
  desktop="${DESKTOP_SESSION:-}"
  session="${XDG_SESSION_DESKTOP:-}"
  case "${current}:${desktop}:${session}" in
    *GNOME*|*gnome*)
      printf 'gnome\n'
      ;;
    *KDE*|*Plasma*|*plasma*)
      printf 'kde\n'
      ;;
    *)
      printf 'unknown\n'
      ;;
  esac
}

usage() {
  cat <<'EOF'
Usage: scripts/integ_ubuntu.sh [--uninstall]

Installs or removes the Ubuntu desktop integration for Terminal Agent popup GUI.
EOF
}

ensure_dirs() {
  mkdir -p "${LOCAL_BIN_DIR}" "${LOCAL_APP_DIR}" "${LOCAL_ICON_DIR}"
}

ensure_ubuntu() {
  need_cmd apt-get
  if [[ -r /etc/os-release ]]; then
    if ! grep -Eiq '^ID="?ubuntu"?$' /etc/os-release; then
      fail "This integration script is intended for Ubuntu."
    fi
  fi
}

ensure_build_deps() {
  local packages=()
  local pkg
  for pkg in libgl1-mesa-dev xorg-dev libx11-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev desktop-file-utils; do
    if ! dpkg -s "$pkg" >/dev/null 2>&1; then
      packages+=("$pkg")
    fi
  done

  if ((${#packages[@]} > 0)); then
    log "Installing missing GUI build dependencies via apt-get"
    sudo apt-get update
    sudo apt-get install -y "${packages[@]}"
  else
    log "Required GUI build dependencies already installed"
  fi
}

build_gui_binary() {
  need_cmd go
  log "Building agent-gui"
  mkdir -p "${REPO_ROOT}/bin"
  go build -o "${REPO_ROOT}/bin/agent-gui" "${REPO_ROOT}/cmd/agent-gui/main.go"
}

install_gui_binary() {
  [[ -x "${REPO_ROOT}/bin/agent-gui" ]] || fail "Expected built binary at ${REPO_ROOT}/bin/agent-gui"
  install -m 0755 "${REPO_ROOT}/bin/agent-gui" "${GUI_BIN_PATH}"
  log "Installed agent-gui to ${GUI_BIN_PATH}"
}

install_icon() {
  [[ -f "${ICON_SOURCE_PATH}" ]] || fail "Missing icon asset at ${ICON_SOURCE_PATH}"
  install -m 0644 "${ICON_SOURCE_PATH}" "${ICON_PATH}"
  log "Installed icon to ${ICON_PATH}"
}

install_desktop_entry() {
  [[ -f "${DESKTOP_TEMPLATE_PATH}" ]] || fail "Missing desktop entry template at ${DESKTOP_TEMPLATE_PATH}"
  sed \
    -e "s|__AGENT_GUI_PATH__|${GUI_BIN_PATH}|g" \
    -e "s|__ICON_PATH__|${ICON_PATH}|g" \
    "${DESKTOP_TEMPLATE_PATH}" > "${DESKTOP_FILE_PATH}"
  chmod 0644 "${DESKTOP_FILE_PATH}"
  log "Installed desktop entry to ${DESKTOP_FILE_PATH}"
}

refresh_desktop_database() {
  if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "${LOCAL_APP_DIR}" >/dev/null 2>&1 || true
    log "Refreshed desktop application database"
  fi
  if command -v kbuildsycoca6 >/dev/null 2>&1; then
    kbuildsycoca6 --noincremental >/dev/null 2>&1 || true
    log "Rebuilt KDE service cache"
  fi
}

reload_kde_shortcuts() {
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user restart plasma-kglobalaccel.service >/dev/null 2>&1 || true
  fi
  if command -v dbus-send >/dev/null 2>&1; then
    dbus-send --session --dest=org.kde.kglobalaccel --print-reply /kglobalaccel org.kde.KGlobalAccel.reloadConfig >/dev/null 2>&1 || true
  fi
  log "Requested KDE global shortcut reload"
}

configure_gnome_shortcut() {
  need_cmd gsettings

  local shortcut_path="/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/terminal-agent-gui/"
  local current_list raw_list new_list escaped_command
  escaped_command="${GUI_BIN_PATH} --show"
  current_list="$(gsettings get org.gnome.settings-daemon.plugins.media-keys custom-keybindings)"

  if [[ "${current_list}" != *"${shortcut_path}"* ]]; then
    raw_list="${current_list#[}"
    raw_list="${raw_list%]}"
    if [[ -n "${raw_list// }" ]]; then
      new_list="[${raw_list}, '${shortcut_path}']"
    else
      new_list="['${shortcut_path}']"
    fi
    gsettings set org.gnome.settings-daemon.plugins.media-keys custom-keybindings "${new_list}"
  fi

  gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:${shortcut_path} name 'Terminal Agent Popup'
  gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:${shortcut_path} command "${escaped_command}"
  gsettings set org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:${shortcut_path} binding '<Primary><Shift>space'

  log "Configured GNOME shortcut ${SHORTCUT_LABEL} for agent-gui --show"
}

configure_kde_shortcut() {
  if ! command -v kwriteconfig6 >/dev/null 2>&1; then
    log "KDE shortcut automation is not enabled; use Plasma Shortcuts to bind the discovered 'Terminal Agent Popup' entry"
    return 0
  fi

  log "KDE detected: open Plasma Shortcuts and bind the discovered 'Terminal Agent Popup' entry"
  log "If the application entry is missing, fall back to a custom command shortcut: ${GUI_BIN_PATH} --show"
}

remove_gnome_shortcut() {
  if ! command -v gsettings >/dev/null 2>&1; then
    return 0
  fi

  local shortcut_path="/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/terminal-agent-gui/"
  local current_list raw_list new_list
  current_list="$(gsettings get org.gnome.settings-daemon.plugins.media-keys custom-keybindings 2>/dev/null || printf '[]')"
  if [[ "${current_list}" != *"${shortcut_path}"* ]]; then
    return 0
  fi

  raw_list="${current_list#[}"
  raw_list="${raw_list%]}"
  new_list="$(printf '%s' "${raw_list}" | sed "s#'${shortcut_path}', \|, '${shortcut_path}'\|'${shortcut_path}'##g")"
  if [[ -n "${new_list// }" ]]; then
    gsettings set org.gnome.settings-daemon.plugins.media-keys custom-keybindings "[${new_list}]"
  else
    gsettings set org.gnome.settings-daemon.plugins.media-keys custom-keybindings "[]"
  fi
  log "Removed GNOME shortcut configuration for Terminal Agent Popup"
}

remove_kde_shortcut() {
  if ! command -v kwriteconfig6 >/dev/null 2>&1; then
    return 0
  fi

  log "No KDE shortcut automation to remove"
}

uninstall_integration() {
  local desktop
  desktop="$(detect_desktop)"
  case "${desktop}" in
    gnome)
      remove_gnome_shortcut
      ;;
    kde)
      remove_kde_shortcut
      ;;
  esac

  rm -f "${DESKTOP_FILE_PATH}" "${ICON_PATH}" "${GUI_BIN_PATH}"
  refresh_desktop_database
  log "Removed Ubuntu desktop integration files"
}

verify_installation() {
  [[ -x "${GUI_BIN_PATH}" ]] || fail "Installed GUI binary is missing or not executable"
  [[ -f "${DESKTOP_FILE_PATH}" ]] || fail "Desktop entry was not installed"
  grep -q "Exec=${GUI_BIN_PATH} --show" "${DESKTOP_FILE_PATH}" || fail "Desktop entry Exec line is incorrect"
  grep -q "Icon=${ICON_PATH}" "${DESKTOP_FILE_PATH}" || fail "Desktop entry Icon line is incorrect"
  desktop-file-validate "${DESKTOP_FILE_PATH}" >/dev/null 2>&1 || fail "desktop-file-validate reported an invalid desktop entry"
  "${GUI_BIN_PATH}" --help >/dev/null 2>&1 || fail "Installed GUI binary is not runnable"
  log "Verified GUI binary and desktop entry"
}

verify_kde_shortcut() {
  log "KDE verification: check that Plasma Shortcuts shows 'Terminal Agent Popup', then bind ${SHORTCUT_LABEL} there"
}

print_summary() {
  local desktop="$1"
  log "Ubuntu desktop integration complete"
  log "Desktop detected: ${desktop}"
  log "Launcher installed: ${DESKTOP_FILE_PATH}"
  log "Binary installed: ${GUI_BIN_PATH}"
  log "Use 'agent-gui' for a normal launch or '${GUI_BIN_PATH} --show' to reopen the popup"
  case "${desktop}" in
    gnome)
      log "Shortcut configured: ${SHORTCUT_LABEL}"
      ;;
    kde)
      log "Next step on KDE: bind ${SHORTCUT_LABEL} to the discovered 'Terminal Agent Popup' shortcut entry"
      ;;
    *)
      log "Suggested shortcut: ${SHORTCUT_LABEL}"
      ;;
  esac
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

  ensure_ubuntu
  if [[ "${uninstall}" == "true" ]]; then
    uninstall_integration
    return
  fi

  ensure_dirs
  ensure_build_deps
  build_gui_binary
  install_gui_binary
  install_icon
  install_desktop_entry
  refresh_desktop_database

  need_cmd desktop-file-validate
  local desktop
  desktop="$(detect_desktop)"
  case "${desktop}" in
    gnome)
      configure_gnome_shortcut
      ;;
    kde)
      configure_kde_shortcut
      verify_kde_shortcut
      ;;
    unknown)
      log "Skipping shortcut automation because the desktop environment could not be identified"
      ;;
  esac

  verify_installation
  print_summary "${desktop}"
}

main "$@"
