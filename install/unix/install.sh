#!/usr/bin/env bash
# DevClaw Installer — Linux + macOS
# Usage: curl -fsSL <domain>/install.sh | bash -s -- [--stage <test|production>]
#
# Options:
#   --stage <env>       Environment: test (homologação) or production (default: production)
#   --url <base_url>    Override base URL (optional)
#   --local <path>      Use local directory instead of downloading (for testing)
#   --version <tag>     Install specific version (default: auto-detect latest from S3)
#   --port <port>       Server port (default: 47716)
#   --no-prompt         Non-interactive mode
#   --dry-run           Show what would happen without making changes
#   --verbose           Show detailed output
#   --help              Show this help message
#
set -euo pipefail

# =============================================================================
# Configuration
# =============================================================================

BINARY="devclaw"
SCRIPT_VERSION="2.1.0"

# GatorClaw Assets URLs
PRODUCTION_URL="https://assets-gatorclaw.hostgator.io"
TEST_URL="https://test-assets-gatorclaw.hostgator.io"

# Installation directories
LINUX_INSTALL_DIR="/opt/devclaw"
MACOS_INSTALL_DIR="/usr/local/opt/devclaw"
BIN_SYMLINK="/usr/local/bin/devclaw"

# Colors
BOLD='\033[1m'
ACCENT='\033[38;2;0;229;204m'
SUCCESS='\033[38;2;0;229;204m'
WARN='\033[38;2;255;176;32m'
ERROR='\033[38;2;230;57;70m'
MUTED='\033[38;2;90;100;128m'
NC='\033[0m'

# Flags
STAGE="production"
BASE_URL=""
LOCAL_PATH=""
VERSION=""
VERSION_SPECIFIED=""
PORT="47716"
NO_PROMPT=0
SKIP_DEPS=0
DRY_RUN=0
VERBOSE=0

# Detected platform
OS=""
ARCH=""
INSTALL_DIR=""

# Temp files for cleanup
TMPFILES=()

# Installation state
CONFIG_WAS_CREATED=0

# =============================================================================
# Utility Functions
# =============================================================================

cleanup() {
  for f in "${TMPFILES[@]:-}"; do
    rm -rf "$f" 2>/dev/null || true
  done
}
trap cleanup EXIT

mktempdir() {
  local d
  d="$(mktemp -d)"
  TMPFILES+=("$d")
  echo "$d"
}

ui_info()  { printf "${MUTED}·${NC} %s\n" "$1"; }
ui_warn()  { printf "${WARN}!${NC} %s\n" "$1"; }
ui_error() { printf "${ERROR}✗${NC} %s\n" "$1" >&2; }
ui_success() { printf "${SUCCESS}✓${NC} %s\n" "$1"; }
ui_section() { printf "\n${ACCENT}${BOLD}%s${NC}\n" "$1"; }
ui_kv() { printf "${MUTED}%-18s${NC} %s\n" "$1:" "$2"; }

print_banner() {
  printf "\n"
  printf "${ACCENT}${BOLD}  ╔══════════════════════════════════════╗${NC}\n"
  printf "${ACCENT}${BOLD}  ║${NC}   ${BOLD}DevClaw${NC} — AI Agent for Tech Teams    ${ACCENT}${BOLD}║${NC}\n"
  printf "${ACCENT}${BOLD}  ╚══════════════════════════════════════╝${NC}\n"
  printf "\n"
}

print_usage() {
  cat <<EOF
DevClaw Installer v${SCRIPT_VERSION}

Usage:
  curl -fsSL <domain>/install.sh | bash -s -- [--stage <test|production>]
  ./install.sh --local <path> [--version <tag>]

Options:
  --stage <env>       Environment: test (homologação) or production (default: production)
  --url <base_url>    Override base URL (optional)
  --local <path>      Use local directory instead of downloading (for testing)
  --version <tag>     Install specific version (default: auto-detect latest from S3)
  --port <port>       Server port (default: 47716)
  --no-prompt         Non-interactive mode (skip confirmations)
  --skip-deps         Skip dependency installation (use if already installed)
  --dry-run           Show what would happen without making changes
  --verbose           Show detailed output (set -x)
  --help, -h          Show this help message

Environment:
  DEVCLAW_STAGE       Environment: test or production (same as --stage)
  DEVCLAW_URL         Override base URL (same as --url)
  DEVCLAW_VERSION     Version to install (same as --version)
  DEVCLAW_PORT        Server port (same as --port)
  DEVCLAW_NO_PROMPT   Set to 1 for non-interactive mode
  DEVCLAW_SKIP_DEPS   Set to 1 to skip dependency installation

Examples:
  # Install latest from production (default)
  curl -fsSL ... | bash -s --

  # Install latest from test/homologação
  curl -fsSL ... | bash -s -- --stage test

  # Install specific version from production
  curl -fsSL ... | bash -s -- --version v1.2.3

  # Install from local directory (for testing)
  ./install.sh --local ./dist --version v1.0.0

  # Non-interactive (for CI/CD)
  curl -fsSL ... | bash -s -- --no-prompt

  # Skip dependency installation (if already installed)
  curl -fsSL ... | bash -s -- --skip-deps --no-prompt
EOF
}

# =============================================================================
# Argument Parsing
# =============================================================================

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --stage) STAGE="$2"; shift 2 ;;
      --url) BASE_URL="$2"; shift 2 ;;
      --local) LOCAL_PATH="$2"; shift 2 ;;
      --version) VERSION="$2"; VERSION_SPECIFIED="1"; shift 2 ;;
      --port) PORT="$2"; shift 2 ;;
      --no-prompt) NO_PROMPT=1; shift ;;
      --skip-deps) SKIP_DEPS=1; shift ;;
      --dry-run) DRY_RUN=1; shift ;;
      --verbose) VERBOSE=1; shift ;;
      --help|-h) print_usage; exit 0 ;;
      *) shift ;;
    esac
  done

  # Environment variable fallbacks
  STAGE="${STAGE:-${DEVCLAW_STAGE:-production}}"
  BASE_URL="${BASE_URL:-${DEVCLAW_URL:-}}"
  VERSION="${VERSION:-${DEVCLAW_VERSION:-}}"
  PORT="${PORT:-${DEVCLAW_PORT:-47716}}"
  NO_PROMPT="${NO_PROMPT:-${DEVCLAW_NO_PROMPT:-0}}"
  SKIP_DEPS="${SKIP_DEPS:-${DEVCLAW_SKIP_DEPS:-0}}"

  if [[ "$VERBOSE" == "1" ]]; then
    set -x
  fi

  # Set base URL from stage if not explicitly provided via --url or --local
  if [[ -z "$BASE_URL" && -z "$LOCAL_PATH" ]]; then
    case "$STAGE" in
      test|staging|homolog|hml)
        BASE_URL="$TEST_URL"
        STAGE="test"
        ;;
      production|prod|prd)
        BASE_URL="$PRODUCTION_URL"
        STAGE="production"
        ;;
      *)
        ui_error "Invalid stage: $STAGE"
        echo "Valid stages: test (homologação), production"
        exit 1
        ;;
    esac
  fi

  # Validate that we have a source (either URL or local path)
  if [[ -z "$BASE_URL" && -z "$LOCAL_PATH" ]]; then
    ui_error "Missing required argument: --stage or --url or --local"
    echo ""
    echo "Usage: install.sh [--stage <test|production>] [--version <tag>] [--port <port>]"
    echo "   or: install.sh --url <base_url> [--version <tag>] [--port <port>]"
    echo "   or: install.sh --local <path> [--version <tag>] [--port <port>]"
    echo ""
    echo "Examples:"
    echo "  install.sh                          # Production (default)"
    echo "  install.sh --stage test             # Test/homologação"
    echo "  install.sh --url https://assets.example.com/devclaw"
    echo "  install.sh --local ./dist --version v1.0.0"
    exit 1
  fi

  # Validate local path exists if specified
  if [[ -n "$LOCAL_PATH" && ! -d "$LOCAL_PATH" ]]; then
    ui_error "Local path does not exist: $LOCAL_PATH"
    exit 1
  fi

  # Remove trailing slash from URL
  if [[ -n "$BASE_URL" ]]; then
    BASE_URL="${BASE_URL%/}"
  fi
}

# =============================================================================
# Platform Detection
# =============================================================================

detect_platform() {
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"

  case "$OS" in
    linux)
      OS="linux"
      INSTALL_DIR="$LINUX_INSTALL_DIR"
      ;;
    darwin)
      OS="darwin"
      INSTALL_DIR="$MACOS_INSTALL_DIR"
      ;;
    *)
      ui_error "Unsupported OS: $OS"
      echo "This installer supports Linux and macOS."
      exit 1
      ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
      ui_error "Unsupported architecture: $ARCH"
      exit 1
      ;;
  esac

  ui_kv "Platform" "${OS}/${ARCH}"
  ui_kv "Install dir" "$INSTALL_DIR"
}

# =============================================================================
# Dependency Installation
# =============================================================================

check_command() {
  command -v "$1" &>/dev/null
}

install_deps_linux() {
  ui_section "Installing Dependencies (Linux)"

  local pkg_manager=""
  local install_cmd=""

  if command -v apt-get &>/dev/null; then
    pkg_manager="apt"
    install_cmd="apt-get update && apt-get install -y"
  elif command -v yum &>/dev/null; then
    pkg_manager="yum"
    install_cmd="yum install -y"
  elif command -v dnf &>/dev/null; then
    pkg_manager="dnf"
    install_cmd="dnf install -y"
  else
    ui_error "No supported package manager found (apt, yum, dnf)"
    exit 1
  fi

  ui_info "Package manager: $pkg_manager"

  # Basic tools
  local basic_tools="wget curl git ca-certificates unzip"

  if [[ "$pkg_manager" == "apt" ]]; then
    basic_tools="$basic_tools build-essential"
  else
    basic_tools="$basic_tools gcc make"
  fi

  ui_info "Installing basic tools..."
  if [[ "$DRY_RUN" != "1" ]]; then
    if [[ "$(id -u)" == "0" ]]; then
      sh -c "$install_cmd $basic_tools" || ui_warn "Some basic tools may already be installed"
    else
      sudo sh -c "$install_cmd $basic_tools" || ui_warn "Some basic tools may already be installed"
    fi
  fi

  # Python 3
  ui_info "Checking Python 3..."
  if ! check_command python3; then
    ui_info "Installing Python 3..."
    if [[ "$DRY_RUN" != "1" ]]; then
      if [[ "$(id -u)" == "0" ]]; then
        sh -c "$install_cmd python3 python3-pip" || ui_warn "Python installation may need manual setup"
      else
        sudo sh -c "$install_cmd python3 python3-pip" || ui_warn "Python installation may need manual setup"
      fi
    fi
  else
    ui_success "Python 3 already installed: $(python3 --version 2>&1 || echo 'unknown')"
  fi

  # Node.js
  ui_info "Checking Node.js..."
  if ! check_command node; then
    ui_info "Installing Node.js 22.x..."
    if [[ "$DRY_RUN" != "1" ]]; then
      if [[ "$pkg_manager" == "apt" ]]; then
        curl -fsSL https://deb.nodesource.com/setup_22.x | sudo bash -
        sudo apt-get install -y nodejs
      else
        # For RHEL-based systems
        if [[ "$(id -u)" == "0" ]]; then
          curl -fsSL https://rpm.nodesource.com/setup_22.x | bash -
          $install_cmd nodejs
        else
          curl -fsSL https://rpm.nodesource.com/setup_22.x | sudo bash -
          sudo $install_cmd nodejs
        fi
      fi
    fi
  else
    local node_version
    node_version=$(node -v 2>/dev/null || echo "unknown")
    ui_success "Node.js already installed: $node_version"
  fi

  # PM2
  ui_info "Checking PM2..."
  if ! check_command pm2; then
    ui_info "Installing PM2..."
    if [[ "$DRY_RUN" != "1" ]]; then
      if [[ "$(id -u)" == "0" ]]; then
        npm install -g pm2
      else
        sudo npm install -g pm2
      fi
    fi
  else
    ui_success "PM2 already installed: $(pm2 -v 2>/dev/null || echo 'unknown')"
  fi

  # Chrome/Chromium (required for browser automation)
  ui_info "Checking Chrome/Chromium..."
  local chrome_found=""
  if command -v google-chrome &>/dev/null; then
    chrome_found="google-chrome"
  elif command -v google-chrome-stable &>/dev/null; then
    chrome_found="google-chrome-stable"
  elif command -v chromium-browser &>/dev/null; then
    chrome_found="chromium-browser"
  elif command -v chromium &>/dev/null; then
    chrome_found="chromium"
  fi

  if [[ -n "$chrome_found" ]]; then
    ui_success "Chrome/Chromium already installed: $chrome_found"
  else
    ui_info "Installing Chrome/Chromium (required for browser automation)..."
    if [[ "$DRY_RUN" != "1" ]]; then
      if [[ "$pkg_manager" == "apt" ]]; then
        # Install Google Chrome directly (most reliable on servers)
        ui_info "Installing Google Chrome..."
        local chrome_installed=0

        # Detect if we need t64 suffix (Ubuntu 22.04+) or not (Debian, older Ubuntu)
        # Ubuntu 22.04+ uses t64 suffix for some packages
        local t64_suffix=""
        if [[ -f /etc/os-release ]]; then
          . /etc/os-release
          if [[ "$ID" == "ubuntu" ]]; then
            # Check Ubuntu version
            local ubuntu_version
            ubuntu_version=$(echo "$VERSION_ID" | cut -d. -f1 2>/dev/null || echo "0")
            if [[ "$ubuntu_version" -ge 22 ]]; then
              t64_suffix="t64"
            fi
          fi
        fi

        # Build package list based on distro
        local chrome_deps="wget ca-certificates fonts-liberation"
        if [[ -n "$t64_suffix" ]]; then
          # Ubuntu 22.04+
          chrome_deps="$chrome_deps libasound2t64 libatk-bridge2.0-0t64 libatk1.0-0t64 libcups2t64 libdbus-1-3 libdrm2 libgbm1 libgtk-3-0t64 libnspr4 libnss3 libxcomposite1 libxdamage1 libxfixes3 libxkbcommon0 libxrandr2 xdg-utils"
        else
          # Debian and older Ubuntu
          chrome_deps="$chrome_deps libasound2 libatk-bridge2.0-0 libatk1.0-0 libcups2 libdbus-1-3 libdrm2 libgbm1 libgtk-3-0 libnspr4 libnss3 libxcomposite1 libxdamage1 libxfixes3 libxkbcommon0 libxrandr2 xdg-utils"
        fi

        if [[ "$(id -u)" == "0" ]]; then
          # Install dependencies
          apt-get install -y $chrome_deps 2>/dev/null || true

          wget -q -O /tmp/google-chrome.deb https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
          apt-get install -y /tmp/google-chrome.deb 2>/dev/null || apt-get install -y -f /tmp/google-chrome.deb 2>/dev/null
          rm -f /tmp/google-chrome.deb

          # Verify installation
          if command -v google-chrome-stable &>/dev/null; then
            chrome_installed=1
            ui_success "Google Chrome installed successfully"
          fi
        else
          # Install dependencies
          sudo apt-get install -y $chrome_deps 2>/dev/null || true

          wget -q -O /tmp/google-chrome.deb https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
          sudo apt-get install -y /tmp/google-chrome.deb 2>/dev/null || sudo apt-get install -y -f /tmp/google-chrome.deb 2>/dev/null
          rm -f /tmp/google-chrome.deb

          # Verify installation
          if command -v google-chrome-stable &>/dev/null; then
            chrome_installed=1
            ui_success "Google Chrome installed successfully"
          fi
        fi

        if [[ "$chrome_installed" != "1" ]]; then
          ui_warn "Could not install Chrome automatically"
          ui_info "Try manually: wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb && sudo apt install ./google-chrome-stable_current_amd64.deb"
          ui_warn "Browser automation features will not work without Chrome/Chromium"
        fi
      else
        # RHEL-based systems
        if [[ "$(id -u)" == "0" ]]; then
          $install_cmd chromium 2>/dev/null || {
            ui_info "Chromium not available, please install Chrome manually"
            ui_warn "Browser automation features will not work without Chrome/Chromium"
          }
        else
          sudo $install_cmd chromium 2>/dev/null || {
            ui_info "Chromium not available, please install Chrome manually"
            ui_warn "Browser automation features will not work without Chrome/Chromium"
          }
        fi
      fi
    fi
  fi
}

install_deps_macos() {
  ui_section "Installing Dependencies (macOS)"

  # Check for Homebrew
  if ! check_command brew; then
    ui_error "Homebrew is required for macOS installation"
    echo "Install Homebrew first: /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
    exit 1
  fi

  ui_info "Package manager: Homebrew"

  # Basic tools
  ui_info "Installing basic tools..."
  if [[ "$DRY_RUN" != "1" ]]; then
    brew install wget git curl unzip || ui_warn "Some tools may already be installed"
  fi

  # Python 3
  ui_info "Checking Python 3..."
  if ! check_command python3; then
    ui_info "Installing Python 3..."
    if [[ "$DRY_RUN" != "1" ]]; then
      brew install python3 || ui_warn "Python installation may need manual setup"
    fi
  else
    ui_success "Python 3 already installed: $(python3 --version 2>&1 || echo 'unknown')"
  fi

  # Node.js
  ui_info "Checking Node.js..."
  if ! check_command node; then
    ui_info "Installing Node.js..."
    if [[ "$DRY_RUN" != "1" ]]; then
      brew install node@22 || brew install node || ui_warn "Node.js installation may need manual setup"
    fi
  else
    local node_version
    node_version=$(node -v 2>/dev/null || echo "unknown")
    ui_success "Node.js already installed: $node_version"
  fi

  # PM2
  ui_info "Checking PM2..."
  if ! check_command pm2; then
    ui_info "Installing PM2..."
    if [[ "$DRY_RUN" != "1" ]]; then
      npm install -g pm2
    fi
  else
    ui_success "PM2 already installed: $(pm2 -v 2>/dev/null || echo 'unknown')"
  fi

  # Chrome/Chromium (required for browser automation)
  ui_info "Checking Chrome/Chromium..."
  local chrome_path="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
  if [[ -x "$chrome_path" ]]; then
    ui_success "Chrome already installed"
  elif command -v google-chrome-stable &>/dev/null; then
    ui_success "Chrome already installed: $(command -v google-chrome-stable)"
  else
    ui_info "Installing Google Chrome (required for browser automation)..."
    if [[ "$DRY_RUN" != "1" ]]; then
      brew install --cask google-chrome 2>/dev/null || {
        ui_warn "Could not install Chrome automatically"
        ui_info "Please install Chrome manually: https://www.google.com/chrome/"
        ui_warn "Browser automation features will not work without Chrome"
      }
    fi
  fi
}

install_dependencies() {
  if [[ "$SKIP_DEPS" == "1" ]]; then
    ui_info "Skipping dependency installation (--skip-deps)"
    return 0
  fi

  case "$OS" in
    linux)  install_deps_linux ;;
    darwin) install_deps_macos ;;
  esac
}

# =============================================================================
# Download and Install
# =============================================================================

fetch_metadata() {
  # If using local path, read metadata directly from file
  if [[ -n "$LOCAL_PATH" ]]; then
    local metadata_file="${LOCAL_PATH}/metadata.json"

    if [[ -f "$metadata_file" ]]; then
      ui_info "Reading metadata from $metadata_file..."
      cat "$metadata_file"
      return 0
    else
      ui_warn "metadata.json not found in local path, using default values"
      echo "{\"version\": \"${VERSION:-local}\", \"binary\": \"devclaw\"}"
      return 0
    fi
  fi

  # Remote mode: download via curl
  # Add cache-busting query parameter to avoid stale CDN/S3 responses
  local cache_buster="_=$(date +%s)"
  local metadata_url="${BASE_URL}/metadata.json?${cache_buster}"

  ui_info "Fetching metadata from ${BASE_URL}/metadata.json..."

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would fetch: ${BASE_URL}/metadata.json"
    echo '{"version": "dry-run", "binary": "devclaw"}'
    return 0
  fi

  local metadata
  metadata=$(curl -fsSL "$metadata_url" 2>/dev/null) || {
    ui_warn "Could not fetch metadata.json, using default values"
    echo "{\"version\": \"${VERSION}\", \"binary\": \"devclaw\"}"
    return 0
  }

  echo "$metadata"
}

detect_latest_version() {
  # Detect latest version from latest.txt file
  # The deployment process should update this file with the current version tag
  local latest_version=""

  # Add cache-busting query parameter to avoid stale CDN/S3 responses
  local cache_buster="_=$(date +%s)"
  local latest_url="${BASE_URL}/latest.txt?${cache_buster}"
  ui_info "Detecting latest version from ${BASE_URL}/latest.txt..."

  if [[ "$DRY_RUN" == "1" ]]; then
    latest_version="v1.0.0-dryrun"
  else
    latest_version=$(curl -fsSL "$latest_url" 2>/dev/null | head -1 | tr -d '[:space:]') || true
  fi

  if [[ -n "$latest_version" ]]; then
    echo "$latest_version"
    return 0
  fi

  return 1
}

resolve_version() {
  local metadata="$1"

  # If version is explicitly specified, show it
  if [[ -n "$VERSION_SPECIFIED" ]]; then
    ui_kv "Version" "$VERSION (specified)"
    return 0
  fi

  # If version is not specified, detect latest from S3
  if [[ -z "$VERSION" ]]; then
    ui_info "Version not specified, detecting latest from S3..."
    local detected_version
    detected_version=$(detect_latest_version)

    if [[ -n "$detected_version" ]]; then
      VERSION="$detected_version"
      ui_kv "Version" "$VERSION (latest)"
    else
      # Fallback: try to get from metadata.json
      detected_version=$(echo "$metadata" | grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*: *"\([^"]*\)".*/\1/')
      if [[ -n "$detected_version" ]]; then
        VERSION="$detected_version"
        ui_kv "Version" "$VERSION (from metadata)"
      else
        # Final fallback: use "latest" as version (will download latest.zip)
        VERSION="latest"
        ui_kv "Version" "latest (fallback)"
      fi
    fi
  else
    ui_kv "Version" "latest (auto-detect)"
  fi
}

download_and_install() {
  local tmpdir
  tmpdir="$(mktempdir)"

  # Determine which archive to download:
  # - If version was explicitly specified: use devclaw-{VERSION}.zip
  # - If version was auto-detected (latest): use latest.zip
  local archive=""
  local use_latest=0

  if [[ -n "$LOCAL_PATH" ]]; then
    # Local mode
    archive="devclaw-${VERSION}.zip"
  elif [[ -n "${VERSION_SPECIFIED:-}" ]]; then
    # User explicitly specified a version
    archive="devclaw-${VERSION}.zip"
  else
    # Version was auto-detected, use latest.zip
    archive="latest.zip"
    use_latest=1
  fi

  # Local mode: copy from local directory
  if [[ -n "$LOCAL_PATH" ]]; then
    ui_section "Installing from Local Directory"

    local local_archive="${LOCAL_PATH}/${archive}"

    # If latest.zip doesn't exist locally, try devclaw-{VERSION}.zip, then any zip
    if [[ ! -f "$local_archive" ]]; then
      # Try devclaw-{VERSION}.zip as fallback
      if [[ -f "${LOCAL_PATH}/devclaw-${VERSION}.zip" ]]; then
        local_archive="${LOCAL_PATH}/devclaw-${VERSION}.zip"
        ui_info "Found versioned archive: $local_archive"
      else
        # Try any devclaw zip
        local found_zip
        found_zip=$(find "$LOCAL_PATH" -name "devclaw*.zip" -type f 2>/dev/null | head -1)
        if [[ -n "$found_zip" ]]; then
          local_archive="$found_zip"
          ui_info "Found archive: $local_archive"
        fi
      fi
    fi

    if [[ -f "$local_archive" ]]; then
      ui_info "Using local archive: $local_archive"

      if [[ "$DRY_RUN" == "1" ]]; then
        ui_info "[dry-run] Would extract to: $INSTALL_DIR"
        return 0
      fi

      # Extract
      ui_info "Extracting..."
      unzip -q -o "$local_archive" -d "${tmpdir}/extracted"
    else
      # No archive found, copy files directly from local path
      ui_info "No archive found, copying files from: $LOCAL_PATH"

      if [[ "$DRY_RUN" == "1" ]]; then
        ui_info "[dry-run] Would copy files to: $INSTALL_DIR"
        return 0
      fi

      # Create extraction directory and copy files
      mkdir -p "${tmpdir}/extracted"

      # Copy binary if it exists
      if [[ -f "${LOCAL_PATH}/devclaw" ]]; then
        cp "${LOCAL_PATH}/devclaw" "${tmpdir}/extracted/"
      elif [[ -f "${LOCAL_PATH}/devclaw-linux-amd64" ]]; then
        cp "${LOCAL_PATH}/devclaw-linux-amd64" "${tmpdir}/extracted/devclaw"
      fi

      # Copy ecosystem.config.js if it exists
      if [[ -f "${LOCAL_PATH}/ecosystem.config.js" ]]; then
        cp "${LOCAL_PATH}/ecosystem.config.js" "${tmpdir}/extracted/"
      elif [[ -f "${PWD}/install/unix/ecosystem.config.js" ]]; then
        cp "${PWD}/install/unix/ecosystem.config.js" "${tmpdir}/extracted/"
      fi
    fi
  else
    # Remote mode: download via curl
    # Add cache-busting query parameter for latest.zip to avoid stale CDN/S3 responses
    local download_url="${BASE_URL}/${archive}"
    if [[ "$use_latest" == "1" ]]; then
      local cache_buster="_=$(date +%s)"
      download_url="${download_url}?${cache_buster}"
    fi

    ui_section "Downloading DevClaw"

    if [[ "$use_latest" == "1" ]]; then
      ui_info "Downloading latest release: ${BASE_URL}/${archive}"
    else
      ui_info "Downloading version ${VERSION}: ${BASE_URL}/${archive}"
    fi

    if [[ "$DRY_RUN" == "1" ]]; then
      ui_info "[dry-run] Would download to: ${tmpdir}/${archive}"
      ui_info "[dry-run] Would extract to: $INSTALL_DIR"
      return 0
    fi

    # Download
    if ! curl -fsSL --progress-bar -o "${tmpdir}/${archive}" "$download_url"; then
      ui_error "Failed to download: $download_url"
      echo ""
      if [[ "$use_latest" == "1" ]]; then
        echo "Make sure latest.zip exists at:"
        echo "  ${BASE_URL}/latest.zip"
      else
        echo "Make sure the release exists at:"
        echo "  ${BASE_URL}/${archive}"
      fi
      exit 1
    fi

    ui_success "Download complete"

    # Extract
    ui_info "Extracting..."
    unzip -q -o "${tmpdir}/${archive}" -d "${tmpdir}/extracted"
  fi

  # Verify we have something to install
  if [[ ! -f "${tmpdir}/extracted/devclaw" ]]; then
    ui_error "No devclaw binary found to install"
    exit 1
  fi

  # Create install directory
  ui_info "Installing to $INSTALL_DIR..."
  if [[ "$(id -u)" == "0" ]]; then
    mkdir -p "$INSTALL_DIR"
  else
    sudo mkdir -p "$INSTALL_DIR"
  fi

  # Copy files
  if [[ "$(id -u)" == "0" ]]; then
    cp -r "${tmpdir}/extracted/"* "$INSTALL_DIR/"
  else
    sudo cp -r "${tmpdir}/extracted/"* "$INSTALL_DIR/"
  fi

  # Make binary executable
  if [[ "$(id -u)" == "0" ]]; then
    chmod +x "${INSTALL_DIR}/devclaw"
  else
    sudo chmod +x "${INSTALL_DIR}/devclaw"
  fi

  ui_success "Binary installed"
}

setup_directories() {
  ui_section "Setting Up Directories"

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would create directories in $INSTALL_DIR"
    return 0
  fi

  local dirs=("data" "sessions" "skills" "logs")

  for dir in "${dirs[@]}"; do
    local full_path="${INSTALL_DIR}/${dir}"
    if [[ ! -d "$full_path" ]]; then
      ui_info "Creating: $full_path"
      if [[ "$(id -u)" == "0" ]]; then
        mkdir -p "$full_path"
      else
        sudo mkdir -p "$full_path"
      fi
    fi
  done

  # Set permissions for sensitive directories
  local secure_dirs=("data" "sessions")
  for dir in "${secure_dirs[@]}"; do
    local full_path="${INSTALL_DIR}/${dir}"
    if [[ "$(id -u)" == "0" ]]; then
      chmod 700 "$full_path"
    else
      sudo chmod 700 "$full_path"
    fi
  done

  ui_success "Directories created"
}

setup_global_command() {
  ui_section "Setting Up Global Command"

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would create symlink: $BIN_SYMLINK -> ${INSTALL_DIR}/devclaw"
    return 0
  fi

  # Remove existing symlink or file
  if [[ -L "$BIN_SYMLINK" ]] || [[ -f "$BIN_SYMLINK" ]]; then
    if [[ "$(id -u)" == "0" ]]; then
      rm -f "$BIN_SYMLINK"
    else
      sudo rm -f "$BIN_SYMLINK"
    fi
  fi

  # Create symlink
  if [[ "$(id -u)" == "0" ]]; then
    ln -sf "${INSTALL_DIR}/devclaw" "$BIN_SYMLINK"
  else
    sudo ln -sf "${INSTALL_DIR}/devclaw" "$BIN_SYMLINK"
  fi

  ui_success "Global command available: devclaw"
}

generate_config() {
  ui_section "Generating Configuration"

  local config_file="${INSTALL_DIR}/config.yaml"

  if [[ -f "$config_file" ]]; then
    ui_info "Config already exists: $config_file"
    return 0
  fi

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would skip config generation (DevClaw will create on first run)"
    return 0
  fi

  # Don't create config.yaml - let DevClaw create it on first run
  # This allows DevClaw to enter "First Run Setup" mode automatically
  ui_info "Skipping config generation (DevClaw will create on first run)"
  ui_info "DevClaw will start in 'First Run Setup' mode"

  CONFIG_WAS_CREATED=1
}

update_pm2_config() {
  ui_section "Configuring PM2"

  local ecosystem_file="${INSTALL_DIR}/ecosystem.config.js"

  if [[ ! -f "$ecosystem_file" ]]; then
    ui_warn "ecosystem.config.js not found, skipping PM2 setup"
    return 0
  fi

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would update ecosystem.config.js with:"
    ui_info "  PORT=$PORT"
    ui_info "  DEVCLAW_STATE_DIR=$INSTALL_DIR"
    return 0
  fi

  # Update paths in ecosystem.config.js
  if [[ "$(id -u)" == "0" ]]; then
    sed -i.bak "s|/opt/devclaw|${INSTALL_DIR}|g" "$ecosystem_file"
    rm -f "${ecosystem_file}.bak"
  else
    sudo sed -i.bak "s|/opt/devclaw|${INSTALL_DIR}|g" "$ecosystem_file"
    sudo rm -f "${ecosystem_file}.bak"
  fi

  ui_success "PM2 config updated"
}

start_with_pm2() {
  ui_section "Starting with PM2"

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_info "[dry-run] Would run: pm2 start ${INSTALL_DIR}/ecosystem.config.js"
    ui_info "[dry-run] Would run: pm2 save"
    ui_info "[dry-run] Would run: pm2 startup"
    return 0
  fi

  cd "$INSTALL_DIR"

  # Stop existing process if running
  if pm2 describe devclaw &>/dev/null; then
    ui_info "Stopping existing devclaw process..."
    pm2 delete devclaw &>/dev/null || true
  fi

  # Start with PM2
  ui_info "Starting devclaw..."
  pm2 start ecosystem.config.js

  # Save PM2 configuration
  pm2 save

  # Configure PM2 startup (auto-start on boot)
  ui_info "Configuring PM2 startup..."

  if [[ "$(id -u)" == "0" ]]; then
    # Running as root: pm2 startup executes directly (no sudo command printed)
    pm2 startup 2>/dev/null || ui_warn "PM2 startup command failed"
    pm2 save 2>/dev/null || true
    ui_success "PM2 startup configured"
  else
    # Running as non-root: pm2 startup prints a sudo command we need to execute
    local startup_output
    startup_output=$(pm2 startup 2>&1) || true

    # Extract the sudo command from pm2 startup output
    # PM2 outputs something like:
    # [PM2] To setup the Startup Script, copy/paste the following command:
    # sudo env PATH=$PATH:/usr/bin pm2 startup systemd -u user --hp /home/user
    local startup_cmd
    startup_cmd=$(echo "$startup_output" | grep -E "sudo env PATH" | tail -1 | sed 's/^[[:space:]]*//') || true

    if [[ -z "$startup_cmd" ]]; then
      # Fallback: try to find any line with sudo
      startup_cmd=$(echo "$startup_output" | grep -E "^\s*sudo" | tail -1 | sed 's/^[[:space:]]*//') || true
    fi

    if [[ -n "$startup_cmd" ]]; then
      ui_info "Running: $startup_cmd"
      eval "$startup_cmd" 2>/dev/null || ui_warn "Startup command failed"
      pm2 save 2>/dev/null || true
      ui_success "PM2 startup configured"
    else
      ui_warn "Could not auto-detect PM2 startup command"
      ui_info "Run these commands manually:"
      echo ""
      echo "    pm2 startup"
      echo "    pm2 save"
      echo ""
    fi
  fi

  if [[ "$CONFIG_WAS_CREATED" == "1" ]]; then
    ui_success "DevClaw started with PM2 (First Run Setup mode)"
    ui_info "Access http://0.0.0.0:${PORT}/setup to complete configuration"
  else
    ui_success "DevClaw started with PM2"
  fi
}

print_pm2_startup() {
  # PM2 startup is now configured automatically in start_with_pm2
  return 0
}

print_success() {
  printf "\n"
  printf "${SUCCESS}${BOLD}  ✓ DevClaw installed successfully!${NC}\n"
  printf "\n"
  printf "  Installation:\n"
  printf "    Binary:     %s\n" "${INSTALL_DIR}/devclaw"
  printf "    Config:     %s\n" "${INSTALL_DIR}/config.yaml"
  printf "    Data:       %s\n" "${INSTALL_DIR}/data/"
  printf "    Logs:       %s\n" "${INSTALL_DIR}/logs/"
  printf "\n"

  if [[ "$CONFIG_WAS_CREATED" == "1" ]]; then
    printf "  ${ACCENT}${BOLD}→ First Run Setup${NC}\n"
    printf "\n"
    printf "  DevClaw is running in setup mode.\n"
    printf "  Complete the configuration by accessing:\n"
    printf "\n"
    printf "    http://0.0.0.0:%s/setup\n" "${PORT}"
    printf "\n"
    printf "  Or run manually:\n"
    printf "    devclaw setup\n"
    printf "\n"
  fi

  printf "  Commands:\n"
  printf "    devclaw --version    # Check version\n"
  printf "    devclaw serve        # Start server (foreground)\n"
  printf "    pm2 status           # Check PM2 status\n"
  printf "    pm2 logs devclaw     # View logs\n"
  printf "    pm2 restart devclaw  # Restart service\n"
  printf "    pm2 stop devclaw     # Stop service\n"
  printf "\n"
  printf "  Web UI:\n"
  printf "    http://0.0.0.0:%s\n" "${PORT}"
  printf "    http://0.0.0.0:%s/setup\n" "${PORT}"
  printf "\n"
}

# =============================================================================
# Main
# =============================================================================

print_install_plan() {
  ui_section "Install Plan"

  if [[ -n "$LOCAL_PATH" ]]; then
    ui_kv "Source" "local directory"
    ui_kv "Local path" "$LOCAL_PATH"
  else
    ui_kv "Stage" "$STAGE"
    ui_kv "Base URL" "$BASE_URL"
  fi

  ui_kv "Version" "$VERSION"
  ui_kv "Port" "$PORT"
  ui_kv "Platform" "${OS}/${ARCH}"
  ui_kv "Install dir" "$INSTALL_DIR"

  if [[ "$DRY_RUN" == "1" ]]; then
    ui_kv "Mode" "dry-run (no changes)"
  fi
}

confirm_install() {
  if [[ "$NO_PROMPT" == "1" ]] || [[ "$DRY_RUN" == "1" ]]; then
    return 0
  fi

  echo ""
  read -p "Continue with installation? [y/N] " -n 1 -r
  echo ""

  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    ui_info "Installation cancelled"
    exit 0
  fi
}

main() {
  parse_args "$@"
  print_banner
  detect_platform

  local metadata
  metadata=$(fetch_metadata)
  resolve_version "$metadata"

  print_install_plan
  confirm_install

  install_dependencies
  download_and_install
  setup_directories
  setup_global_command
  generate_config
  update_pm2_config
  start_with_pm2

  print_pm2_startup
  print_success
}

main "$@"
