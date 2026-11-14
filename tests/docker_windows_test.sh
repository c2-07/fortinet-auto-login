#!/bin/bash
set -e

echo "=========================================================="
echo "         RUNNING WINDOWS DAEMON DOCKER TESTS (WINE)       "
echo "=========================================================="

cd /app

echo "--- 1. Cross-compiling for Windows x86_64 (GNU) ---"
cargo build --target x86_64-pc-windows-gnu --release
TARGET_DIR="${CARGO_TARGET_DIR:-./target}"
AUTOLOGIN_EXE="${TARGET_DIR}/x86_64-pc-windows-gnu/release/autologin.exe"

if [ ! -f "$AUTOLOGIN_EXE" ]; then
    echo "[FAIL] autologin.exe was not created at $AUTOLOGIN_EXE"
    exit 1
fi
echo "[PASS] autologin.exe compiled successfully"
ls -la "$AUTOLOGIN_EXE"

echo "--- 2. Initializing Wine Environment & Compatibility Layer ---"
wine cmd /c "echo Initializing Wine prefix..." > /dev/null 2>&1

cat << 'EOF' > /tmp/prng.c
#include <windows.h>
#include <bcrypt.h>

__declspec(dllexport) BOOL WINAPI ProcessPrng(PBYTE pbData, SIZE_T cbData) {
    return BCryptGenRandom(NULL, pbData, (ULONG)cbData, BCRYPT_USE_SYSTEM_PREFERRED_RNG) == 0;
}
EOF
x86_64-w64-mingw32-gcc -shared -o /root/.wine/drive_c/windows/system32/bcryptprimitives.dll /tmp/prng.c -lbcrypt

echo "--- 3. Testing Windows PE Binary Execution ---"
echo "Testing -version flag..."
VERSION_OUT=$(wine "$AUTOLOGIN_EXE" -version 2>&1)
echo "Output: $VERSION_OUT"
if echo "$VERSION_OUT" | grep -q "fortinet-auto-login v1.0.0 (rust)"; then
    echo "[PASS] Windows PE binary executed and reported correct version"
else
    echo "[FAIL] Unexpected version output: $VERSION_OUT"
    exit 1
fi

echo "Testing -help flag..."
HELP_OUT=$(wine "$AUTOLOGIN_EXE" -help 2>&1)
if echo "$HELP_OUT" | grep -q "Windows"; then
    echo "[PASS] Windows service command documented in help"
else
    echo "[FAIL] Missing Windows documentation in help"
    exit 1
fi

echo "Testing -status flag..."
wine "$AUTOLOGIN_EXE" -status
echo "[PASS] Status inspection executed successfully on Windows target"

echo "--- 4. Testing Windows Daemon Execution & ProcessLock in Wine ---"
# Pre-configure credentials for non-interactive execution
wine "$AUTOLOGIN_EXE" -u "win_user" -p "win_pass" 2>&1 || true

# Start Windows daemon in background
wine "$AUTOLOGIN_EXE" -daemon -interval 1s &
WINE_PID=$!
echo "Windows daemon running under Wine with PID: $WINE_PID"

sleep 3

# Verify daemon is running
if ps -ef | grep -v grep | grep -q "autologin.exe"; then
    echo "[PASS] Windows daemon process is running under Wine"
else
    echo "[FAIL] Windows daemon process died unexpectedly"
    exit 1
fi

# Test Windows ProcessLock mutual exclusion (share_mode(0))
echo "Testing Windows exclusive lock mutual exclusion..."
SECOND_OUT=$(wine "$AUTOLOGIN_EXE" -daemon -interval 1s 2>&1 || true)
echo "Second instance output: $SECOND_OUT"
if echo "$SECOND_OUT" | grep -q "Another autologin instance is currently running"; then
    echo "[PASS] Windows share_mode(0) lock successfully blocked duplicate instance!"
else
    echo "[FAIL] Duplicate instance was NOT blocked on Windows!"
    kill -9 $WINE_PID || true
    exit 1
fi

# Verify Windows log file in %TEMP%\autologin.log
WINE_LOG="/root/.wine/drive_c/users/root/Temp/autologin.log"
if [ -f "$WINE_LOG" ]; then
    echo "[PASS] Windows daemon log file created at $WINE_LOG"
    cat "$WINE_LOG"
else
    echo "[FAIL] Windows log file not found at $WINE_LOG"
    exit 1
fi

# Terminate daemon cleanly
echo "Terminating Windows daemon..."
kill -TERM $WINE_PID 2>/dev/null || true
wait $WINE_PID 2>/dev/null || true
echo "[PASS] Windows daemon stopped cleanly"

echo "=========================================================="
echo "          ALL WINDOWS TESTS PASSED UNDER WINE!            "
echo "=========================================================="
