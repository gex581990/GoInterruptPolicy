package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"strings"
	"text/template"
	"unicode/utf16"

	"github.com/tailscale/walk"
)

const REG_FILE_HEADER = "Windows Registry Editor Version 5.00\n\n"

var packageInfo = template.New("packageInfo")
var tmplProperty = template.Must(packageInfo.Parse(string(`[{{.RegPath}}\Interrupt Management]

[{{.RegPath}}\Interrupt Management\Affinity Policy]
"DevicePolicy"=dword:{{printf "%08d" .Device.DevicePolicy}}
{{if eq .Device.DevicePriority 0}}"DevicePriority"=-{{else}}"DevicePriority"=dword:{{printf "%08d" .Device.DevicePriority}}{{end}}
{{if ne .Device.DevicePolicy 4}}"AssignmentSetOverride"=-{{else}}"AssignmentSetOverride"=hex:{{.AssignmentSetOverride}}{{end}}

[{{.RegPath}}\Interrupt Management\MessageSignaledInterruptProperties]
"MSISupported"=dword:{{printf "%08d" .Device.MsiSupported}}
{{if eq .Device.MsiSupported 1}}{{if ne .Device.MessageNumberLimit 0}}"MessageNumberLimit"=dword:{{printf "%08d" .Device.MessageNumberLimit}}{{end}}{{else}}"MessageNumberLimit"=-{{end}}

`)))

func createRegFile(dlg *walk.Dialog, regpath string, item Device) string {
	var buf bytes.Buffer
	err := tmplProperty.Execute(&buf, struct {
		RegPath               string
		Device                Device
		AssignmentSetOverride string
	}{
		regpath,
		item,
		addComma(ToLittleEndian(uint64(item.AssignmentSetOverride))),
	})

	if err != nil {
		walk.MsgBox(dlg, "CreateRegFile Error", err.Error(), walk.MsgBoxIconError)
		log.Fatalln(err)
	}

	// Plain newlines here. regFileDocument turns the whole document, header
	// included, into the line endings a .reg file needs; doing it per section
	// was what left the header behind with bare newlines.
	return buf.String()
}

// regFileDocument turns exported device sections into the bytes of a .reg
// file, in the shape regedit itself writes: the header, CRLF line endings
// throughout, and UTF-16 little endian with a byte order mark.
//
// Both of those matter. A parser checks that the first line reads exactly
// "Windows Registry Editor Version 5.00", so a header ending in a bare newline
// leaves it reading the rest of the file as part of that line and calling the
// file invalid.
func regFileDocument(body string) []byte {
	// Fold any carriage returns back out first, so a document that already has
	// them does not come out with doubled ones.
	text := strings.ReplaceAll(REG_FILE_HEADER+body, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n", "\r\n")

	units := utf16.Encode([]rune(text))

	out := make([]byte, 0, 2+len(units)*2)
	out = append(out, 0xFF, 0xFE) // UTF-16LE byte order mark
	for _, u := range units {
		out = binary.LittleEndian.AppendUint16(out, u)
	}

	return out
}

func addComma(data string) string {
	var b strings.Builder
	for i := 0; i < len(data); i++ {
		if i != 0 && i%2 == 0 {
			b.WriteString(",")
		}
		b.WriteString(string(data[i]))
	}

	return b.String()
}

func saveFileExplorer(owner walk.Form, path, filename, title, filter string) (filePath string, cancel bool, err error) {
	dlg := new(walk.FileDialog)

	dlg.Title = title
	dlg.InitialDirPath = path
	dlg.Filter = filter
	dlg.FilePath = filename

	ok, err := dlg.ShowSave(owner)
	if err != nil {
		return "", !ok, err
	} else if !ok {
		return "", !ok, nil
	}

	return dlg.FilePath, !ok, nil
}
