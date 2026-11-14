#!/usr/bin/env bash
# ==============================================================================
# Fortinet Auto-Login Universal Installer for macOS and Linux
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/c2-07/fortinet-auto-login/main/install.sh | bash
# ==============================================================================

set -euo pipefail

REPO="c2-07/fortinet-auto-login"
INSTALL_DIR="${HOME}/.local/bin"
BIN_NAME="autologin"
TARGET_PATH="${INSTALL_DIR}/${BIN_NAME}"

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;36m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m' # No Color

printf "${BLUE}${BOLD}"
cat << 'EOF'
    ____           __  _            __ 
   / __/___  _____/ /_(_)___  ___  / /_
  / /_/ __ \/ ___/ __/ / __ \/ _ \/ __/
 / __/ /_/ / /  / /_/ / / / /  __/ /_  
/_/  \____/_/   \__/_/_/ /_/\___/\__/  
      Captive Portal Auto-Login
EOF
printf "${NC}\n"

echo -e "${BLUE}[INFO]${NC} Detecting operating system and architecture..."

# 1. Detect OS
OS_RAW="$(uname -s)"
case "${OS_RAW}" in
    Darwin)
        OS="darwin"
        ;;
    Linux)
        OS="linux"
        ;;
    *)
        echo -e "${RED}[FAIL]${NC} Unsupported operating system: ${OS_RAW}. Only macOS and Linux are supported by this script."
        echo -e "${YELLOW}[NOTE]${NC} For Windows, run: irm https://raw.githubusercontent.com/${REPO}/main/install.ps1 | iex"
        exit 1
        ;;
esac

# 2. Detect Architecture
ARCH_RAW="$(uname -m)"
case "${ARCH_RAW}" in
    x86_64|amd64)
        ARCH="amd64"
        ;;
    arm64|aarch64)
        ARCH="arm64"
        ;;
    *)
        echo -e "${RED}[FAIL]${NC} Unsupported architecture: ${ARCH_RAW}. Only x86_64 and arm64 are supported."
        exit 1
        ;;
esac

ASSET_NAME="fortinet-auto-login-${OS}-${ARCH}"
echo -e "${GREEN}[OK]${NC}   Target detected: ${BOLD}${OS}/${ARCH}${NC} (${ASSET_NAME})"

# 3. Create target directory
mkdir -p "${INSTALL_DIR}"

# 4. Resolve download URL
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET_NAME}"
echo -e "${BLUE}[INFO]${NC} Downloading latest release from GitHub..."

TEMP_FILE="$(mktemp "${TMPDIR:-/tmp}/autologin.XXXXXX")"
cleanup() {
    rm -f "${TEMP_FILE}"
}
trap cleanup EXIT

if command -v curl >/dev/null 2>&1; then
    if ! curl -fL --progress-bar "${DOWNLOAD_URL}" -o "${TEMP_FILE}"; then
        echo -e "${RED}[FAIL]${NC} Failed to download release asset from ${DOWNLOAD_URL}"
        exit 1
    fi
elif command -v wget >/dev/null 2>&1; then
    if ! wget -q --show-progress -O "${TEMP_FILE}" "${DOWNLOAD_URL}"; then
        echo -e "${RED}[FAIL]${NC} Failed to download release asset from ${DOWNLOAD_URL}"
        exit 1
    fi
else
    echo -e "${RED}[FAIL]${NC} Neither curl nor wget was found on your system."
    exit 1
fi

# Verify downloaded binary is non-empty
if [ ! -s "${TEMP_FILE}" ]; then
    echo -e "${RED}[FAIL]${NC} Downloaded binary is empty or corrupt."
    exit 1
fi

chmod +x "${TEMP_FILE}"
mv -f "${TEMP_FILE}" "${TARGET_PATH}"
echo -e "${GREEN}[OK]${NC}   Binary installed to: ${BOLD}${TARGET_PATH}${NC}"

# 5. Ensure INSTALL_DIR is in PATH permanently
PATH_MODIFIED=0

check_and_add_path() {
    local rc_file="$1"
    local export_line='export PATH="$HOME/.local/bin:$PATH"'
    local comment_line='# Added by fortinet-auto-login installer'

    if [ -f "${rc_file}" ] || [ "${rc_file}" = "${HOME}/.profile" ]; then
        if ! grep -qF "${INSTALL_DIR}" "${rc_file}" 2>/dev/null; then
            echo "" >> "${rc_file}"
            echo "${comment_line}" >> "${rc_file}"
            echo "${export_line}" >> "${rc_file}"
            echo -e "${GREEN}[OK]${NC}   Added ${INSTALL_DIR} to ${rc_file}"
            PATH_MODIFIED=1
        fi
    fi
}

case "${SHELL:-}" in
    */zsh)
        check_and_add_path "${HOME}/.zshrc"
        ;;
    */bash)
        check_and_add_path "${HOME}/.bashrc"
        if [ "${OS}" = "darwin" ]; then
            check_and_add_path "${HOME}/.bash_profile"
        fi
        ;;
    */fish)
        FISH_CONFIG="${HOME}/.config/fish/config.fish"
        if [ -f "${FISH_CONFIG}" ] && ! grep -qF "${INSTALL_DIR}" "${FISH_CONFIG}"; then
            echo "fish_add_path ${INSTALL_DIR}" >> "${FISH_CONFIG}"
            echo -e "${GREEN}[OK]${NC}   Added ${INSTALL_DIR} to ${FISH_CONFIG}"
            PATH_MODIFIED=1
        fi
        ;;
    *)
        check_and_add_path "${HOME}/.profile"
        ;;
esac

# Fallback: ensure ~/.zshrc or ~/.bashrc is updated if current shell was ambiguous
if [ "${PATH_MODIFIED}" -eq 0 ] && [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
    if [ -f "${HOME}/.zshrc" ]; then
        check_and_add_path "${HOME}/.zshrc"
    fi
    if [ -f "${HOME}/.bashrc" ]; then
        check_and_add_path "${HOME}/.bashrc"
    fi
fi

echo ""
echo -e "${GREEN}${BOLD}Installation complete!${NC}"
echo -e "${BLUE}[INFO]${NC} Run: ${BOLD}autologin${NC} to log in to your Fortinet captive portal."
echo -e "${BLUE}[INFO]${NC} Run: ${BOLD}autologin -h${NC} to view available commands."

if [ "${PATH_MODIFIED}" -eq 1 ] || [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
    echo -e "${YELLOW}[NOTE]${NC} Restart your terminal or run: ${BOLD}export PATH=\"\$HOME/.local/bin:\$PATH\"${NC} to use 'autologin' directly."
fi
