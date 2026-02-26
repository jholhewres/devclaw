#!/usr/bin/env bash
# DevClaw Uninstaller — Linux + macOS
#
# Usage: sudo ./uninstall.sh [--yes]
#
# Options:
#   --yes    Skip confirmation prompt

set -euo pipefail

# Colors
BOLD='\033[1m'
ACCENT='\033[38;2;0;229;204m'
SUCCESS='\033[38;2;0;229;204m'
WARN='\033[38;2;255;176;32m'
ERROR='\033[38;2;230;57;70m'
MUTED='\033[38;2;90;100;128m'
NC='\033[0m'

# Installation directories
LINUX_INSTALL_DIR="/opt/devclaw"
MACOS_INSTALL_DIR="/usr/local/opt/devclaw"
BIN_SYMLINK="/usr/local/bin/devclaw"

# Flags
YES=0

ui_info()  { printf "${MUTED}·${NC} %s\n" "$1"; }
ui_warn()  { printf "${WARN}!${NC} %s\n" "$1"; }
ui_error() { printf "${ERROR}✗${NC} %s\n" "$1" >&2; }
ui_success() { printf "${SUCCESS}✓${NC} %s\n" "$1"; }
ui_section() { printf "\n${ACCENT}${BOLD}%s${NC}\n" "$1"; }

print_banner() {
  printf "\n"
  printf "${ACCENT}${BOLD}  ╔══════════════════════════════════════╗${NC}\n"
  printf "${ACCENT}${BOLD}  ║${NC}  ${BOLD}DevClaw Uninstaller${NC}                    ${ACCENT}${BOLD}║${NC}\n"
  printf "${ACCENT}${BOLD}  ╚══════════════════════════════════════╝${NC}\n"
  printf "\n"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --yes|-y) YES=1; shift ;;
      --help|-h)
        echo "Usage: sudo ./uninstall.sh [--yes]"
        echo ""
        echo "Options:"
        echo "  --yes    Skip confirmation prompt"
        exit 0
        ;;
      *) shift ;;
    esac
  done
}

detect_install_dir() {
  if [[ -d "$LINUX_INSTALL_DIR" ]]; then
    echo "$LINUX_INSTALL_DIR"
  elif [[ -d "$MACOS_INSTALL_DIR" ]]; then
    echo "$MACOS_INSTALL_DIR"
  else
    echo ""
  fi
}

confirm_uninstall() {
  if [[ "$YES" == "1" ]]; then
    return 0
  fi

  printf "\n${WARN}${BOLD}⚠ This will completely remove DevClaw from your system.${NC}\n"
  printf "\nThe following will be deleted:\n"
  printf "  - DevClaw binary and all data\n"
  printf "  - PM2 process configuration\n"
  printf "  - Global 'devclaw' command\n"

  if [[ -n "$INSTALL_DIR" ]]; then
    printf "\n${MUTED}Install directory:${NC} %s\n" "$INSTALL_DIR"
  fi

  printf "\n"
  read -p "Continue with uninstall? [y/N] " -n 1 -r
  printf "\n"

  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    ui_info "Uninstall cancelled"
    exit 0
  fi
}

stop_pm2() {
  ui_section "Stopping PM2 Process"

  if ! command -v pm2 &>/dev/null; then
    ui_info "PM2 not found, skipping"
    return 0
  fi

  if pm2 describe devclaw &>/dev/null; then
    ui_info "Stopping devclaw process..."
    pm2 delete devclaw &>/dev/null || true
    pm2 save &>/dev/null || true
    ui_success "PM2 process removed"
  else
    ui_info "No devclaw process found in PM2"
  fi
}

remove_symlink() {
  ui_section "Removing Global Command"

  if [[ -L "$BIN_SYMLINK" ]] || [[ -f "$BIN_SYMLINK" ]]; then
    if rm -f "$BIN_SYMLINK" 2>/dev/null; then
      ui_success "Removed: $BIN_SYMLINK"
    else
      ui_warn "Could not remove $BIN_SYMLINK (may need manual removal)"
    fi
  else
    ui_info "Symlink not found: $BIN_SYMLINK"
  fi
}

remove_installation() {
  ui_section "Removing Installation"

  if [[ -z "$INSTALL_DIR" ]] || [[ ! -d "$INSTALL_DIR" ]]; then
    ui_info "Install directory not found"
    return 0
  fi

  ui_info "Removing: $INSTALL_DIR"

  if rm -rf "$INSTALL_DIR" 2>/dev/null; then
    ui_success "Installation removed"
  else
    ui_error "Could not remove $INSTALL_DIR"
    ui_info "You may need to run: sudo rm -rf $INSTALL_DIR"
    exit 1
  fi
}

print_complete() {
  printf "\n"
  printf "${SUCCESS}${BOLD}  ✓ DevClaw uninstalled successfully${NC}\n"
  printf "\n"

  if command -v pm2 &>/dev/null; then
    printf "  ${MUTED}To completely remove PM2 (optional):${NC}\n"
    printf "    npm uninstall -g pm2\n"
    printf "\n"
  fi
}

main() {
  parse_args "$@"
  print_banner

  # Check if running as root for system directories
  if [[ "$(id -u)" != "0" ]]; then
    if [[ -d "$LINUX_INSTALL_DIR" ]]; then
      ui_error "This script must be run as root (sudo) to remove system installation"
      exit 1
    fi
  fi

  INSTALL_DIR=$(detect_install_dir)

  if [[ -z "$INSTALL_DIR" ]]; then
    ui_warn "DevClaw installation not found"
    printf "\n${MUTED}Looked in:${NC}\n"
    printf "  - %s\n" "$LINUX_INSTALL_DIR"
    printf "  - %s\n" "$MACOS_INSTALL_DIR"
    exit 1
  fi

  ui_info "Found installation: $INSTALL_DIR"

  confirm_uninstall
  stop_pm2
  remove_symlink
  remove_installation
  print_complete
}

main "$@"
