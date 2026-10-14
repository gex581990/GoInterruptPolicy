package main

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf16"
)

// decodeUTF16LE is the reader's half of regFileDocument, so a test can read
// back what it wrote.
func decodeUTF16LE(t *testing.T, b []byte) string {
	t.Helper()

	if len(b) < 2 || b[0] != 0xFF || b[1] != 0xFE {
		t.Fatalf("missing UTF-16LE byte order mark, got % x", b[:min(4, len(b))])
	}
	if (len(b)-2)%2 != 0 {
		t.Fatalf("odd number of bytes after the byte order mark")
	}

	units := make([]uint16, 0, (len(b)-2)/2)
	for i := 2; i < len(b); i += 2 {
		units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
	}

	return string(utf16.Decode(units))
}

const sampleSection = `[HKLM\Some\Path\Interrupt Management]

[HKLM\Some\Path\Interrupt Management\Affinity Policy]
"DevicePolicy"=dword:00000004
"AssignmentSetOverride"=hex:00,04
`

// regedit reads the first line and expects it verbatim. The header used to be
// concatenated after the newline conversion, so it kept bare newlines and the
// first "line" swallowed the rest of the file.
func TestRegFileDocumentHeaderEndsWithCRLF(t *testing.T) {
	text := decodeUTF16LE(t, regFileDocument(sampleSection))

	header, rest, found := strings.Cut(text, "\r\n")
	if !found {
		t.Fatal("no CRLF anywhere in the document")
	}
	if header != "Windows Registry Editor Version 5.00" {
		t.Errorf("first line = %q, want the registry editor header on its own", header)
	}
	if !strings.HasPrefix(rest, "\r\n[") {
		t.Errorf("after the header the document continues %q, want a blank CRLF line then a key", rest[:min(12, len(rest))])
	}
}

func TestRegFileDocumentUsesCRLFThroughout(t *testing.T) {
	text := decodeUTF16LE(t, regFileDocument(sampleSection))

	for i, r := range text {
		if r == '\n' && (i == 0 || text[i-1] != '\r') {
			t.Fatalf("bare newline at byte %d, every line has to end CRLF", i)
		}
		if r == '\r' && (i+1 >= len(text) || text[i+1] != '\n') {
			t.Fatalf("carriage return without a newline at byte %d", i)
		}
	}
}

// The conversion has to be idempotent: a section that already carries CRLF
// must not come out with doubled carriage returns.
func TestRegFileDocumentDoesNotDoubleCarriageReturns(t *testing.T) {
	crlf := strings.ReplaceAll(sampleSection, "\n", "\r\n")

	if got, want := regFileDocument(crlf), regFileDocument(sampleSection); !bytes.Equal(got, want) {
		t.Errorf("a section already using CRLF produced a different document than the same section using newlines")
	}
	if strings.Contains(decodeUTF16LE(t, regFileDocument(crlf)), "\r\r") {
		t.Error("doubled carriage returns")
	}
}

func TestRegFileDocumentRoundTripsContent(t *testing.T) {
	text := decodeUTF16LE(t, regFileDocument(sampleSection))

	for _, want := range []string{
		`[HKLM\Some\Path\Interrupt Management\Affinity Policy]`,
		`"DevicePolicy"=dword:00000004`,
		`"AssignmentSetOverride"=hex:00,04`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("document is missing %q", want)
		}
	}
}

const samplePath = `HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Enum\PCI\X\Device Parameters`

// The whole of one device's section, so that any change to its layout shows.
func TestRegFileSection(t *testing.T) {
	got := createRegFile(nil, samplePath, Device{
		DevicePolicy:          4,
		DevicePriority:        3,
		AssignmentSetOverride: 0xA005,
		MsiSupported:          MSI_On,
		MessageNumberLimit:    16,
	})

	want := `[` + samplePath + `\Interrupt Management]

[` + samplePath + `\Interrupt Management\Affinity Policy]
"DevicePolicy"=dword:00000004
"DevicePriority"=dword:00000003
"AssignmentSetOverride"=hex:05,a0

[` + samplePath + `\Interrupt Management\MessageSignaledInterruptProperties]
"MSISupported"=dword:00000001
"MessageNumberLimit"=dword:00000010

`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// A .reg file is read with the number after "dword:" in hex. Written in
// decimal, an MSI limit of 16 came back from a backup as 22, and 2048 as 8264.
func TestRegFileWritesDwordsInHex(t *testing.T) {
	for limit, want := range map[uint32]string{
		8:    `"MessageNumberLimit"=dword:00000008`,
		10:   `"MessageNumberLimit"=dword:0000000a`,
		16:   `"MessageNumberLimit"=dword:00000010`,
		2048: `"MessageNumberLimit"=dword:00000800`,
	} {
		section := createRegFile(nil, samplePath, Device{MsiSupported: MSI_On, MessageNumberLimit: limit})
		if !strings.Contains(section, want+"\n") {
			t.Errorf("a limit of %d is not written as %s:\n%s", limit, want, section)
		}
	}
}

// A backup of a device without an MSI limit removes any limit set since,
// instead of leaving it there when restored.
func TestRegFileRemovesALimitThatWasNotSet(t *testing.T) {
	section := createRegFile(nil, samplePath, Device{MsiSupported: MSI_On})
	if !strings.Contains(section, `"MessageNumberLimit"=-`+"\n") {
		t.Errorf("no limit is not written as one to remove:\n%s", section)
	}
}

// A device that reports no interrupt types has no MSI settings to back up,
// and an MSISupported of 3 would only be an invalid one restored.
func TestRegFileLeavesOutMSIForADeviceWithout(t *testing.T) {
	section := createRegFile(nil, samplePath, Device{DevicePolicy: 1, MsiSupported: MSI_Invalid})
	if strings.Contains(section, "MessageSignaledInterruptProperties") || strings.Contains(section, "MSISupported") {
		t.Errorf("MSI settings written for a device without any:\n%s", section)
	}
	if !strings.HasSuffix(section, "=-\n\n") {
		t.Errorf("the section does not end on its last value and a blank line:\n%q", section)
	}
}

// The name an export offers is the device's description, which can hold
// characters Windows does not allow in a file name.
func TestRegFileNameHasOnlyCharactersAFileNameMay(t *testing.T) {
	for desc, want := range map[string]string{
		"High Definition Audio Controller": "High_Definition_Audio_Controller.reg",
		"Intel(R) Xeon(R) E3 - 1200/1500 v5/6th Gen Intel(R) Core(TM) PCIe Controller (x16) - 1901": "Intel(R)_Xeon(R)_E3_-_1200_1500_v5_6th_Gen_Intel(R)_Core(TM)_PCIe_Controller_(x16)_-_1901.reg",
		`a\b:c*d?e"f<g>h|i`: "a_b_c_d_e_f_g_h_i.reg",
		"tab\there":         "tab_here.reg",
	} {
		if got := regFileName(desc); got != want {
			t.Errorf("regFileName(%q) = %q, want %q", desc, got, want)
		}
	}
}
