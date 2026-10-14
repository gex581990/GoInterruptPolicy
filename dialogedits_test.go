package main

import (
	"strconv"
	"strings"
	"testing"
)

// settings is the part of a Device the dialog edits, which unlike a Device
// can be compared.
type settings struct {
	policy, priority uint32
	mask             Bits
	msi, limit       uint32
}

func settingsOf(d Device) settings {
	return settings{d.DevicePolicy, d.DevicePriority, d.AssignmentSetOverride, d.MsiSupported, d.MessageNumberLimit}
}

// opened is what the dialog holds for devices once it is built: what they have
// in common, and the placeholders RunDialog gives what they do not.
func opened(devices ...Device) Device {
	return Device{
		DevicePolicy:          FindCommonValue(devices, 32, func(d Device) uint32 { return d.DevicePolicy }),
		DevicePriority:        FindCommonValue(devices, 32, func(d Device) uint32 { return d.DevicePriority }),
		AssignmentSetOverride: FindCommonValue(devices, 0, func(d Device) Bits { return d.AssignmentSetOverride }),
		MsiSupported:          FindCommonValue(devices, MSI_Tristate, func(d Device) uint32 { return d.MsiSupported }),
		MessageNumberLimit:    FindCommonValue(devices, 0, func(d Device) uint32 { return d.MessageNumberLimit }),
	}
}

var (
	// MSI on with no limit, the device pressing OK used to cap at one message.
	gpu = Device{DevicePolicy: IrqPolicySpecifiedProcessors, AssignmentSetOverride: 0b0110, DevicePriority: 3, MsiSupported: MSI_On}
	// MSI on with a limit, and no policy of its own.
	nic = Device{DevicePolicy: IrqPolicyMachineDefault, DevicePriority: 0, MsiSupported: MSI_On, MessageNumberLimit: 8}
	// MSI off, and another policy again.
	usb = Device{DevicePolicy: IrqPolicyAllCloseProcessors, DevicePriority: 2, MsiSupported: MSI_Off}
	// Reports no interrupt types, so the dialog hides its MSI settings.
	legacy = Device{DevicePolicy: IrqPolicyMachineDefault, MsiSupported: MSI_Invalid}
)

func TestDialogEditsLeaveAnUntouchedDeviceAlone(t *testing.T) {
	for _, d := range []Device{gpu, nic, usb, legacy} {
		now := opened(d)
		if got, want := settingsOf((dialogEdits{opened: now}).apply(d, now)), settingsOf(d); got != want {
			t.Errorf("OK without a change made %+v into %+v", want, got)
		}
	}
}

func TestDialogEditsLeaveUntouchedDevicesAlone(t *testing.T) {
	devices := []Device{gpu, nic, usb, legacy}
	now := opened(devices...)

	got := (dialogEdits{opened: now}).applyAll(devices, now)
	for i := range devices {
		if settingsOf(got[i]) != settingsOf(devices[i]) {
			t.Errorf("device %d: OK without a change made %+v into %+v", i, settingsOf(devices[i]), settingsOf(got[i]))
		}
	}
}

func TestDialogEditsApplyOnlyWhatChanged(t *testing.T) {
	devices := []Device{gpu, nic, usb}

	tests := []struct {
		name  string
		edit  func(d *Device)
		limit bool
		want  []settings
	}{
		{
			name: "priority",
			edit: func(d *Device) { d.DevicePriority = 1 },
			want: []settings{
				{policy: 4, priority: 1, mask: 0b0110, msi: MSI_On},
				{policy: 0, priority: 1, msi: MSI_On, limit: 8},
				{policy: 1, priority: 1, msi: MSI_Off},
			},
		},
		{
			name: "specified processors",
			edit: func(d *Device) { d.DevicePolicy, d.AssignmentSetOverride = 4, 0b1000 },
			want: []settings{
				{policy: 4, priority: 3, mask: 0b1000, msi: MSI_On},
				{policy: 4, priority: 0, mask: 0b1000, msi: MSI_On, limit: 8},
				{policy: 4, priority: 2, mask: 0b1000, msi: MSI_Off},
			},
		},
		{
			// The processors stay with the device: writing a policy other
			// than 4 removes them from the registry anyway.
			name: "another policy",
			edit: func(d *Device) { d.DevicePolicy = 5 },
			want: []settings{
				{policy: 5, priority: 3, mask: 0b0110, msi: MSI_On},
				{policy: 5, priority: 0, msi: MSI_On, limit: 8},
				{policy: 5, priority: 2, msi: MSI_Off},
			},
		},
		{
			// Each keeps the limit it had, none or 8, rather than the 0 the
			// box opened on because their limits differ.
			name: "MSI on",
			edit: func(d *Device) { d.MsiSupported = MSI_On },
			want: []settings{
				{policy: 4, priority: 3, mask: 0b0110, msi: MSI_On},
				{policy: 0, priority: 0, msi: MSI_On, limit: 8},
				{policy: 1, priority: 2, msi: MSI_On},
			},
		},
		{
			name:  "MSI on with a limit",
			edit:  func(d *Device) { d.MsiSupported, d.MessageNumberLimit = MSI_On, 4 },
			limit: true,
			want: []settings{
				{policy: 4, priority: 3, mask: 0b0110, msi: MSI_On, limit: 4},
				{policy: 0, priority: 0, msi: MSI_On, limit: 4},
				{policy: 1, priority: 2, msi: MSI_On, limit: 4},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := opened(devices...)
			edits := dialogEdits{opened: now, limit: tt.limit}
			tt.edit(&now)

			got := edits.applyAll(devices, now)
			for i := range devices {
				if s := settingsOf(got[i]); s != tt.want[i] {
					t.Errorf("device %d: got %+v, want %+v", i, s, tt.want[i])
				}
			}
		})
	}
}

// A device without a limit gives the dialog a limit of 0, which RunDialog's
// binding shows in the box. A limit the box reports as changed, such as a 1
// typed over that 0, is applied, and one left alone is not. The binding, and
// the box reporting the 1, are walk's NumberEdit, which only Windows can run.
func TestDialogEditsApplyALimitOfOneTheUserChose(t *testing.T) {
	now := opened(gpu)
	if now.MessageNumberLimit != 0 {
		t.Fatalf("opened() gave %d for a device without a limit, want 0", now.MessageNumberLimit)
	}

	if got := (dialogEdits{opened: now}).apply(gpu, now).MessageNumberLimit; got != 0 {
		t.Errorf("an untouched box gave the limit %d, want none", got)
	}

	edits := dialogEdits{opened: now, limit: true}
	now.MessageNumberLimit = 1
	if got := edits.apply(gpu, now).MessageNumberLimit; got != 1 {
		t.Errorf("a 1 chosen in the box gave the limit %d, want 1", got)
	}
}

// For devices whose limits differ the box opens on 0. Left alone, each keeps
// its own limit. Changed back to 0 by the user, they all get no limit, which
// a comparison of values alone could not tell from the box being left alone.
func TestDialogEditsApplyNoLimitChosenForDevicesWhoseLimitsDiffer(t *testing.T) {
	devices := []Device{gpu, nic}
	now := opened(devices...)

	for i, d := range (dialogEdits{opened: now}).applyAll(devices, now) {
		if d.MessageNumberLimit != devices[i].MessageNumberLimit {
			t.Errorf("device %d: an untouched box gave the limit %d, want its own %d", i, d.MessageNumberLimit, devices[i].MessageNumberLimit)
		}
	}

	for i, d := range (dialogEdits{opened: now, limit: true}).applyAll(devices, now) {
		if d.MessageNumberLimit != 0 {
			t.Errorf("device %d: a 0 chosen in the box gave the limit %d, want none", i, d.MessageNumberLimit)
		}
	}
}

// RunDialog opens the MSI box grey for devices whose MSI settings differ,
// and its click handler reads a box clicked round to grey again as
// MSI_Tristate. Both are walk's, which only Windows can run. Given them, each
// device keeps its own MSI settings, whatever was typed into the limit box
// while it was ticked.
func TestDialogEditsLeaveMSIAloneWhenTheBoxEndsGrey(t *testing.T) {
	devices := []Device{gpu, usb}
	now := opened(devices...)
	if now.MsiSupported != MSI_Tristate {
		t.Fatalf("opened() gave %d for devices whose MSI differs, want MSI_Tristate", now.MsiSupported)
	}

	edits := dialogEdits{opened: now, limit: true}
	now.MessageNumberLimit = 4

	for i, d := range edits.applyAll(devices, now) {
		if got, want := settingsOf(d), settingsOf(devices[i]); got != want {
			t.Errorf("device %d: got %+v, want it as it was, %+v", i, got, want)
		}
	}
}

// A limit typed while MSI was ticked for a moment, on a device whose MSI is
// off, is not applied once MSI is unticked again. Applied, it made OK delete
// the device's MSI key and ask for a restart although MSI never changed.
func TestDialogEditsApplyALimitOnlyWithMSIOn(t *testing.T) {
	now := opened(usb)
	edits := dialogEdits{opened: now, limit: true}
	now.MessageNumberLimit = 4

	if got, want := settingsOf(edits.apply(usb, now)), settingsOf(usb); got != want {
		t.Errorf("unticked again: got %+v, want the device as it was, %+v", got, want)
	}

	now.MsiSupported = MSI_On
	want := settings{policy: usb.DevicePolicy, priority: usb.DevicePriority, msi: MSI_On, limit: 4}
	if got := settingsOf(edits.apply(usb, now)); got != want {
		t.Errorf("left ticked: got %+v, want %+v", got, want)
	}
}

// The limit box cannot show the devices' limit when it differs between them,
// or when theirs is above the most the box goes up to.
func TestLimitHidden(t *testing.T) {
	tests := []struct {
		name    string
		devices []Device
		most    float64
		want    bool
	}{
		{name: "one device without a limit", devices: []Device{gpu}, most: 16},
		{name: "one device with a limit", devices: []Device{nic}, most: 16},
		{name: "a limit above the most", devices: []Device{{MessageNumberLimit: 32}}, most: 16, want: true},
		{name: "limits that differ", devices: []Device{gpu, nic}, most: 2048, want: true},
		{name: "the same limit", devices: []Device{nic, nic}, most: 16},
	}

	for _, tt := range tests {
		shown := opened(tt.devices...).MessageNumberLimit
		if got := limitHidden(tt.devices, shown, tt.most); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

// Switching to specified processors shows the processors to tick, and
// switching back leaves the ticks behind. They are not the device's to keep.
func TestDialogEditsKeepProcessorsOnlyForTheirPolicy(t *testing.T) {
	now := opened(usb)
	edits := dialogEdits{opened: now}
	now.DevicePolicy, now.AssignmentSetOverride = IrqPolicySpecifiedProcessors, 0b1000
	now.DevicePolicy = usb.DevicePolicy

	if got, want := settingsOf(edits.apply(usb, now)), settingsOf(usb); got != want {
		t.Errorf("got %+v, want the device as it was, %+v", got, want)
	}
}

func TestDialogEditsLeaveMSIAloneOnADeviceWithout(t *testing.T) {
	devices := []Device{nic, legacy}
	now := opened(devices...)
	edits := dialogEdits{opened: now, limit: true}
	now.MsiSupported, now.MessageNumberLimit = MSI_On, 2

	got := edits.apply(legacy, now)
	if got.MsiSupported != MSI_Invalid || got.MessageNumberLimit != 0 {
		t.Errorf("got MSI %d with limit %d, want it left without MSI settings", got.MsiSupported, got.MessageNumberLimit)
	}
}

func TestDialogEditsRefusals(t *testing.T) {
	cores := func(d Device, mask Bits) Device { d.AssignmentSetOverride = mask; return d }
	// Already on specified processors with none, as the command line used to
	// leave a device it was given -policy 4 alone for.
	leftEmpty := Device{DevicePolicy: IrqPolicySpecifiedProcessors, MsiSupported: MSI_Off}
	// Values Windows does not document, as another tool could have stored.
	oddPolicy := Device{DevicePolicy: 7, MsiSupported: MSI_Off}
	oddPriority := Device{DevicePolicy: IrqPolicyAllCloseProcessors, DevicePriority: 9, MsiSupported: MSI_Off}
	msi := Device{DeviceDesc: "MSI device", InterruptTypeMap: 0b011, MsiSupported: MSI_On, MessageNumberLimit: 8}
	msiX := Device{DeviceDesc: "MSI-X device", InterruptTypeMap: 0b111, MsiSupported: MSI_On, MessageNumberLimit: 64}
	limit3Off := Device{DeviceDesc: "MSI device", InterruptTypeMap: 0b011, MsiSupported: MSI_Off, MessageNumberLimit: 3}
	limit3On := Device{DeviceDesc: "MSI device", InterruptTypeMap: 0b011, MsiSupported: MSI_On, MessageNumberLimit: 3}
	limit3000 := Device{DeviceDesc: "MSI-X device", InterruptTypeMap: 0b111, MsiSupported: MSI_On, MessageNumberLimit: 3000}
	// Processors 8 and 9 on a machine of 8, numbered 0 to 7, as a mask set
	// before SMT was turned off can be.
	lacking := Device{DevicePolicy: IrqPolicySpecifiedProcessors, AssignmentSetOverride: 0b11 << 8, MsiSupported: MSI_Off}
	mixed := cores(lacking, 0b11<<8|0b1)
	eight := absentProcessors(8)
	// Two devices of one kind, with one name, told apart by their location.
	audio := Device{DeviceDesc: "High Definition Audio Controller", LocationInformation: "PCI bus 1, device 0, function 1", InterruptTypeMap: 0b011, MsiSupported: MSI_On, MessageNumberLimit: 3}
	audio2 := Device{DeviceDesc: "High Definition Audio Controller", LocationInformation: "PCI bus 0, device 31, function 3", InterruptTypeMap: 0b011, MsiSupported: MSI_On, MessageNumberLimit: 4}

	const lacks = "names processors this machine does not have"

	const none = "at least one processor"

	tests := []struct {
		name    string
		devices []Device
		edit    func(d *Device)
		limit   bool // whether the user changed the MSI limit
		absent  Bits // the processors the machine lacks
		// ok and export are part of the reason OK and Export give, or empty
		// for neither refused.
		ok, export string
	}{
		{
			name:    "untouched",
			devices: []Device{gpu},
			edit:    func(d *Device) {},
		},
		{
			name:    "every processor cleared",
			devices: []Device{gpu},
			edit:    func(d *Device) { d.AssignmentSetOverride = 0 },
			ok:      none, export: none,
		},
		{
			name:    "specified processors chosen and none ticked",
			devices: []Device{usb},
			edit:    func(d *Device) { d.DevicePolicy = 4 },
			ok:      none, export: none,
		},
		{
			// The devices share the policy but not the processors, so the
			// dialog shows none ticked, and each keeps its own.
			name:    "shared policy, own processors untouched",
			devices: []Device{gpu, cores(gpu, 0b0001)},
			edit:    func(d *Device) { d.DevicePriority = 1 },
		},
		{
			name:    "specified processors chosen for one that has none",
			devices: []Device{gpu, usb},
			edit:    func(d *Device) { d.DevicePolicy = 4 },
			ok:      none, export: none,
		},
		{
			name:    "specified processors chosen with processors ticked",
			devices: []Device{gpu, usb},
			edit:    func(d *Device) { d.DevicePolicy, d.AssignmentSetOverride = 4, 0b0001 },
		},
		{
			// OK writes nothing, but the file would put the device back on
			// specified processors with none.
			name:    "left with none, untouched",
			devices: []Device{leftEmpty},
			edit:    func(d *Device) {},
			export:  none,
		},
		{
			name:    "left with none, beside another device, untouched",
			devices: []Device{leftEmpty, usb},
			edit:    func(d *Device) {},
			export:  none,
		},
		{
			// OK writes MSI on its own, and leaves the Affinity Policy key as
			// it is. The file writes that key too.
			name:    "left with none, MSI changed",
			devices: []Device{leftEmpty},
			edit:    func(d *Device) { d.MsiSupported = MSI_On },
			export:  none,
		},
		{
			// A new priority rewrites the whole key, policy 4 and no
			// processor included.
			name:    "left with none, priority changed",
			devices: []Device{leftEmpty},
			edit:    func(d *Device) { d.DevicePriority = 3 },
			ok:      none, export: none,
		},
		{
			name:    "a policy Windows does not define, untouched",
			devices: []Device{oddPolicy},
			edit:    func(d *Device) {},
			export:  "Device Policy 7",
		},
		{
			name:    "a policy Windows does not define, priority changed",
			devices: []Device{oddPolicy},
			edit:    func(d *Device) { d.DevicePriority = 2 },
			ok:      "Device Policy 7", export: "Device Policy 7",
		},
		{
			name:    "a policy Windows does not define, replaced",
			devices: []Device{oddPolicy},
			edit:    func(d *Device) { d.DevicePolicy = 1 },
		},
		{
			name:    "a priority Windows does not define, policy changed",
			devices: []Device{oddPriority},
			edit:    func(d *Device) { d.DevicePolicy = 3 },
			ok:      "Device Priority 9", export: "Device Priority 9",
		},
		{
			name:    "a priority Windows does not define, MSI changed",
			devices: []Device{oddPriority},
			edit:    func(d *Device) { d.MsiSupported = MSI_On },
			export:  "Device Priority 9",
		},
		{
			name:    "an MSI limit of 3 without MSI-X",
			devices: []Device{msi},
			edit:    func(d *Device) { d.MessageNumberLimit = 3 },
			limit:   true,
			ok:      "MSI limit of 3", export: "MSI limit of 3",
		},
		{
			name:    "an MSI limit of 16 without MSI-X",
			devices: []Device{msi},
			edit:    func(d *Device) { d.MessageNumberLimit = 16 },
			limit:   true,
		},
		{
			name:    "an MSI limit of 12 with MSI-X",
			devices: []Device{msiX},
			edit:    func(d *Device) { d.MessageNumberLimit = 12 },
			limit:   true,
		},
		{
			name:    "no MSI limit",
			devices: []Device{msi},
			edit:    func(d *Device) { d.MessageNumberLimit = 0 },
			limit:   true,
		},
		{
			name:    "an MSI-X limit above 2048 already set, untouched",
			devices: []Device{limit3000},
			edit:    func(d *Device) {},
			export:  "MSI limit of 3000",
		},
		{
			// MSI on writes the limit the device had with MSI off.
			name:    "MSI turned on over a limit of 3",
			devices: []Device{limit3Off},
			edit:    func(d *Device) { d.MsiSupported = MSI_On },
			ok:      "MSI limit of 3", export: "MSI limit of 3",
		},
		{
			// MSI off deletes the limit, in the registry and in the file.
			name:    "MSI turned off over a limit of 3",
			devices: []Device{limit3On},
			edit:    func(d *Device) { d.MsiSupported = MSI_Off },
		},
		{
			name:    "only processors the machine lacks, untouched",
			devices: []Device{lacking},
			edit:    func(d *Device) {},
			absent:  eight,
			export:  none,
		},
		{
			name:    "only processors the machine lacks, priority changed",
			devices: []Device{lacking},
			edit:    func(d *Device) { d.DevicePriority = 2 },
			absent:  eight,
			ok:      none, export: none,
		},
		{
			name:    "a processor the machine has beside ones it lacks, untouched",
			devices: []Device{mixed},
			edit:    func(d *Device) {},
			absent:  eight,
			export:  lacks,
		},
		{
			name:    "a processor the machine has beside ones it lacks, priority changed",
			devices: []Device{mixed},
			edit:    func(d *Device) { d.DevicePriority = 2 },
			absent:  eight,
			ok:      lacks, export: lacks,
		},
		{
			// Ticking keeps the bits there is no box for.
			name:    "a processor ticked beside ones the machine lacks",
			devices: []Device{mixed},
			edit:    func(d *Device) { d.AssignmentSetOverride |= 0b10 },
			absent:  eight,
			ok:      lacks, export: lacks,
		},
		{
			// All Off clears them, and the processors ticked after are all
			// the mask holds.
			name:    "All Off, then a processor ticked",
			devices: []Device{mixed},
			edit:    func(d *Device) { d.AssignmentSetOverride, d.DevicePriority = 0b1, 2 },
			absent:  eight,
		},
		{
			name:    "an MSI limit of 3 on one of two devices with one name",
			devices: []Device{audio, audio2},
			edit:    func(d *Device) {},
			export:  "High Definition Audio Controller (PCI bus 1, device 0, function 1): An MSI limit of 3",
		},
		{
			// With the machine's processors unknown, every bit counts.
			name:    "processors 8 and 9 on a machine not known",
			devices: []Device{lacking},
			edit:    func(d *Device) { d.DevicePriority = 2 },
		},
		{
			name:    "an MSI limit of 3 named among several devices",
			devices: []Device{limit3On, msiX},
			edit:    func(d *Device) {},
			export:  "MSI device: An MSI limit of 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := opened(tt.devices...)
			edits := dialogEdits{opened: now, limit: tt.limit, absent: tt.absent}
			tt.edit(&now)

			for _, check := range []struct {
				name  string
				whole bool
				want  string
			}{{"OK", false, tt.ok}, {"Export", true, tt.export}} {
				refused := strings.Join(edits.refusals(tt.devices, now, check.whole), "\n")
				switch {
				case check.want == "" && refused != "":
					t.Errorf("%s refused, want it allowed: %q", check.name, refused)
				case check.want != "" && !strings.Contains(refused, check.want):
					t.Errorf("%s gave %q, want it refused saying %q", check.name, refused, check.want)
				}
			}
		})
	}
}

func TestAbsentProcessors(t *testing.T) {
	for _, tt := range []struct {
		threads int
		want    Bits
	}{
		{threads: 0, want: 0},
		{threads: 1, want: ^Bits(0b1)},
		{threads: 8, want: ^Bits(0xff)},
		{threads: 63, want: 1 << 63},
		{threads: 64, want: 0},
	} {
		if got := absentProcessors(tt.threads); got != tt.want {
			t.Errorf("absentProcessors(%d) = %#x, want %#x", tt.threads, got, tt.want)
		}
	}
}

// Microsoft documents a limit of 1, 2, 4, 8 or 16 for MSI, and any number up
// to 2048 for MSI-X. 0 is no limit.
func TestLimitTaken(t *testing.T) {
	const msi, msiX = Bits(0b011), Bits(0b111)
	for limit := uint32(0); limit <= 2049; limit++ {
		wantMSI := limit == 0 || limit == 1 || limit == 2 || limit == 4 || limit == 8 || limit == 16
		if got := limitTaken(limit, msi); got != wantMSI {
			t.Errorf("limit %d without MSI-X: got %v, want %v", limit, got, wantMSI)
		}
		if got, want := limitTaken(limit, msiX), limit <= 2048; got != want {
			t.Errorf("limit %d with MSI-X: got %v, want %v", limit, got, want)
		}
	}
}

// The limit box shows its value with no decimals, which strconv rounds half
// to even, and the limit is the number it shows.
func TestBoxLimitIsTheNumberTheBoxShows(t *testing.T) {
	for _, v := range []float64{0, 0.25, 0.5, 0.75, 1.5, 2.5, 7.5, 7.9, 8.5, 15.6, 2047.5, 2048} {
		if got, want := strconv.FormatUint(uint64(boxLimit(v)), 10), strconv.FormatFloat(v, 'f', 0, 64); got != want {
			t.Errorf("boxLimit(%v) = %s, the box shows %s", v, got, want)
		}
	}
}
