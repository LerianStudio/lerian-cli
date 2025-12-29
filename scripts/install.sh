#!/bin/sh
set -e

# Lerian CLI Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/install.sh | sh
# Or: curl -fsSL ... | sh -s -- --version v1.0.0

REPO="LerianStudio/lerian-cli"
PROJECT_NAME="lerian-cli"
BINARY_NAME="lerian"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() {
    printf "${BLUE}[INFO]${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}[OK]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1" >&2
    exit 1
}

# Parse arguments
VERSION=""
while [ $# -gt 0 ]; do
    case "$1" in
        --version|-v)
            VERSION="$2"
            shift 2
            ;;
        --help|-h)
            echo "Lerian CLI Installer"
            echo ""
            echo "Usage: curl -fsSL <url> | sh -s -- [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --version, -v <version>  Install specific version (e.g., v1.0.0)"
            echo "  --help, -h               Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  INSTALL_DIR              Installation directory (default: ~/.local/bin)"
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            ;;
    esac
done

# Detect OS
detect_os() {
    OS="$(uname -s)"
    case "$OS" in
        Linux*)  echo "Linux" ;;
        Darwin*) echo "Darwin" ;;
        *)       error "Unsupported operating system: $OS" ;;
    esac
}

# Detect architecture
detect_arch() {
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64)  echo "x86_64" ;;
        arm64|aarch64) echo "arm64" ;;
        armv7l)        echo "armv7" ;;
        *)             error "Unsupported architecture: $ARCH" ;;
    esac
}

# Check if gh CLI is installed and authenticated
check_gh() {
    if ! command -v gh >/dev/null 2>&1; then
        error "GitHub CLI (gh) is required but not installed.\n\nInstall it from: https://cli.github.com/\n\nThen authenticate with: gh auth login"
    fi
    
    if ! gh auth status >/dev/null 2>&1; then
        error "GitHub CLI is not authenticated.\n\nRun: gh auth login"
    fi
}

# Get latest version from GitHub releases (includes pre-releases)
get_latest_version() {
    gh release list --repo "$REPO" --limit 1 --json tagName -q '.[0].tagName' 2>/dev/null || \
        error "Failed to get latest release. Make sure you have access to $REPO"
}

# Download and install
install() {
    OS=$(detect_os)
    ARCH=$(detect_arch)
    
    info "Detected OS: $OS, Architecture: $ARCH"
    
    check_gh
    
    if [ -z "$VERSION" ]; then
        info "Fetching latest version..."
        VERSION=$(get_latest_version)
    fi
    
    info "Installing $BINARY_NAME $VERSION..."
    
    # Build archive name (matches goreleaser template)
    VERSION_NO_V="${VERSION#v}"
    ARCHIVE_NAME="${PROJECT_NAME}_${VERSION_NO_V}_${OS}_${ARCH}.tar.gz"
    
    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT
    
    cd "$TMP_DIR"
    
    # Download release archive
    info "Downloading $ARCHIVE_NAME..."
    if ! gh release download "$VERSION" --repo "$REPO" --pattern "$ARCHIVE_NAME" 2>/dev/null; then
        error "Failed to download $ARCHIVE_NAME\n\nAvailable assets for $VERSION:"
        gh release view "$VERSION" --repo "$REPO" --json assets -q '.assets[].name' 2>/dev/null || true
        exit 1
    fi
    
    # Download checksums
    info "Downloading checksums..."
    gh release download "$VERSION" --repo "$REPO" --pattern "checksums.txt" 2>/dev/null || \
        warn "Checksums file not found, skipping verification"
    
    # Verify checksum if available
    if [ -f "checksums.txt" ]; then
        info "Verifying checksum..."
        EXPECTED=$(grep "$ARCHIVE_NAME" checksums.txt | awk '{print $1}')
        if [ -n "$EXPECTED" ]; then
            if command -v sha256sum >/dev/null 2>&1; then
                ACTUAL=$(sha256sum "$ARCHIVE_NAME" | awk '{print $1}')
            elif command -v shasum >/dev/null 2>&1; then
                ACTUAL=$(shasum -a 256 "$ARCHIVE_NAME" | awk '{print $1}')
            else
                warn "No sha256sum or shasum found, skipping verification"
                ACTUAL="$EXPECTED"
            fi
            
            if [ "$EXPECTED" != "$ACTUAL" ]; then
                error "Checksum verification failed!\nExpected: $EXPECTED\nActual: $ACTUAL"
            fi
            success "Checksum verified"
        fi
    fi
    
    # Extract archive
    info "Extracting archive..."
    tar -xzf "$ARCHIVE_NAME"
    
    # Create install directory if needed
    mkdir -p "$INSTALL_DIR"
    
    # Install binary
    info "Installing to $INSTALL_DIR/$BINARY_NAME..."
    mv "$BINARY_NAME" "$INSTALL_DIR/"
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
    
    success "Successfully installed $BINARY_NAME $VERSION"
    
    # Check if install dir is in PATH
    case ":$PATH:" in
        *":$INSTALL_DIR:"*) ;;
        *)
            echo ""
            warn "$INSTALL_DIR is not in your PATH"
            echo ""
            echo "Add it to your shell profile:"
            echo ""
            echo "  # For bash (~/.bashrc or ~/.bash_profile)"
            echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
            echo ""
            echo "  # For zsh (~/.zshrc)"
            echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
            echo ""
            echo "  # For fish (~/.config/fish/config.fish)"
            echo "  set -gx PATH \$HOME/.local/bin \$PATH"
            echo ""
            ;;
    esac
    
    # Verify installation
    if [ -x "$INSTALL_DIR/$BINARY_NAME" ]; then
        echo ""
        info "Verify installation:"
        echo "  $INSTALL_DIR/$BINARY_NAME --version"
        echo ""
        info "Get started:"
        echo "  $BINARY_NAME --help"
    fi
}

# Run installer
install
