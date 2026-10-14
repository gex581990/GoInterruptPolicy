package main

import (
	"fmt"
	"math/rand"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// GetMaskValue reads a mask stored as each of the three types Microsoft
// allows for AssignmentSetOverride, and nothing from a value of another type
// or one that is not there. It writes a key of its own under
// HKEY_CURRENT_USER and deletes it again. The name is random, so two runs at
// once, or the leftover of a run that was killed, cannot share it.
func TestGetMaskValueReadsEveryMaskType(t *testing.T) {
	path := fmt.Sprintf(`Software\GoInterruptPolicyTest%016x`, rand.Uint64())

	key, existing, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if existing {
		key.Close()
		t.Fatalf("%s is already there, and is not this test's to delete", path)
	}
	defer func() {
		key.Close()
		if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
			t.Error(err)
		}
	}()

	for _, err := range []error{
		key.SetBinaryValue("binary", []byte{0x06}),
		key.SetDWordValue("dword", 0x06),
		key.SetQWordValue("qword", 0x06),
		key.SetStringValue("string", "6"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Decoded the way FindAllDevices decodes it.
	want := Set(Set(ZeroBit, CPUBits[1]), CPUBits[2])
	for _, name := range []string{"binary", "dword", "qword"} {
		bytes := make([]byte, 8)
		copy(bytes, GetMaskValue(key, name))
		if got := Bits(btoi64(bytes)); got != want {
			t.Errorf("%s: read as %b, want processors 1 and 2, %b", name, got, want)
		}
	}

	for _, name := range []string{"string", "missing"} {
		if data := GetMaskValue(key, name); len(data) != 0 {
			t.Errorf("%s: read as % x, want nothing", name, data)
		}
	}
}

// GetDWORDuint32Value reads a REG_DWORD, and nothing from a value of another
// type, which it once read as the number its bytes make: "4" as 52.
func TestGetDWORDuint32ValueReadsOnlyADword(t *testing.T) {
	path := fmt.Sprintf(`Software\GoInterruptPolicyTest%016x`, rand.Uint64())

	key, existing, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if existing {
		key.Close()
		t.Fatalf("%s is already there, and is not this test's to delete", path)
	}
	defer func() {
		key.Close()
		if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
			t.Error(err)
		}
	}()

	for _, err := range []error{
		key.SetDWordValue("dword", 4),
		key.SetStringValue("string", "4"),
		key.SetQWordValue("qword", 4),
		key.SetBinaryValue("binary", []byte{4}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	for name, want := range map[string]uint32{"dword": 4, "string": 0, "qword": 0, "binary": 0, "missing": 0} {
		if got := GetDWORDuint32Value(key, name); got != want {
			t.Errorf("%s: read as %d, want %d", name, got, want)
		}
	}
}
