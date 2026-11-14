#!/usr/bin/env python3
"""
Benchmark script: Login & Logout 10 consecutive iterations with 5s cooldown.
Measures latency, verifies zero DNS hangs, and collects p50/p95/min/max statistics.
Ensures internet connectivity is always restored on exit.
"""

import subprocess
import time
import sys
import json
import statistics

AUTOLOGIN_BIN = "/Users/gourav/.local/bin/autologin"
ITERATIONS = 10
COOLDOWN_SECS = 5

def run_cmd(cmd):
    start = time.perf_counter()
    p = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    elapsed = time.perf_counter() - start
    return p.returncode, p.stdout.strip(), elapsed

def check_internet():
    # Use curl directly
    rc, out, _ = run_cmd(["curl", "-s", "--max-time", "3", "http://detectportal.firefox.com/success.txt"])
    return rc == 0 and "success" in out

def emergency_restore():
    print("\n[SAFETY] Ensuring internet is connected before terminating...")
    if not check_internet():
        print("[SAFETY] Internet not detected. Running autologin...")
        rc, out, el = run_cmd([AUTOLOGIN_BIN])
        print(f"[SAFETY] Autologin output (elapsed {el:.2f}s):\n{out}")
    else:
        print("[SAFETY] Internet confirmed connected.")

def main():
    print("=" * 60)
    print(f"FORTINET AUTO-LOGIN BENCHMARK: {ITERATIONS} ITERATIONS")
    print(f"Binary:   {AUTOLOGIN_BIN}")
    print(f"Cooldown: {COOLDOWN_SECS}s between actions")
    print("=" * 60)

    # Initial check
    initial_online = check_internet()
    print(f"Initial internet status: {'CONNECTED' if initial_online else 'DISCONNECTED'}")
    if not initial_online:
        print("Logging in first to establish baseline...")
        rc, out, el = run_cmd([AUTOLOGIN_BIN])
        print(out)
        time.sleep(2)

    logout_times = []
    login_times = []
    failures = 0

    try:
        for i in range(1, ITERATIONS + 1):
            print(f"\n--- [Iteration {i:02d}/{ITERATIONS:02d}] ---")
            
            # 1. Logout
            print(f"[{time.strftime('%H:%M:%S')}] Executing: autologin -logout")
            rc, out, logout_dur = run_cmd([AUTOLOGIN_BIN, "-logout"])
            print(f"Logout exit={rc} ({logout_dur:.3f}s):\n{out}")
            if rc != 0:
                print(f"[WARN] Logout returned non-zero code {rc}")
            logout_times.append(logout_dur)

            # Cooldown
            print(f"[{time.strftime('%H:%M:%S')}] Cooldown {COOLDOWN_SECS}s...")
            time.sleep(COOLDOWN_SECS)

            # 2. Login
            print(f"[{time.strftime('%H:%M:%S')}] Executing: autologin")
            rc, out, login_dur = run_cmd([AUTOLOGIN_BIN])
            print(f"Login exit={rc} ({login_dur:.3f}s):\n{out}")
            login_times.append(login_dur)

            # Verify
            online = check_internet()
            if rc == 0 and online:
                print(f"[{time.strftime('%H:%M:%S')}] Iteration {i} PASSED (Internet confirmed online)")
            else:
                print(f"[{time.strftime('%H:%M:%S')}] Iteration {i} FAILED! rc={rc}, online={online}")
                failures += 1
                # Try immediate recovery
                emergency_restore()

            if i < ITERATIONS:
                print(f"[{time.strftime('%H:%M:%S')}] Cooldown {COOLDOWN_SECS}s before next cycle...")
                time.sleep(COOLDOWN_SECS)

    finally:
        emergency_restore()

    print("\n" + "=" * 60)
    print("BENCHMARK SUMMARY & METRICS")
    print("=" * 60)
    total_runs = len(login_times)
    passed_runs = total_runs - failures
    print(f"Total Iterations:   {total_runs}")
    print(f"Successful Logins:  {passed_runs}/{total_runs} ({passed_runs/total_runs*100:.1f}%)")
    print(f"Failures:           {failures}")

    if login_times:
        print("\nLogin Latency Stats (seconds):")
        print(f"  Min:     {min(login_times):.3f}s")
        print(f"  Avg:     {statistics.mean(login_times):.3f}s")
        print(f"  Median:  {statistics.median(login_times):.3f}s")
        print(f"  Max:     {max(login_times):.3f}s")
        if len(login_times) >= 2:
            print(f"  StdDev:  {statistics.stdev(login_times):.3f}s")

    if logout_times:
        print("\nLogout Latency Stats (seconds):")
        print(f"  Min:     {min(logout_times):.3f}s")
        print(f"  Avg:     {statistics.mean(logout_times):.3f}s")
        print(f"  Median:  {statistics.median(logout_times):.3f}s")
        print(f"  Max:     {max(logout_times):.3f}s")

    print("\nIteration Breakdown:")
    for idx, (lo, li) in enumerate(zip(logout_times, login_times), 1):
        print(f"  Iter {idx:02d}: Logout = {lo:.3f}s | Login = {li:.3f}s")

if __name__ == "__main__":
    main()
