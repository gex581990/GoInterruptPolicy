package main

import (
	"fmt"
	"log"

	"syscall"
	"unsafe"

	"github.com/tailscale/walk"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modSetupapi = windows.NewLazySystemDLL("setupapi.dll")

	procSetupDiGetClassDevsW = modSetupapi.NewProc("SetupDiGetClassDevsW")
)

const CONFIG_FLAG_DISABLED uint32 = 1

func FindAllDevices() ([]Device, DevInfo) {
	var allDevices []Device
	handle, err := SetupDiGetClassDevs(nil, nil, 0, uint32(DIGCF_ALLCLASSES|DIGCF_PRESENT))
	if err != nil {
		walk.MsgBox(nil, "FindAllDevices Error", err.Error(), walk.MsgBoxIconError)
		panic(err)
	}

	var index = 0
	for {
		idata, err := SetupDiEnumDeviceInfo(handle, index)
		if err != nil { // ERROR_NO_MORE_ITEMS
			break
		}
		index++

		hasIRQ, irqs, err := DevNodeHasIRQ(idata.DevInst)
		if err != nil {
			log.Printf("Error: %v\n", err)
			continue
		}
		if !hasIRQ {
			continue
		}

		dev := Device{
			Idata: *idata,
		}

		lines := make([]string, len(irqs))
		for i, line := range irqs {
			if line > 0x80000000 {
				lines[i] = fmt.Sprintf("-%d", 0x100000000-uint64(line))
			} else {
				lines[i] = fmt.Sprintf("%d", line)
			}
		}
		dev.IRQLanes = lines

		val, err := SetupDiGetDeviceRegistryProperty(handle, idata, SPDRP_CONFIGFLAGS)
		if err == nil {
			if val.(uint32)&CONFIG_FLAG_DISABLED != 0 {
				// Sorts out deactivated devices
				continue
			}
		}

		val, err = SetupDiGetDeviceRegistryProperty(handle, idata, SPDRP_DEVICEDESC)
		if err == nil {
			if v, ok := val.(string); ok {
				if v == "" {
					continue
				}
				dev.DeviceDesc = v
			}
		} else {
			continue
		}

		valProp, err := GetDeviceProperty(handle, idata, DEVPKEY_PciDevice_InterruptSupport)
		if err == nil {
			dev.InterruptTypeMap = Bits(btoi16(valProp))
		}

		valProp, err = GetDeviceProperty(handle, idata, DEVPKEY_PciDevice_InterruptMessageMaximum)
		if err == nil {
			dev.MaxMSILimit = btoi32(valProp)
		}

		val, err = SetupDiGetDeviceRegistryProperty(handle, idata, SPDRP_FRIENDLYNAME)
		if err == nil {
			if v, ok := val.(string); ok {
				dev.FriendlyName = v
			}
		}

		val, err = SetupDiGetDeviceRegistryProperty(handle, idata, SPDRP_PHYSICAL_DEVICE_OBJECT_NAME)
		if err == nil {
			if v, ok := val.(string); ok {
				dev.DevObjName = v
			}
		}

		val, err = SetupDiGetDeviceRegistryProperty(handle, idata, SPDRP_LOCATION_INFORMATION)
		if err == nil {
			if v, ok := val.(string); ok {
				dev.LocationInformation = v
			}
		}

		val, err = SetupDiGetDeviceRegistryProperty(handle, idata, SPDRP_CLASS)
		if err == nil {
			if v, ok := val.(string); ok {
				dev.Class = v
			}
		}

		dev.reg, _ = SetupDiOpenDevRegKey(handle, idata, DICS_FLAG_GLOBAL, 0, DIREG_DEV, windows.KEY_SET_VALUE)

		affinityPolicyKey, _ := registry.OpenKey(dev.reg, `Interrupt Management\Affinity Policy`, registry.QUERY_VALUE)
		dev.DevicePolicy = GetDWORDuint32Value(affinityPolicyKey, "DevicePolicy")             // REG_DWORD
		dev.DevicePriority = GetDWORDuint32Value(affinityPolicyKey, "DevicePriority")         // REG_DWORD
		AssignmentSetOverrideByte := GetMaskValue(affinityPolicyKey, "AssignmentSetOverride") // REG_BINARY, REG_DWORD or REG_QWORD
		affinityPolicyKey.Close()

		if len(AssignmentSetOverrideByte) != 0 {
			AssignmentSetOverrideBytes := make([]byte, 8)
			copy(AssignmentSetOverrideBytes, AssignmentSetOverrideByte)
			dev.AssignmentSetOverride = Bits(btoi64(AssignmentSetOverrideBytes))
		}

		if dev.InterruptTypeMap != ZeroBit {
			messageSignaledInterruptPropertiesKey, _ := registry.OpenKey(dev.reg, `Interrupt Management\MessageSignaledInterruptProperties`, registry.QUERY_VALUE)
			dev.MessageNumberLimit = GetDWORDuint32Value(messageSignaledInterruptPropertiesKey, "MessageNumberLimit") // REG_DWORD https://docs.microsoft.com/de-de/windows-hardware/drivers/kernel/enabling-message-signaled-interrupts-in-the-registry
			dev.MsiSupported = GetDWORDuint32Value(messageSignaledInterruptPropertiesKey, "MSISupported")             // REG_DWORD
			messageSignaledInterruptPropertiesKey.Close()
		} else {
			dev.MsiSupported = MSI_Invalid
		}

		allDevices = append(allDevices, dev)
	}
	return allDevices, handle
}

func SetupDiGetClassDevs(classGuid *windows.GUID, enumerator *uint16, hwndParent uintptr, flags uint32) (handle DevInfo, err error) {
	r0, _, e1 := syscall.SyscallN(procSetupDiGetClassDevsW.Addr(), uintptr(unsafe.Pointer(classGuid)), uintptr(unsafe.Pointer(enumerator)), uintptr(hwndParent), uintptr(flags))
	handle = DevInfo(r0)
	if handle == DevInfo(windows.InvalidHandle) {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func GetDeviceProperty(dis DevInfo, devInfoData *DevInfoData, devPropKey DEVPROPKEY) ([]byte, error) {
	var propt, size uint32
	buf := make([]byte, 16)
	run := true
	for run {
		err := SetupDiGetDeviceProperty(dis, devInfoData, &devPropKey, &propt, &buf[0], uint32(len(buf)), &size, 0)
		switch {
		case size > uint32(len(buf)):
			buf = make([]byte, size+16)
		case err != nil:
			return buf, err
		default:
			run = false
		}
	}

	return buf, nil
}

var (
	cfgmgr32 = windows.NewLazySystemDLL("cfgmgr32.dll")

	procCM_Get_First_Log_Conf    = cfgmgr32.NewProc("CM_Get_First_Log_Conf")
	procCM_Get_Next_Res_Des      = cfgmgr32.NewProc("CM_Get_Next_Res_Des")
	procCM_Get_Res_Des_Data_Size = cfgmgr32.NewProc("CM_Get_Res_Des_Data_Size")
	procCM_Get_Res_Des_Data      = cfgmgr32.NewProc("CM_Get_Res_Des_Data")
	procCM_Free_Res_Des_Handle   = cfgmgr32.NewProc("CM_Free_Res_Des_Handle")
	procCM_Free_Log_Conf_Handle  = cfgmgr32.NewProc("CM_Free_Log_Conf_Handle")
)

const (
	CR_SUCCESS          uintptr = 0x00000000
	CR_NO_MORE_LOG_CONF uintptr = 0x0000000F
	CR_NO_MORE_RES_DES  uintptr = 0x0000000E
	CR_INVALID_RES_DES  uintptr = 0x00000006

	ALLOC_LOG_CONF    uintptr = 0x00000002
	FILTERED_LOG_CONF uintptr = 0x00000001
	BASIC_LOG_CONF    uintptr = 0x00000000
	BOOT_LOG_CONF     uintptr = 0x00000003

	ResType_IRQ uintptr = 0x00000004
)

type IRQ_DES struct {
	IRQD_Count     uint32
	IRQD_Type      uint32
	IRQD_Flags     uint32
	IRQD_Alloc_Num uint32
	IRQD_Affinity  uint64
}

func DevNodeHasIRQ(devInst uint32) (bool, []uint32, error) {
	for _, confType := range []uintptr{ALLOC_LOG_CONF, FILTERED_LOG_CONF, BASIC_LOG_CONF} {
		var logConf uintptr
		// https://learn.microsoft.com/en-us/windows/win32/api/cfgmgr32/nf-cfgmgr32-cm_get_first_log_conf
		ret, _, _ := procCM_Get_First_Log_Conf.Call(
			uintptr(unsafe.Pointer(&logConf)),
			uintptr(devInst),
			confType,
		)
		if ret != CR_SUCCESS {
			continue
		}

		irqs, err := collectIRQsFromLogConf(logConf)
		// https://learn.microsoft.com/en-us/windows/win32/api/cfgmgr32/nf-cfgmgr32-cm_free_log_conf_handle
		procCM_Free_Log_Conf_Handle.Call(logConf)
		if err != nil {
			return false, nil, err
		}
		if len(irqs) > 0 {
			return true, irqs, nil
		}
	}
	return false, nil, nil
}

func collectIRQsFromLogConf(logConf uintptr) ([]uint32, error) {
	var irqNumbers []uint32

	var firstResDes uintptr
	// https://learn.microsoft.com/en-us/windows/win32/api/cfgmgr32/nf-cfgmgr32-cm_get_next_res_des
	ret, _, _ := procCM_Get_Next_Res_Des.Call(
		uintptr(unsafe.Pointer(&firstResDes)),
		logConf,
		ResType_IRQ,
		0,
		0,
	)

	if ret == CR_NO_MORE_RES_DES || ret == CR_NO_MORE_LOG_CONF {
		return nil, nil
	}

	if ret != CR_SUCCESS {
		return nil, fmt.Errorf("CM_Get_Next_Res_Des (first): CONFIGRET 0x%X", ret)
	}

	currentResDes := firstResDes
	for {
		irq, err := readIRQResDes(currentResDes)
		if err != nil {
			// https://learn.microsoft.com/en-us/windows/win32/api/cfgmgr32/nf-cfgmgr32-cm_free_res_des_handle
			procCM_Free_Res_Des_Handle.Call(currentResDes)
			return irqNumbers, err
		}
		irqNumbers = append(irqNumbers, *irq)

		var nextResDes uintptr
		ret, _, _ = procCM_Get_Next_Res_Des.Call(
			uintptr(unsafe.Pointer(&nextResDes)),
			currentResDes,
			ResType_IRQ,
			0,
			0,
		)

		procCM_Free_Res_Des_Handle.Call(currentResDes)

		if ret == CR_NO_MORE_RES_DES || ret == CR_NO_MORE_LOG_CONF {
			break
		}
		if ret != CR_SUCCESS {
			return irqNumbers, fmt.Errorf("CM_Get_Next_Res_Des CONFIGRET 0x%X", ret)
		}

		currentResDes = nextResDes
	}

	return irqNumbers, nil
}

func readIRQResDes(resDes uintptr) (*uint32, error) {
	var dataSize uint32
	// https://learn.microsoft.com/en-us/windows/win32/api/cfgmgr32/nf-cfgmgr32-cm_get_res_des_data_size
	ret, _, _ := procCM_Get_Res_Des_Data_Size.Call(
		uintptr(unsafe.Pointer(&dataSize)),
		resDes,
		0,
	)

	if ret != CR_SUCCESS {
		return nil, fmt.Errorf("CONFIGRET 0x%X", ret)
	}
	if dataSize < uint32(unsafe.Sizeof(IRQ_DES{})) {
		return nil, fmt.Errorf("IRQ_DES too small: %d bytes", dataSize)
	}

	buf := make([]byte, dataSize)
	ret, _, _ = procCM_Get_Res_Des_Data.Call(
		resDes,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(dataSize),
		0,
	)
	if ret != CR_SUCCESS {
		return nil, fmt.Errorf("CM_Get_Res_Des_Data CONFIGRET 0x%X", ret)
	}

	irqDes := (*IRQ_DES)(unsafe.Pointer(&buf[0]))
	// log.Printf("[DEBUG] Count=%d Type=%d Flags=0x%X Alloc_Num=%d Affinity=0x%X\n",
	// 	irqDes.IRQD_Count,
	// 	irqDes.IRQD_Type,
	// 	irqDes.IRQD_Flags,
	// 	irqDes.IRQD_Alloc_Num,
	// 	irqDes.IRQD_Affinity,
	// )

	return &irqDes.IRQD_Alloc_Num, nil
}
