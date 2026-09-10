package main

import (
	"encoding/hex"
	"log"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// https://www.digitalocean.com/community/tutorials/using-ldflags-to-set-version-information-for-go-applications
var Version = "development"

type Device struct {
	Idata               DevInfoData
	reg                 registry.Key
	IrqPolicy           int32
	DeviceDesc          string
	DeviceIDs           []string
	DevObjName          string
	Driver              string
	LocationInformation string
	FriendlyName        string
	Class               string
	IRQLanes            []string

	// AffinityPolicy
	DevicePolicy          uint32
	DevicePriority        uint32
	AssignmentSetOverride Bits

	// MessageSignaledInterruptProperties
	MsiSupported       uint32
	MessageNumberLimit uint32
	MaxMSILimit        uint32
	InterruptTypeMap   Bits
}

func (d *Device) getInstanceID() (string, error) {
	n := uint32(0)
	setupDiGetDeviceInstanceId(handle, &d.Idata, nil, 0, &n)
	buff := make([]uint16, n)
	if err := setupDiGetDeviceInstanceId(handle, &d.Idata, unsafe.Pointer(&buff[0]), uint32(len(buff)), &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buff), nil
}

const (
	// https://docs.microsoft.com/en-us/windows-hardware/drivers/kernel/interrupt-affinity-and-priority
	IrqPolicyMachineDefault                    = iota // 0
	IrqPolicyAllCloseProcessors                       // 1
	IrqPolicyOneCloseProcessor                        // 2
	IrqPolicyAllProcessorsInMachine                   // 3
	IrqPolicySpecifiedProcessors                      // 4
	IrqPolicySpreadMessagesAcrossAllProcessors        // 5
)

const (
	MSI_Off uint32 = iota
	MSI_On
	MSI_Tristate
	MSI_Invalid
)

type Bits uint64

// maxProcessors is how many logical processors this tool can address. The
// AssignmentSetOverride it writes to the registry is a single 64 bit KAFFINITY
// affinity mask, and that mask covers processor group 0 only. Machines with
// more processors than that need GROUP_AFFINITY, which is not implemented.
const maxProcessors = 64

var CPUMap map[Bits]string

// CPUBits holds the affinity mask bit of every addressable logical processor,
// indexed by its group relative processor number. It always has maxProcessors
// entries, one per bit of the mask, so indexing it by a processor number that
// the CPU set information reports is safe.
var CPUBits []Bits
var InterruptTypeMap = map[Bits]string{
	0: "unknown",
	1: "LineBased",
	2: "Msi",

	4: "MsiX",
}

var sysInfo SystemInfo
var handle DevInfo

const ZeroBit = Bits(0)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	sysInfo = GetSystemInfo()
	CPUMap = make(map[Bits]string, maxProcessors)
	CPUBits = make([]Bits, 0, maxProcessors)
	for i := range maxProcessors {
		index := Bits(1) << i
		CPUMap[index] = strconv.Itoa(i)
		CPUBits = append(CPUBits, index)
	}
}

func Set(b, flag Bits) Bits    { return b | flag }
func Clear(b, flag Bits) Bits  { return b &^ flag }
func Toggle(b, flag Bits) Bits { return b ^ flag }
func Has(b, flag Bits) bool    { return b&flag != 0 }

// https://gist.github.com/chiro-hiro/2674626cebbcb5a676355b7aaac4972d
func i64tob(val uint64) []byte {
	r := make([]byte, 8)
	for i := range uint64(8) {
		r[i] = byte((val >> (i * 8)) & 0xff)
	}
	return r
}

func btoi64(val []byte) uint64 {
	r := uint64(0)
	for i := range uint64(8) {
		r |= uint64(val[i]) << (8 * i)
	}
	return r
}

func btoi32(val []byte) uint32 {
	r := uint32(0)
	for i := range uint32(4) {
		r |= uint32(val[i]) << (8 * i)
	}
	return r
}

func btoi16(val []byte) uint16 {
	r := uint16(0)
	for i := range uint16(2) {
		r |= uint16(val[i]) << (8 * i)
	}
	return r
}

func ToLittleEndian(v uint64) string {
	if v == 0 {
		return "00"
	}
	var b [8]byte
	i := 0
	for v > 0 {
		b[i] = byte(v & 0xff)
		v >>= 8
		i++
	}
	return hex.EncodeToString(b[:i])
}
