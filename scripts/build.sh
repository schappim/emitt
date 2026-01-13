#!/bin/bash
set -e

# 🧤 eMitt Build Script
# Usage: ./scripts/build.sh [platform]
# Examples:
#   ./scripts/build.sh           # Build all platforms
#   ./scripts/build.sh linux     # Build Linux only
#   ./scripts/build.sh darwin    # Build macOS only
#   ./scripts/build.sh local     # Build for current platform

BINARY_NAME="emitt"
DIST_DIR="dist"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { printf "${GREEN}[INFO]${NC} %s\n" "$1"; }
step() { printf "${BLUE}[STEP]${NC} %s\n" "$1"; }

# All platforms
ALL_PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

build_platform() {
    local OS="$1"
    local ARCH="$2"

    OUTPUT="${DIST_DIR}/${BINARY_NAME}-${OS}-${ARCH}"
    if [ "$OS" = "windows" ]; then
        OUTPUT="${OUTPUT}.exe"
    fi

    info "Building ${OS}/${ARCH}..."
    GOOS="$OS" GOARCH="$ARCH" go build -ldflags="-s -w" -o "$OUTPUT" ./cmd/emitt

    SIZE=$(ls -lh "$OUTPUT" | awk '{print $5}')
    echo "  → ${OUTPUT} (${SIZE})"
}

build_local() {
    step "Building for current platform..."

    OUTPUT="${BINARY_NAME}"
    go build -ldflags="-s -w" -o "$OUTPUT" ./cmd/emitt

    SIZE=$(ls -lh "$OUTPUT" | awk '{print $5}')
    info "Built: ${OUTPUT} (${SIZE})"
}

build_all() {
    step "Building for all platforms..."

    rm -rf "$DIST_DIR"
    mkdir -p "$DIST_DIR"

    for PLATFORM in "${ALL_PLATFORMS[@]}"; do
        OS="${PLATFORM%/*}"
        ARCH="${PLATFORM#*/}"
        build_platform "$OS" "$ARCH"
    done

    info "All builds complete"
    echo ""
    ls -lh "$DIST_DIR"
}

build_filtered() {
    local FILTER="$1"

    step "Building for ${FILTER}..."

    mkdir -p "$DIST_DIR"

    for PLATFORM in "${ALL_PLATFORMS[@]}"; do
        OS="${PLATFORM%/*}"
        ARCH="${PLATFORM#*/}"

        if [[ "$OS" == "$FILTER" ]]; then
            build_platform "$OS" "$ARCH"
        fi
    done

    info "Build complete"
}

main() {
    echo ""
    echo "  🧤 eMitt Build Script"
    echo "  ====================="
    echo ""

    case "${1:-all}" in
        local)
            build_local
            ;;
        linux|darwin|windows)
            build_filtered "$1"
            ;;
        all|"")
            build_all
            ;;
        *)
            echo "Usage: $0 [all|local|linux|darwin|windows]"
            exit 1
            ;;
    esac
}

main "$@"
