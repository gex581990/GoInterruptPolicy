package main

// https://github.com/prometheus-community/windows_exporter/blob/74eac8f29b8083b9e6a4832d739748739e4e3fe0/headers/sysinfoapi/sysinfoapi.go#L44

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	// Library
	libKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	// Functions
	getSystemCpuSetInformation = libKernel32.NewProc("GetSystemCpuSetInformation")
)

// The SystemInformationClass constants have been derived from the SYSTEM_INFORMATION_CLASS enum definition.
const (
	SystemAllowedCpuSetsInformation = 0xA8
	SystemCpuSetInformation         = 0xAF

	ProcessDefaultCpuSetsInformation = 0x42

	// https://learn.microsoft.com/en-us/windows/win32/procthread/process-security-and-access-rights
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	PROCESS_SET_LIMITED_INFORMATION   = 0x2000
)

const (
	SYSTEM_CPU_SET_INFORMATION_PARKED                      uint32 = 0x1
	SYSTEM_CPU_SET_INFORMATION_ALLOCATED                   uint32 = 0x2
	SYSTEM_CPU_SET_INFORMATION_ALLOCATED_TO_TARGET_PROCESS uint32 = 0x4
	SYSTEM_CPU_SET_INFORMATION_REALTIME                    uint32 = 0x8
)

// https://github.com/zzl/go-win32api/blob/d9f481c2ab64b5df06e8d62e1bac33dd9141de43/win32/System.SystemInformation.go

type SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1_Anonymous struct {
	Bitfield_ byte
}

type SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1 struct {
	SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1_Anonymous
}

func (t *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1) AllFlags() *byte {
	return (*byte)(unsafe.Pointer(t))
}

func (t *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1) AllFlagsVal() byte {
	return *(*byte)(unsafe.Pointer(t))
}

func (t *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1) Anonymous() *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1_Anonymous {
	return (*SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1_Anonymous)(unsafe.Pointer(t))
}

func (t *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1) AnonymousVal() SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1_Anonymous {
	return *(*SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1_Anonymous)(unsafe.Pointer(t))
}

type SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous2 struct {
	Data [1]uint32
}

func (t *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous2) Reserved() *uint32 {
	return (*uint32)(unsafe.Pointer(t))
}

func (t *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous2) ReservedVal() uint32 {
	return *(*uint32)(unsafe.Pointer(t))
}

func (t *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous2) SchedulingClass() *byte {
	return (*byte)(unsafe.Pointer(t))
}

func (t *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous2) SchedulingClassVal() byte {
	return *(*byte)(unsafe.Pointer(t))
}

type SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet struct {
	Id                    uint32
	Group                 uint16
	LogicalProcessorIndex byte
	CoreIndex             byte
	LastLevelCacheIndex   byte
	NumaNodeIndex         byte
	EfficiencyClass       byte
	SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous1
	SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet_Anonymous2
	AllocationTag uint64
}

type SYSTEM_CPU_SET_INFORMATION_Anonymous struct {
	Data [3]uint64
}

func (t *SYSTEM_CPU_SET_INFORMATION_Anonymous) CpuSet() *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet {
	return (*SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet)(unsafe.Pointer(t))
}

func (t *SYSTEM_CPU_SET_INFORMATION_Anonymous) CpuSetVal() SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet {
	return *(*SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet)(unsafe.Pointer(t))
}

type CPU_SET_INFORMATION_TYPE int32

type SYSTEM_CPU_SET_INFORMATION struct {
	Size uint32
	Type CPU_SET_INFORMATION_TYPE
	SYSTEM_CPU_SET_INFORMATION_Anonymous
}

func GetSystemCpuSetInformation(
	information *SYSTEM_CPU_SET_INFORMATION,
	bufferLength uint32,
	returnedLength *uint32,
	process uintptr,
	flags uint32,
) uint32 {
	r1, _, _ := syscall.SyscallN(getSystemCpuSetInformation.Addr(),
		uintptr(unsafe.Pointer(information)),
		uintptr(bufferLength),
		uintptr(unsafe.Pointer(returnedLength)),
		process,
		uintptr(flags),
	)
	return uint32(r1)
}
