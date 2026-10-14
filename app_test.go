package main

import (
	"strings"
	"testing"
)

func TestCLIRefusals(t *testing.T) {
	const threads = 8

	onNone := Device{DevicePolicy: IrqPolicyAllCloseProcessors, MsiSupported: MSI_Off}
	onTwo := Device{DevicePolicy: IrqPolicySpecifiedProcessors, AssignmentSetOverride: 0b0110, MsiSupported: MSI_On}
	// Left on specified processors with none, as the command line used to
	// leave a device it was given -policy 4 alone for.
	leftEmpty := Device{DevicePolicy: IrqPolicySpecifiedProcessors, MsiSupported: MSI_On}
	noMSI := Device{MsiSupported: MSI_Invalid}
	// MSISupported 2, which no tool should write, with a limit stored.
	oddMSI := Device{MsiSupported: 2, MessageNumberLimit: 4}
	msiX := Device{InterruptTypeMap: 0b111, MsiSupported: MSI_On}
	// Values Windows does not document, as another tool could have stored.
	oddPolicy := Device{DevicePolicy: 7, MsiSupported: MSI_Off}
	oddPriority := Device{DevicePolicy: IrqPolicyAllCloseProcessors, DevicePriority: 9, MsiSupported: MSI_Off}
	limit3Off := Device{InterruptTypeMap: 0b011, MsiSupported: MSI_Off, MessageNumberLimit: 3}
	limit3On := Device{InterruptTypeMap: 0b011, DevicePolicy: IrqPolicyAllCloseProcessors, MsiSupported: MSI_On, MessageNumberLimit: 3}
	limit2049Off := Device{InterruptTypeMap: 0b111, MsiSupported: MSI_Off, MessageNumberLimit: 2049}
	// Processors 8 and 9, which this machine of 8 lacks, alone and beside
	// processor 0.
	lacking := Device{DevicePolicy: IrqPolicySpecifiedProcessors, AssignmentSetOverride: 0b11 << 8, MsiSupported: MSI_Off}
	mixed := Device{DevicePolicy: IrqPolicySpecifiedProcessors, AssignmentSetOverride: 0b11<<8 | 0b1, MsiSupported: MSI_Off}

	// ask is a request with every flag left out but those set by edit.
	ask := func(edit func(r *cliRequest)) cliRequest {
		r := cliRequest{policy: -1, priority: -1, msi: -1, limit: -1}
		if edit != nil {
			edit(&r)
		}
		return r
	}

	tests := []struct {
		name string
		dev  Device
		req  cliRequest
		// says is part of the reason given, or empty for a run that is not
		// refused.
		says string
	}{
		{name: "nothing asked", dev: onNone, req: ask(nil)},

		{name: "-policy 4 without -cpu", dev: onNone, req: ask(func(r *cliRequest) { r.policy = 4 }), says: "-policy 4 needs at least one processor"},
		{name: "-policy 4 without -cpu on a device with processors", dev: onTwo, req: ask(func(r *cliRequest) { r.policy = 4 })},
		{name: "-policy 4 -cpu", dev: onNone, req: ask(func(r *cliRequest) { r.policy, r.cpu = 4, 0b0011 })},
		{name: "-priority on a device left with none", dev: leftEmpty, req: ask(func(r *cliRequest) { r.priority = 2 }), says: "add -cpu, or a -policy other than 4"},
		{name: "-msisupported on a device left with none", dev: leftEmpty, req: ask(func(r *cliRequest) { r.msi = 0 })},

		{name: "-cpu without -policy", dev: onNone, req: ask(func(r *cliRequest) { r.cpu = 0b0011 }), says: "add -policy 4"},
		{name: "-cpu with another policy", dev: onTwo, req: ask(func(r *cliRequest) { r.policy, r.cpu = 3, 0b0011 }), says: "not -policy 3"},
		{name: "-cpu on a device on policy 4", dev: onTwo, req: ask(func(r *cliRequest) { r.cpu = 0b0011 })},

		// These start from a request main has already sorted into processors
		// and bad entries. The sorting itself, in main's loop, needs Windows.
		{name: "-cpu naming a processor this machine lacks", dev: onTwo, req: ask(func(r *cliRequest) { r.policy, r.cpu, r.badCPU = 4, 0b0010, []string{"8"} }), says: "-cpu 8: this machine's addressable processors are 0 to 7"},
		{name: "-cpu naming only processors this machine lacks", dev: onNone, req: ask(func(r *cliRequest) { r.badCPU = []string{"64"} }), says: "-cpu 64"},
		{name: "-cpu that is not a number", dev: onTwo, req: ask(func(r *cliRequest) { r.badCPU = []string{"a"} }), says: "-cpu a"},

		{name: "-msilimit with MSI off", dev: onNone, req: ask(func(r *cliRequest) { r.limit = 4 }), says: "add -msisupported 1"},
		{name: "-msilimit with -msisupported 0", dev: onTwo, req: ask(func(r *cliRequest) { r.msi, r.limit = 0, 4 }), says: "-msisupported 0 turns it off"},
		{name: "-msilimit with MSI on", dev: onTwo, req: ask(func(r *cliRequest) { r.limit = 4 })},
		{name: "-msilimit with -msisupported 1", dev: onNone, req: ask(func(r *cliRequest) { r.msi, r.limit = 1, 4 })},
		{name: "-msilimit on a device without MSI", dev: noMSI, req: ask(func(r *cliRequest) { r.limit = 4 }), says: "reports no interrupt types"},

		// 0 is no limit, which MSI off leaves as well.
		{name: "-msilimit 0 with MSI on", dev: onTwo, req: ask(func(r *cliRequest) { r.limit = 0 })},
		{name: "-msilimit 0 with MSI off", dev: onNone, req: ask(func(r *cliRequest) { r.limit = 0 })},
		{name: "-msilimit 0 with -msisupported 0", dev: onTwo, req: ask(func(r *cliRequest) { r.msi, r.limit = 0, 0 })},
		{name: "-msilimit 0 on a device without MSI", dev: noMSI, req: ask(func(r *cliRequest) { r.limit = 0 })},
		{name: "-msilimit 0 on a device whose MSISupported is 2", dev: oddMSI, req: ask(func(r *cliRequest) { r.limit = 0 }), says: "MSISupported is 2"},
		{name: "-msilimit 0 with -msisupported 1 on a device whose MSISupported is 2", dev: oddMSI, req: ask(func(r *cliRequest) { r.msi, r.limit = 1, 0 })},
		{name: "-msilimit 0 with -msisupported 0 on a device whose MSISupported is 2", dev: oddMSI, req: ask(func(r *cliRequest) { r.msi, r.limit = 0, 0 })},

		{name: "a negative -policy", dev: onTwo, req: ask(func(r *cliRequest) { r.policy = -3 }), says: "-policy -3"},
		{name: "a negative -priority", dev: onTwo, req: ask(func(r *cliRequest) { r.priority = -2 }), says: "-priority -2"},
		{name: "a negative -msilimit", dev: onTwo, req: ask(func(r *cliRequest) { r.limit = -5 }), says: "-msilimit -5"},
		{name: "a -msilimit no DWORD holds", dev: onTwo, req: ask(func(r *cliRequest) { r.limit = 1 << 32 }), says: "-msilimit 4294967296"},
		{name: "a -policy no DWORD holds", dev: onNone, req: ask(func(r *cliRequest) { r.policy = 1<<32 + 4 }), says: "-policy 4294967300"},
		{name: "the largest DWORD as an MSI-X limit", dev: msiX, req: ask(func(r *cliRequest) { r.limit = 1<<32 - 1 }), says: "-msilimit 4294967295: it takes a number from 0 to 2048"},
		{name: "-msisupported 2", dev: onTwo, req: ask(func(r *cliRequest) { r.msi = 2 }), says: "-msisupported 2"},

		// What Windows documents: policies 0 to 5, priorities 0 to 3, and an
		// MSI limit of 1, 2, 4, 8 or 16, or up to 2048 for MSI-X.
		{name: "-policy 5", dev: onNone, req: ask(func(r *cliRequest) { r.policy = 5 })},
		{name: "-policy 6", dev: onNone, req: ask(func(r *cliRequest) { r.policy = 6 }), says: "-policy 6: it takes a number from 0 to 5"},
		{name: "-priority 3", dev: onNone, req: ask(func(r *cliRequest) { r.priority = 3 })},
		{name: "-priority 4", dev: onNone, req: ask(func(r *cliRequest) { r.priority = 4 }), says: "-priority 4: it takes a number from 0 to 3"},
		{name: "-msilimit 3 without MSI-X", dev: onTwo, req: ask(func(r *cliRequest) { r.limit = 3 }), says: "-msilimit 3: a device without MSI-X takes 1, 2, 4, 8 or 16"},
		{name: "-msilimit 16 without MSI-X", dev: onTwo, req: ask(func(r *cliRequest) { r.limit = 16 })},
		{name: "-msilimit 3 with MSI-X", dev: msiX, req: ask(func(r *cliRequest) { r.limit = 3 })},
		{name: "-msilimit 2048 with MSI-X", dev: msiX, req: ask(func(r *cliRequest) { r.limit = 2048 })},
		{name: "-msilimit 2049 with MSI-X", dev: msiX, req: ask(func(r *cliRequest) { r.limit = 2049 }), says: "-msilimit 2049: it takes a number from 0 to 2048"},
		{name: "-msisupported 1 over an MSI-X limit of 2049", dev: limit2049Off, req: ask(func(r *cliRequest) { r.msi = 1 }), says: "the device's MSI limit is 2049, and an MSI-X device takes up to 2048"},
		{name: "-msisupported 1 over a limit of 3", dev: limit3Off, req: ask(func(r *cliRequest) { r.msi = 1 }), says: "the device's MSI limit is 3"},
		{name: "-msisupported 1 -msilimit 4 over a limit of 3", dev: limit3Off, req: ask(func(r *cliRequest) { r.msi, r.limit = 1, 4 })},
		{name: "-msisupported 0 over a limit of 3", dev: limit3Off, req: ask(func(r *cliRequest) { r.msi = 0 })},
		// MSI already on writes nothing, its limit included.
		{name: "-msisupported 1 with MSI on and a limit of 3", dev: limit3On, req: ask(func(r *cliRequest) { r.msi = 1 })},
		{name: "-msisupported 0 with MSI on and a limit of 3", dev: limit3On, req: ask(func(r *cliRequest) { r.msi = 0 })},
		{name: "-msilimit 3 with MSI on and a limit of 3", dev: limit3On, req: ask(func(r *cliRequest) { r.limit = 3 }), says: "-msilimit 3: a device without MSI-X takes 1, 2, 4, 8 or 16"},
		{name: "-policy on a device whose MSI limit Windows does not define", dev: limit3On, req: ask(func(r *cliRequest) { r.policy = 3 })},
		{name: "-priority on a device whose policy Windows does not define", dev: oddPolicy, req: ask(func(r *cliRequest) { r.priority = 2 }), says: "the device's DevicePolicy is 7"},
		{name: "-policy over a policy Windows does not define", dev: oddPolicy, req: ask(func(r *cliRequest) { r.policy = 1 })},
		{name: "-msisupported on a device whose policy Windows does not define", dev: oddPolicy, req: ask(func(r *cliRequest) { r.msi = 1 })},
		{name: "-policy on a device whose priority Windows does not define", dev: oddPriority, req: ask(func(r *cliRequest) { r.policy = 3 }), says: "the device's DevicePriority is 9"},
		{name: "-priority over a priority Windows does not define", dev: oddPriority, req: ask(func(r *cliRequest) { r.priority = 1 })},
		{name: "-msisupported on a device whose priority Windows does not define", dev: oddPriority, req: ask(func(r *cliRequest) { r.msi = 1 })},
		// The policy it has already: nothing is written.
		{name: "-policy 1 on a device on policy 1 whose priority Windows does not define", dev: oddPriority, req: ask(func(r *cliRequest) { r.policy = 1 })},
		{name: "-priority 0 on a device with priority 0 whose policy Windows does not define", dev: oddPolicy, req: ask(func(r *cliRequest) { r.priority = 0 })},

		{name: "-priority on a device with only processors this machine lacks", dev: lacking, req: ask(func(r *cliRequest) { r.priority = 2 }), says: "with no processor this machine has: add -cpu, or a -policy other than 4"},
		{name: "-cpu on a device with only processors this machine lacks", dev: lacking, req: ask(func(r *cliRequest) { r.cpu = 0b0001 })},
		{name: "-priority on a device with processors this machine lacks", dev: mixed, req: ask(func(r *cliRequest) { r.priority = 2 }), says: "the device's mask names processors this machine does not have: add -cpu, or a -policy other than 4"},
		{name: "-policy 4 -priority on a device with processors this machine lacks", dev: mixed, req: ask(func(r *cliRequest) { r.policy, r.priority = 4, 2 }), says: "-policy 4: the device's mask names processors this machine does not have: add -cpu"},
		{name: "-cpu on a device with processors this machine lacks", dev: mixed, req: ask(func(r *cliRequest) { r.cpu = 0b0001 })},
		{name: "-policy 4 on a device on policy 4 with processors this machine lacks", dev: mixed, req: ask(func(r *cliRequest) { r.policy = 4 })},
		{name: "-policy 1 on a device with processors this machine lacks", dev: mixed, req: ask(func(r *cliRequest) { r.policy = 1 })},
	}

	for _, tt := range tests {
		refused := cliRefusals(&tt.dev, tt.req, threads)

		if tt.says == "" {
			if len(refused) != 0 {
				t.Errorf("%s: refused, want it written: %q", tt.name, refused)
			}
			continue
		}

		if !strings.Contains(strings.Join(refused, "\n"), tt.says) {
			t.Errorf("%s: got %q, want it refused saying %q", tt.name, refused, tt.says)
		}
	}
}

// A device whose MSI settings this program does not read is not told to turn
// MSI on, which would write settings the dialog never shows for it.
func TestCLIRefusalForADeviceWithoutMSIDoesNotSuggestTurningItOn(t *testing.T) {
	noMSI := Device{MsiSupported: MSI_Invalid}
	refused := cliRefusals(&noMSI, cliRequest{policy: -1, priority: -1, msi: -1, limit: 4}, 8)

	if strings.Contains(strings.Join(refused, "\n"), "-msisupported 1") {
		t.Errorf("got %q, want no advice to turn MSI on", refused)
	}
}
