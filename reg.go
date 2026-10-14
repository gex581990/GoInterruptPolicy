package main

import (
	"log"
	"os/exec"
	"strings"

	"github.com/tailscale/walk"
	"golang.org/x/sys/windows/registry"
)

func clen(n []byte) int {
	for i := len(n) - 1; i >= 0; i-- {
		if n[i] != 0 {
			return i + 1
		}
	}
	return len(n)
}

func GetStringValue(key registry.Key, name string) string {
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}

// GetMaskValue reads a processor mask such as AssignmentSetOverride, which
// Microsoft documents as a REG_BINARY, a REG_DWORD or a REG_QWORD value. All
// three keep the mask's lowest byte first, so their bytes are read the same
// way. It replaces GetBinaryValue, which read the first type only, so a mask
// stored as either of the others came back as no processors at all.
func GetMaskValue(key registry.Key, name string) []byte {
	size, valtype, err := key.GetValue(name, nil)
	if err != nil || size == 0 || !isMaskType(valtype) {
		return []byte{}
	}

	buf := make([]byte, size)
	n, _, err := key.GetValue(name, buf)
	if err != nil {
		return []byte{}
	}

	return buf[:n]
}

// isMaskType says whether a registry value of type valtype can hold a
// processor mask.
func isMaskType(valtype uint32) bool {
	switch valtype {
	case registry.BINARY, registry.DWORD, registry.QWORD:
		return true
	}

	return false
}

// GetDWORDuint32Value reads a REG_DWORD, which is the type Microsoft
// documents for every setting this program reads with it, and 0, which they
// all take for not set, for anything else. A value of another type used to be
// read as the number its bytes make, a REG_SZ "4" as 52, and written back,
// to the registry and to an export, as a REG_DWORD of that number.
func GetDWORDuint32Value(key registry.Key, name string) uint32 {
	buf := make([]byte, 4)
	if n, valtype, err := key.GetValue(name, buf); err != nil || n != 4 || valtype != registry.DWORD {
		return 0
	}
	return btoi32(buf)
}

func (d *Device) setMSIMode(item *Device) (changed bool) {
	var k registry.Key
	var err error

	switch item.MsiSupported {
	case MSI_Off:
		d.MsiSupported = MSI_Off
		// The key goes, and the limit in it. Kept here, it came back when MSI
		// was turned on again in the same run.
		d.MessageNumberLimit = 0
		k, err = registry.OpenKey(d.reg, `Interrupt Management\MessageSignaledInterruptProperties`, registry.ALL_ACCESS)
		if err != nil {
			log.Println(err)
			return
		}
		if err := registry.DeleteKey(d.reg, `Interrupt Management\MessageSignaledInterruptProperties`); err != nil {
			log.Println(err)
		}
		changed = true
	case MSI_On:
		k, _, err = registry.CreateKey(d.reg, `Interrupt Management\MessageSignaledInterruptProperties`, registry.ALL_ACCESS)
		if err != nil {
			log.Println(err)
			return
		}
		if err := k.SetDWordValue("MSISupported", 1); err != nil {
			log.Println(err)
		}
		d.MsiSupported = MSI_On

		if item.MessageNumberLimit == 0 {
			if err := k.DeleteValue("MessageNumberLimit"); err != nil {
				log.Println(err)
			}
			d.MessageNumberLimit = 0
		} else {
			if err := k.SetDWordValue("MessageNumberLimit", item.MessageNumberLimit); err != nil {
				log.Println(err)
			}
			d.MessageNumberLimit = item.MessageNumberLimit
		}
		changed = true
	}
	if err := k.Close(); err != nil {
		log.Println(err)
	}
	return
}

func (d *Device) setAffinityPolicy(item *Device) (changed bool) {
	var k registry.Key
	var err error
	if item.DevicePolicy == 0 && item.DevicePriority == 0 {
		d.DevicePolicy = 0
		d.DevicePriority = 0
		d.AssignmentSetOverride = ZeroBit
		k, err = registry.OpenKey(d.reg, `Interrupt Management\Affinity Policy`, registry.ALL_ACCESS)
		if err != nil {
			log.Println(err)
			return
		}
		defer k.Close()

		if err := registry.DeleteKey(d.reg, `Interrupt Management\Affinity Policy`); err != nil {
			log.Println(err)
		}
		changed = true
	} else {
		k, _, err = registry.CreateKey(d.reg, `Interrupt Management\Affinity Policy`, registry.ALL_ACCESS)
		if err != nil {
			log.Println(err)
			return
		}
		defer k.Close()

		if err := k.SetDWordValue("DevicePolicy", item.DevicePolicy); err != nil {
			log.Println(err)
		}
		d.DevicePolicy = item.DevicePolicy

		if item.DevicePolicy != 4 { // IrqPolicySpecifiedProcessors
			k.DeleteValue("AssignmentSetOverride")
			d.AssignmentSetOverride = ZeroBit
		} else {
			AssignmentSetOverrideByte := i64tob(uint64(item.AssignmentSetOverride))
			if err := k.SetBinaryValue("AssignmentSetOverride", AssignmentSetOverrideByte[:clen(AssignmentSetOverrideByte)]); err != nil {
				log.Println(err)
			}
			d.AssignmentSetOverride = item.AssignmentSetOverride
		}

		if item.DevicePriority == 0 {
			k.DeleteValue("DevicePriority")
		} else if err := k.SetDWordValue("DevicePriority", item.DevicePriority); err != nil {
			log.Println(err)
		}
		d.DevicePriority = item.DevicePriority

		changed = true
	}
	return
}

// \REGISTRY\MACHINE\
func replaceRegistryMachine(regPath string) string {
	_, regPathAfter, found := strings.Cut(regPath, "\\REGISTRY\\MACHINE\\")
	if !found {
		log.Println("not Found")
		return ""
	}
	return regPathAfter
}

// replaces ControlSet00X with CurrentControlSet
func generalizeControlSet(regPath string) string {
	// https://learn.microsoft.com/en-us/windows-hardware/drivers/install/hklm-system-currentcontrolset-control-registry-tree

	regPathArray := strings.Split(regPath, "\\")
	for i := 0; i < len(regPathArray); i++ {
		if strings.HasPrefix(regPathArray[i], "ControlSet00") {
			regPathArray[i] = "CurrentControlSet"
			return strings.Join(regPathArray, "\\")
		}
	}
	return strings.Join(regPathArray, "\\")
}

func OpenRegistry(dlg walk.Form, reg registry.Key) {
	regPath, err := GetRegistryLocation(uintptr(reg))
	if err != nil {
		walk.MsgBox(dlg, "NtQueryKey Error", err.Error(), walk.MsgBoxIconError)
		return
	}

	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Applets\Regedit`, registry.SET_VALUE)
	if err != nil {
		walk.MsgBox(dlg, "Registry Error", err.Error(), walk.MsgBoxIconError)
		log.Fatal(err)
	}
	defer k.Close()

	if err := k.SetStringValue("LastKey", regPath); err == nil {
		exec.Command("regedit", "-m").Start()
	}
}
