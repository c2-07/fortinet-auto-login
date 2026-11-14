#!/bin/bash
set -e

echo "=========================================================="
echo "          RUNNING LINUX DAEMON DOCKER TESTS               "
echo "=========================================================="

cd /app

echo "--- 1. Linux Compilation & Unit Tests ---"
go test -v ./...

echo "--- 2. Build Release Binary for Linux ---"
go build -o autologin ./src
AUTOLOGIN="./autologin"

echo "--- 3. Setup Test Credentials ---"
$AUTOLOGIN -u "docker_user" -p "docker_pass"

echo "--- 4. Testing Linux Service Unit Installation ---"
$AUTOLOGIN -service install || true
SERVICE_FILE="$HOME/.config/systemd/user/fortinet-autologin.service"
if [ -f "$SERVICE_FILE" ]; then
    echo "[PASS] Systemd unit created successfully at $SERVICE_FILE"
    grep "ExecStart=" "$SERVICE_FILE"
else
    echo "[FAIL] Systemd unit was not created at $SERVICE_FILE"
    exit 1
fi

$AUTOLOGIN -service uninstall || true
if [ ! -f "$SERVICE_FILE" ]; then
    echo "[PASS] Systemd unit removed successfully"
else
    echo "[FAIL] Systemd unit still exists after uninstall"
    exit 1
fi

echo "--- 5. Testing Linux Daemon Execution & Mutual Exclusion ---"
rm -f /tmp/autologin.log
rm -rf /tmp/fortinet-autologin-locks

# Launch daemon in background with 1s interval
$AUTOLOGIN -daemon -interval 1s &
DAEMON_PID=$!
echo "Daemon started with PID: $DAEMON_PID"

sleep 2

# Verify process is alive
if ps -p $DAEMON_PID > /dev/null 2>&1; then
    echo "[PASS] Daemon process is running (PID: $DAEMON_PID)"
else
    echo "[FAIL] Daemon process died unexpectedly!"
    cat /tmp/autologin.log || true
    exit 1
fi

# Verify lock file exists
LOCK_FILE="/tmp/fortinet-autologin-locks/daemon.lock"
if [ -f "$LOCK_FILE" ]; then
    echo "[PASS] Daemon lock file created at $LOCK_FILE"
else
    echo "[FAIL] Lock file $LOCK_FILE not found!"
    exit 1
fi

# Test duplicate instance rejection (Mutual Exclusion)
echo "Testing duplicate instance rejection..."
OUTPUT=$($AUTOLOGIN -daemon -interval 1s 2>&1 || true)
echo "Second instance output: $OUTPUT"
if echo "$OUTPUT" | grep -qi "Another autologin"; then
    echo "[PASS] Mutual exclusion enforced: duplicate instance rejected!"
else
    echo "[FAIL] Duplicate instance was NOT rejected!"
    kill -9 $DAEMON_PID || true
    exit 1
fi

# Verify log file generated and formatted
if [ -f "/tmp/autologin.log" ]; then
    echo "[PASS] Log file created at /tmp/autologin.log"
    echo "--- Log contents ---"
    cat /tmp/autologin.log
    echo "--------------------"
    if grep -q "\[DAEMON\] Starting background monitoring daemon" /tmp/autologin.log; then
        echo "[PASS] Daemon lifecycle logged with standard formatting"
    else
        echo "[FAIL] Daemon log format missing expected tag"
        exit 1
    fi
else
    echo "[FAIL] Log file /tmp/autologin.log not found!"
    exit 1
fi

# Test log viewer
echo "Testing log viewing..."
$AUTOLOGIN -logs -n 10

# Test log clearing
echo "Testing log clearing..."
$AUTOLOGIN -logs -clear
if [ ! -s "/tmp/autologin.log" ]; then
    echo "[PASS] Log file cleared successfully"
else
    echo "[FAIL] Log file was not cleared"
    exit 1
fi

# Clean shutdown via SIGTERM
echo "Testing graceful daemon shutdown via SIGTERM..."
kill -TERM $DAEMON_PID
wait $DAEMON_PID 2>/dev/null || true
echo "[PASS] Daemon exited cleanly"

# Verify lock released and removed
sleep 1
if [ ! -f "$LOCK_FILE" ]; then
    echo "[PASS] ProcessLock cleaned up after graceful SIGTERM exit"
else
    echo "[FAIL] ProcessLock still present after daemon exit"
    exit 1
fi

echo "--- 6. Testing Crash Recovery (Stale Lock File) ---"
# Simulate an abrupt crash with kill -9
$AUTOLOGIN -daemon -interval 1s &
CRASH_PID=$!
sleep 1
touch "$LOCK_FILE"
kill -9 $CRASH_PID 2>/dev/null || true
sleep 1

# Stale lock file remains on disk
if [ -f "$LOCK_FILE" ]; then
    echo "[INFO] Stale lock file verified on disk after SIGKILL"
fi

# A new instance must be able to acquire lock immediately because flock was released by kernel
$AUTOLOGIN -daemon -interval 1s &
RECOVERED_PID=$!
sleep 2

if ps -p $RECOVERED_PID > /dev/null 2>&1; then
    echo "[PASS] Crash recovery successful: new daemon acquired lock despite stale file"
    kill -TERM $RECOVERED_PID 2>/dev/null || true
    wait $RECOVERED_PID 2>/dev/null || true
else
    echo "[FAIL] Failed to recover from stale lock file!"
    exit 1
fi

echo "=========================================================="
echo "           ALL LINUX DAEMON TESTS PASSED!                 "
echo "=========================================================="
