package main

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// Microsoft documents AssignmentSetOverride as a REG_BINARY, a REG_DWORD or a
// REG_QWORD, all with the mask's lowest byte first. isMaskType takes those
// three, and the bytes of each come out as the same processors through the
// decoding FindAllDevices does, copying them into eight bytes and reading
// those. Reading them out of the registry is reg_windows_test.go's.
func TestEveryMaskTypeDecodesTheSame(t *testing.T) {
	tests := []struct {
		name    string
		valtype uint32
		data    []byte
	}{
		{name: "REG_BINARY, as this program writes it", valtype: registry.BINARY, data: []byte{0x06}},
		{name: "REG_BINARY of eight bytes", valtype: registry.BINARY, data: []byte{0x06, 0, 0, 0, 0, 0, 0, 0}},
		{name: "REG_DWORD", valtype: registry.DWORD, data: []byte{0x06, 0, 0, 0}},
		{name: "REG_QWORD", valtype: registry.QWORD, data: []byte{0x06, 0, 0, 0, 0, 0, 0, 0}},
	}

	for _, tt := range tests {
		if !isMaskType(tt.valtype) {
			t.Errorf("%s: not taken for a mask", tt.name)
			continue
		}

		bytes := make([]byte, 8)
		copy(bytes, tt.data)
		if got, want := Bits(btoi64(bytes)), Set(Set(ZeroBit, CPUBits[1]), CPUBits[2]); got != want {
			t.Errorf("%s: read as %b, want processors 1 and 2, %b", tt.name, got, want)
		}
	}

	if isMaskType(registry.SZ) {
		t.Error("a string taken for a mask")
	}
}
