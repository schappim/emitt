#!/bin/sh
set -e

# 🧤 eMitt Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/schappim/emitt/main/install.sh | sh

REPO="schappim/emitt"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="emitt"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    printf "${GREEN}[INFO]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1"
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux" ;;
        Darwin*)    echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *)          error "Unsupported operating system: $(uname -s)" ;;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64" ;;
        arm64|aarch64)  echo "arm64" ;;
        *)              error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Get latest release version
get_latest_version() {
    curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | \
        grep '"tag_name":' | \
        sed -E 's/.*"([^"]+)".*/\1/'
}

main() {
    echo ""
    echo "  🧤 eMitt Installer"
    echo "  ==================="
    echo ""

    OS=$(detect_os)
    ARCH=$(detect_arch)

    info "Detected OS: ${OS}"
    info "Detected Architecture: ${ARCH}"

    # Get latest version
    info "Fetching latest version..."
    VERSION=$(get_latest_version)

    if [ -z "$VERSION" ]; then
        error "Could not determine latest version"
    fi

    info "Latest version: ${VERSION}"

    # Construct download URL
    EXT=""
    if [ "$OS" = "windows" ]; then
        EXT=".exe"
    fi

    FILENAME="emitt-${OS}-${ARCH}${EXT}"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

    info "Downloading ${FILENAME}..."

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    TMP_FILE="${TMP_DIR}/${BINARY_NAME}${EXT}"

    # Download binary
    if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"; then
        rm -rf "$TMP_DIR"
        error "Failed to download from ${DOWNLOAD_URL}"
    fi

    # Make executable
    chmod +x "$TMP_FILE"

    # Install
    if [ "$OS" = "windows" ]; then
        INSTALL_DIR="$HOME/bin"
        mkdir -p "$INSTALL_DIR"
    fi

    info "Installing to ${INSTALL_DIR}/${BINARY_NAME}${EXT}..."

    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}${EXT}"
    else
        warn "Need sudo to install to ${INSTALL_DIR}"
        sudo mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}${EXT}"
    fi

    # Cleanup
    rm -rf "$TMP_DIR"

    echo ""
    info "Successfully installed eMitt ${VERSION}!"
    echo ""
    echo "  Get started:"
    echo "    1. Create a config file: cp config.yaml.example config.yaml"
    echo "    2. Set your OpenAI API key: export OPENAI_API_KEY=sk-..."
    echo "    3. Run: emitt -config config.yaml"
    echo ""
    echo "  Documentation: https://github.com/${REPO}"
    echo ""
}

main
