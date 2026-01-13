#!/bin/bash
set -e

# 🧤 eMitt Release Script
# Usage: ./scripts/release.sh [version]
# Example: ./scripts/release.sh v1.1.0

REPO="schappim/emitt"
BINARY_NAME="emitt"
DIST_DIR="dist"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { printf "${GREEN}[INFO]${NC} %s\n" "$1"; }
warn() { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; }
error() { printf "${RED}[ERROR]${NC} %s\n" "$1"; exit 1; }
step() { printf "${BLUE}[STEP]${NC} %s\n" "$1"; }

# Platforms to build
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

# Get version from argument or prompt
get_version() {
    if [ -n "$1" ]; then
        VERSION="$1"
    else
        # Get latest tag
        LATEST=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
        echo ""
        echo "  🧤 eMitt Release Script"
        echo "  ======================="
        echo ""
        echo "  Latest version: ${LATEST}"
        echo ""
        read -p "  Enter new version (e.g., v1.1.0): " VERSION
    fi

    # Validate version format
    if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        error "Invalid version format. Use semantic versioning (e.g., v1.0.0)"
    fi

    # Check if tag already exists
    if git rev-parse "$VERSION" >/dev/null 2>&1; then
        error "Version $VERSION already exists"
    fi
}

# Check prerequisites
check_prereqs() {
    step "Checking prerequisites..."

    if ! command -v go &> /dev/null; then
        error "Go is not installed"
    fi

    if ! command -v gh &> /dev/null; then
        error "GitHub CLI (gh) is not installed"
    fi

    if ! gh auth status &> /dev/null; then
        error "Not authenticated with GitHub. Run 'gh auth login'"
    fi

    # Check for uncommitted changes
    if [ -n "$(git status --porcelain)" ]; then
        warn "You have uncommitted changes"
        read -p "  Continue anyway? (y/N): " CONTINUE
        if [[ ! "$CONTINUE" =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi

    info "Prerequisites OK"
}

# Build binaries
build_binaries() {
    step "Building binaries for ${#PLATFORMS[@]} platforms..."

    rm -rf "$DIST_DIR"
    mkdir -p "$DIST_DIR"

    for PLATFORM in "${PLATFORMS[@]}"; do
        OS="${PLATFORM%/*}"
        ARCH="${PLATFORM#*/}"

        OUTPUT="${DIST_DIR}/${BINARY_NAME}-${OS}-${ARCH}"
        if [ "$OS" = "windows" ]; then
            OUTPUT="${OUTPUT}.exe"
        fi

        info "Building ${OS}/${ARCH}..."
        GOOS="$OS" GOARCH="$ARCH" go build -ldflags="-s -w" -o "$OUTPUT" ./cmd/emitt

        # Show file size
        SIZE=$(ls -lh "$OUTPUT" | awk '{print $5}')
        echo "    → ${OUTPUT} (${SIZE})"
    done

    info "All binaries built successfully"
}

# Create checksums
create_checksums() {
    step "Creating checksums..."

    cd "$DIST_DIR"
    shasum -a 256 * > checksums.txt
    cd ..

    info "Checksums created"
    cat "$DIST_DIR/checksums.txt"
}

# Commit and tag
commit_and_tag() {
    step "Creating git tag ${VERSION}..."

    git tag -a "$VERSION" -m "Release ${VERSION}"

    info "Tag created: ${VERSION}"
}

# Push to GitHub
push_to_github() {
    step "Pushing to GitHub..."

    git push origin main --tags

    info "Pushed to GitHub"
}

# Create GitHub release
create_release() {
    step "Creating GitHub release..."

    # Build release notes
    NOTES="## 🧤 eMitt ${VERSION}

Catch every email with LLM-powered automation.

### Installation

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh
\`\`\`

### Binaries

| Platform | Architecture | File |
|----------|--------------|------|
| Linux | x86_64 | \`emitt-linux-amd64\` |
| Linux | ARM64 | \`emitt-linux-arm64\` |
| macOS | Intel | \`emitt-darwin-amd64\` |
| macOS | Apple Silicon | \`emitt-darwin-arm64\` |
| Windows | x86_64 | \`emitt-windows-amd64.exe\` |

### Checksums (SHA-256)

\`\`\`
$(cat ${DIST_DIR}/checksums.txt)
\`\`\`
"

    # Create release with all binaries
    gh release create "$VERSION" \
        ${DIST_DIR}/emitt-linux-amd64 \
        ${DIST_DIR}/emitt-linux-arm64 \
        ${DIST_DIR}/emitt-darwin-amd64 \
        ${DIST_DIR}/emitt-darwin-arm64 \
        ${DIST_DIR}/emitt-windows-amd64.exe \
        ${DIST_DIR}/checksums.txt \
        --title "${VERSION}" \
        --notes "$NOTES"

    info "Release created: https://github.com/${REPO}/releases/tag/${VERSION}"
}

# Cleanup
cleanup() {
    step "Cleaning up..."
    rm -rf "$DIST_DIR"
    info "Cleanup complete"
}

# Main
main() {
    echo ""
    get_version "$1"
    echo ""
    info "Preparing release ${VERSION}"
    echo ""

    check_prereqs
    build_binaries
    create_checksums
    commit_and_tag
    push_to_github
    create_release

    echo ""
    echo "  ✅ Release ${VERSION} published successfully!"
    echo ""
    echo "  https://github.com/${REPO}/releases/tag/${VERSION}"
    echo ""
}

main "$@"
