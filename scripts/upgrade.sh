#!/bin/sh
set -e

# Lerian CLI Upgrader
# Usage: curl -fsSL https://raw.githubusercontent.com/LerianStudio/lerian-cli/main/scripts/upgrade.sh | sh
# Or:    curl -fsSL ... | sh -s -- --version v1.3.0
#
# It replaces the binary already on this machine, in the directory it is already
# in. Installing somewhere else and leaving the old one on PATH is the failure
# this exists to avoid: `lerian version` then reports whichever the shell finds
# first, and the upgrade looks as though it did nothing.

REPO="LerianStudio/lerian-cli"
PROJECT_NAME="lerian-cli"
BINARY_NAME="lerian"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()    { printf "${BLUE}[INFO]${NC} %s\n" "$1"; }
success() { printf "${GREEN}[OK]${NC} %s\n" "$1"; }
warn()    { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; }
error()   { printf "${RED}[ERROR]${NC} %s\n" "$1" >&2; exit 1; }

VERSION=""
CHECK_ONLY=""
while [ $# -gt 0 ]; do
    case "$1" in
        --version|-v)
            VERSION="$2"
            [ -n "$VERSION" ] || error "--version needs a tag, e.g. --version v1.3.0"
            shift 2
            ;;
        --check|-c)
            CHECK_ONLY="yes"
            shift
            ;;
        --help|-h)
            echo "Lerian CLI Upgrader"
            echo ""
            echo "Usage: curl -fsSL <url> | sh -s -- [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --version, -v <version>  Upgrade (or downgrade) to a specific version"
            echo "  --check, -c              Say what is installed and what is available, change nothing"
            echo "  --help, -h               Show this message"
            echo ""
            echo "Environment variables:"
            echo "  INSTALL_DIR              Where to write. Default: the directory the"
            echo "                           current binary is in, or ~/.local/bin"
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            ;;
    esac
done

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "Linux" ;;
        Darwin*) echo "Darwin" ;;
        *)       error "Unsupported operating system: $(uname -s)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "x86_64" ;;
        arm64|aarch64) echo "arm64" ;;
        armv7l)        echo "armv7" ;;
        *)             error "Unsupported architecture: $(uname -m)" ;;
    esac
}

check_gh() {
    command -v gh >/dev/null 2>&1 || \
        error "GitHub CLI (gh) is required but not installed.\n\nInstall it from: https://cli.github.com/\n\nThen authenticate with: gh auth login"
    gh auth status >/dev/null 2>&1 || \
        error "GitHub CLI is not authenticated.\n\nRun: gh auth login"
}

# Where the binary being replaced lives. Upgrading in place is the whole point:
# writing to a different directory leaves two binaries on PATH and the shell
# picks whichever comes first, which is rarely the new one.
find_current() {
    CURRENT_PATH=$(command -v "$BINARY_NAME" 2>/dev/null || true)
    if [ -n "$CURRENT_PATH" ]; then
        # Resolve a symlink, so an upgrade through one replaces the real file
        # rather than the link.
        if [ -L "$CURRENT_PATH" ]; then
            LINK_TARGET=$(readlink "$CURRENT_PATH")
            case "$LINK_TARGET" in
                /*) CURRENT_PATH="$LINK_TARGET" ;;
                *)  CURRENT_PATH="$(dirname "$CURRENT_PATH")/$LINK_TARGET" ;;
            esac
        fi
        CURRENT_DIR=$(dirname "$CURRENT_PATH")
        CURRENT_VERSION=$("$CURRENT_PATH" version 2>/dev/null | head -1 | awk '{print $NF}' || echo "unknown")
    else
        CURRENT_DIR=""
        CURRENT_VERSION=""
    fi
}

latest_version() {
    # --exclude-pre-releases: an upgrade should land on a release, not on
    # whatever beta was cut this afternoon. --version takes one deliberately.
    gh release list --repo "$REPO" --exclude-pre-releases --limit 1 --json tagName -q '.[0].tagName' 2>/dev/null || \
        error "Failed to read the releases. Make sure you have access to $REPO"
}

upgrade() {
    OS=$(detect_os)
    ARCH=$(detect_arch)

    check_gh
    find_current

    if [ -z "$CURRENT_DIR" ]; then
        warn "$BINARY_NAME is not on your PATH — there is nothing to upgrade."
        echo ""
        echo "Install it first:"
        echo "  curl -fsSL https://raw.githubusercontent.com/$REPO/main/scripts/install.sh | sh"
        exit 1
    fi

    info "Installed: $CURRENT_VERSION  ($CURRENT_PATH)"

    if [ -z "$VERSION" ]; then
        VERSION=$(latest_version)
    fi
    info "Available: $VERSION"

    # Compared as written, not ordered: the tags carry pre-release suffixes and
    # a shell cannot order those reliably. Equal means nothing to do; anything
    # else is a move the operator asked for, in either direction.
    if [ "v${CURRENT_VERSION#v}" = "v${VERSION#v}" ]; then
        success "Already on $VERSION — nothing to do."
        exit 0
    fi

    if [ -n "$CHECK_ONLY" ]; then
        info "Run without --check to move to $VERSION."
        exit 0
    fi

    INSTALL_DIR="${INSTALL_DIR:-$CURRENT_DIR}"

    # Writable before anything is downloaded: finding out afterwards means a
    # download thrown away and a confusing error at the last step.
    if [ ! -w "$INSTALL_DIR" ]; then
        error "$INSTALL_DIR is not writable.\n\nRe-run with sudo, or choose another directory:\n  INSTALL_DIR=\$HOME/.local/bin sh -s -- upgrade"
    fi

    VERSION_NO_V="${VERSION#v}"
    ARCHIVE_NAME="${PROJECT_NAME}_${VERSION_NO_V}_${OS}_${ARCH}.tar.gz"

    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT
    cd "$TMP_DIR"

    info "Downloading $ARCHIVE_NAME..."
    if ! gh release download "$VERSION" --repo "$REPO" --pattern "$ARCHIVE_NAME" 2>/dev/null; then
        printf "${RED}[ERROR]${NC} Failed to download %s\n\nAvailable assets for %s:\n" "$ARCHIVE_NAME" "$VERSION" >&2
        gh release view "$VERSION" --repo "$REPO" --json assets -q '.assets[].name' 2>/dev/null || true
        exit 1
    fi

    if gh release download "$VERSION" --repo "$REPO" --pattern "checksums.txt" 2>/dev/null; then
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
            [ "$EXPECTED" = "$ACTUAL" ] || \
                error "Checksum verification failed!\nExpected: $EXPECTED\nActual: $ACTUAL"
            success "Checksum verified"
        fi
    else
        warn "Checksums file not found, skipping verification"
    fi

    tar -xzf "$ARCHIVE_NAME"

    # Into place with mv, which is atomic within a filesystem: a half-written
    # binary on PATH is worse than an upgrade that failed. The old one is kept
    # until the new one is in, so a failure here leaves a working CLI.
    BACKUP="$INSTALL_DIR/.$BINARY_NAME.previous"
    cp "$INSTALL_DIR/$BINARY_NAME" "$BACKUP" 2>/dev/null || true
    if ! mv "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"; then
        [ -f "$BACKUP" ] && mv "$BACKUP" "$INSTALL_DIR/$BINARY_NAME"
        error "Could not write $INSTALL_DIR/$BINARY_NAME"
    fi
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
    rm -f "$BACKUP"

    success "Upgraded $CURRENT_VERSION → $VERSION"
    echo ""
    info "Verify:"
    echo "  $BINARY_NAME version"
}

upgrade
