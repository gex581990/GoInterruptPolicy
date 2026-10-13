package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

var (
	devmgr         = windows.NewLazySystemDLL("devmgr.dll")
	procDeviceProp = devmgr.NewProc("DeviceProperties_RunDLLW")
)

// https://learn.microsoft.com/en-us/windows-hardware/drivers/install/deviceproperties-rundll-function-prototype
func showDeviceProperties(hwnd win.HWND, deviceID string) {
	// https://learn.microsoft.com/en-us/windows-hardware/drivers/install/invoking-a-device-properties-dialog-box-from-a-command-line-prompt
	// https://support.microsoft.com/en-us/topic/how-to-invoke-the-device-properties-dialog-box-from-the-application-or-from-a-command-prompt-ca8ba122-ec37-2bbe-432d-6ff831f05fcd
	//
	// The function returns nothing, so there is no result to check. What can
	// fail is finding it, and Call would panic on that, so find it first.
	if err := procDeviceProp.Find(); err != nil {
		walk.MsgBox(nil, "ShowDeviceProperties Error", err.Error(), walk.MsgBoxIconError)
		fmt.Printf("%v\n", err)
		return
	}

	cmdLine, _ := syscall.UTF16PtrFromString(fmt.Sprintf("/DeviceID %s", deviceID))
	procDeviceProp.Call(
		uintptr(hwnd),
		0,
		uintptr(unsafe.Pointer(cmdLine)),
		win.SW_SHOWNORMAL,
	)
}
