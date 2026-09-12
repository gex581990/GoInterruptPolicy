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
