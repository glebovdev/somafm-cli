#!/bin/sh
# SomaFM CLI Installer
# Usage: curl -sSL https://raw.githubusercontent.com/glebovdev/somafm-cli/master/install.sh | sh
#
# Options (via environment variables):
#   VERSION=v0.1.5    Install specific version (default: latest)
#   INSTALL_DIR=/path Custom install directory (default: /usr/local/bin)

set -e

REPO="glebovdev/somafm-cli"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="somafm"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() {
    printf "${GREEN}[INFO]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1" >&2
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux";;
        Darwin*)    echo "darwin";;
        MINGW*|MSYS*|CYGWIN*) echo "windows";;
        *)          error "Unsupported operating system: $(uname -s)";;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64";;
        arm64|aarch64)  echo "arm64";;
        *)              error "Unsupported architecture: $(uname -m)";;
    esac
}

# Get latest version from GitHub
get_latest_version() {
    curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | \
        grep '"tag_name":' | \
        sed -E 's/.*"([^"]+)".*/\1/'
}

# Verify checksum
verify_checksum() {
    CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

    info "Verifying checksum..."

    CHECKSUMS=$(curl -fsSL "${CHECKSUM_URL}") || {
        warn "Could not download checksums, skipping verification"
        return 0
    }

    EXPECTED=$(echo "${CHECKSUMS}" | grep "${FILENAME}" | awk '{print $1}')
    if [ -z "$EXPECTED" ]; then
        warn "Checksum not found for ${FILENAME}, skipping verification"
        return 0
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL=$(sha256sum "${FILENAME}" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        ACTUAL=$(shasum -a 256 "${FILENAME}" | awk '{print $1}')
    else
        warn "No sha256 tool found, skipping verification"
        return 0
    fi

    if [ "$EXPECTED" != "$ACTUAL" ]; then
        error "Checksum verification failed"
    fi

    info "Checksum verified"
}

# Download and install
install() {
    OS=$(detect_os) || exit 1
    ARCH=$(detect_arch) || exit 1

    info "Detected OS: ${OS}, Arch: ${ARCH}"

    # Use VERSION env var or fetch latest
    if [ -z "$VERSION" ]; then
        VERSION=$(get_latest_version)
        if [ -z "$VERSION" ]; then
            error "Could not determine latest version"
        fi
        info "Latest version: ${VERSION}"
    else
        info "Installing version: ${VERSION}"
    fi

    # Build download URL
    FILENAME="${BINARY_NAME}_${VERSION#v}_${OS}_${ARCH}.tar.gz"
    if [ "$OS" = "windows" ]; then
        FILENAME="${BINARY_NAME}_${VERSION#v}_${OS}_${ARCH}.zip"
    fi

    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

    info "Downloading from: ${DOWNLOAD_URL}"

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf ${TMP_DIR}" EXIT INT TERM

    # Download
    curl -fsSL "${DOWNLOAD_URL}" -o "${TMP_DIR}/${FILENAME}"

    # Verify checksum
    cd "${TMP_DIR}"
    verify_checksum

    # Extract
    if [ "$OS" = "windows" ]; then
        unzip -q "${FILENAME}"
    else
        tar -xzf "${FILENAME}"
    fi

    # Install
    if [ ! -d "${INSTALL_DIR}" ]; then
        info "Creating ${INSTALL_DIR}"
        if [ -w "$(dirname "${INSTALL_DIR}")" ]; then
            mkdir -p "${INSTALL_DIR}"
        else
            sudo mkdir -p "${INSTALL_DIR}"
        fi
    fi

    if [ -w "${INSTALL_DIR}" ]; then
        mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        info "Requesting sudo access to install to ${INSTALL_DIR}"
        sudo mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

    info "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"

    # Verify installation
    if "${INSTALL_DIR}/${BINARY_NAME}" --version >/dev/null 2>&1; then
        info "Verified: $(${INSTALL_DIR}/${BINARY_NAME} --version | head -1)"
    else
        warn "Binary installed but could not run it"
        if [ "$OS" = "linux" ]; then
            warn "Linux requires libasound2 (ALSA). Install it with: sudo apt-get install libasound2"
        fi
    fi
}

# Main
main() {
    echo ""
    echo "  SomaFM CLI Installer"
    echo ""

    # Check for required tools
    command -v curl >/dev/null 2>&1 || error "curl is required but not installed"
    command -v tar >/dev/null 2>&1 || error "tar is required but not installed"

    install

    echo ""
    printf "${GREEN}Installation complete!${NC}\n"
    echo ""
    echo "  Run 'somafm' to start listening"
    echo "  Run 'somafm --help' for options"
    echo ""
}

main
