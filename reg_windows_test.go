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

// MSI off deletes the device's MSI key, its limit with it, and the device
// forgets the limit too: kept, it was written back when MSI went on again in
// the same run. Also with no key to delete, which returns early.
func TestSetMSIModeOffForgetsTheLimit(t *testing.T) {
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
		for _, sub := range []string{`\Interrupt Management\MessageSignaledInterruptProperties`, `\Interrupt Management`, ""} {
			if err := registry.DeleteKey(registry.CURRENT_USER, path+sub); err != nil && err != registry.ErrNotExist {
				t.Error(err)
			}
		}
	}()

	d := Device{reg: key, MsiSupported: MSI_On, MessageNumberLimit: 8}
	d.setMSIMode(&Device{MsiSupported: MSI_Off})
	if d.MessageNumberLimit != 0 {
		t.Errorf("with no key: limit %d kept after MSI off", d.MessageNumberLimit)
	}

	d.setMSIMode(&Device{MsiSupported: MSI_On, MessageNumberLimit: 8})
	d.setMSIMode(&Device{MsiSupported: MSI_Off})
	if d.MessageNumberLimit != 0 {
		t.Errorf("limit %d kept after MSI off", d.MessageNumberLimit)
	}

	d.setMSIMode(&Device{MsiSupported: MSI_On, MessageNumberLimit: d.MessageNumberLimit})
	msi, err := registry.OpenKey(key, `Interrupt Management\MessageSignaledInterruptProperties`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer msi.Close()
	if _, _, err := msi.GetIntegerValue("MessageNumberLimit"); err != registry.ErrNotExist {
		t.Errorf("MSI on again wrote a limit: %v", err)
	}
}
