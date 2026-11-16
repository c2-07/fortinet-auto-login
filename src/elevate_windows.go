//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	modadvapi32            = syscall.NewLazyDLL("advapi32.dll")
	modshell32             = syscall.NewLazyDLL("shell32.dll")
	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcessToken   = modadvapi32.NewProc("OpenProcessToken")
	procGetTokenInfo       = modadvapi32.NewProc("GetTokenInformation")
	procShellExecuteExW    = modshell32.NewProc("ShellExecuteExW")
	procGetExitCodeProcess = modkernel32.NewProc("GetExitCodeProcess")
)

const (
	tokenQuery            = 0x0008
	tokenClassElevation   = 20
	seeMaskNoCloseProcess = 0x00000040
	swHide                = 0
)

type tokenElevation struct {
	TokenIsElevated uint32
}

type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           uintptr
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       uintptr
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      uintptr
	dwHotKey       uint32
	hIconOrMonitor uintptr
	hProcess       syscall.Handle
}

// check if process is running elevated
func isAdmin() bool {
	var token syscall.Token
	handle := syscall.Handle(^uintptr(0))
	err := syscall.OpenProcessToken(handle, tokenQuery, &token)
	if err != nil {
		return false
	}
	defer token.Close()

	var elevation tokenElevation
	var returnedLen uint32
	r1, _, _ := procGetTokenInfo.Call(
		uintptr(token),
		uintptr(tokenClassElevation),
		uintptr(unsafe.Pointer(&elevation)),
		uintptr(unsafe.Sizeof(elevation)),
		uintptr(unsafe.Pointer(&returnedLen)),
	)
	if r1 == 0 {
		return false
	}
	return elevation.TokenIsElevated != 0
}

// relaunch via runas verb and wait for child exit
func runElevated() (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}

	args := strings.Join(os.Args[1:], " ")
	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	argPtr, _ := syscall.UTF16PtrFromString(args)

	sei := shellExecuteInfo{
		cbSize:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       verbPtr,
		lpFile:       exePtr,
		lpParameters: argPtr,
		nShow:        swHide,
	}

	r1, _, errSys := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	if r1 == 0 {
		return false, errSys
	}
	defer syscall.CloseHandle(sei.hProcess)

	_, _ = syscall.WaitForSingleObject(sei.hProcess, syscall.INFINITE)

	var exitCode uint32
	_, _, _ = procGetExitCodeProcess.Call(uintptr(sei.hProcess), uintptr(unsafe.Pointer(&exitCode)))
	return exitCode == 0, nil
}

// prompt uac if not admin
func ensureAdmin(action string) bool {
	if os.Getenv("AUTOLOGIN_SKIP_ELEVATION") == "1" {
		return false
	}
	if isAdmin() {
		return false
	}

	fmt.Println("\033[36m[INFO]\033[0m   Administrator privileges required. Requesting elevation...")
	success, err := runElevated()
	if err != nil {
		if strings.Contains(err.Error(), "1223") || strings.Contains(err.Error(), "5") {
			fmt.Println("\033[31m[FAIL]\033[0m   UAC elevation was declined by the user.")
		} else {
			fmt.Printf("\033[31m[FAIL]\033[0m   Failed to request administrator privileges: %v\n", err)
		}
		return true
	}

	if !success {
		fmt.Printf("\033[31m[FAIL]\033[0m   Elevated service command failed.\n")
		return true
	}

	if action == "install" {
		logPath := getLogFilePath()
		fmt.Println("\033[32m[OK]\033[0m     Windows Task Scheduler background service installed and started!")
		fmt.Println("\033[36m[INFO]\033[0m   Runs automatically at logon with 45s session watchdog")
		fmt.Printf("\033[36m[INFO]\033[0m   Logs: %s\n", logPath)
	} else if action == "uninstall" {
		fmt.Println("\033[32m[OK]\033[0m     Windows background service uninstalled successfully")
	}

	return true
}
