package main

import "testing"

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
// in common, the placeholders RunDialog gives what they do not, and the MSI
// limit box's 1 for no limit.
func opened(devices ...Device) Device {
	d := Device{
		DevicePolicy:          FindCommonValue(devices, 32, func(d Device) uint32 { return d.DevicePolicy }),
		DevicePriority:        FindCommonValue(devices, 32, func(d Device) uint32 { return d.DevicePriority }),
		AssignmentSetOverride: FindCommonValue(devices, 0, func(d Device) Bits { return d.AssignmentSetOverride }),
		MsiSupported:          FindCommonValue(devices, MSI_Tristate, func(d Device) uint32 { return d.MsiSupported }),
		MessageNumberLimit:    FindCommonValue(devices, 0, func(d Device) uint32 { return d.MessageNumberLimit }),
	}
	if d.MessageNumberLimit < 1 {
		d.MessageNumberLimit = 1
	}

	return d
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
			// Each keeps the limit it had, none or 8, rather than the 1 the
			// box showed.
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

// The box shows no limit as 1, so a limit of 1 the user picked cannot be told
// from it by value. It is the user having changed the box that counts.
func TestDialogEditsApplyALimitOfOneTheUserChose(t *testing.T) {
	now := opened(gpu)

	if got := (dialogEdits{opened: now}).apply(gpu, now).MessageNumberLimit; got != 0 {
		t.Errorf("an untouched box gave the limit %d, want none", got)
	}
	if got := (dialogEdits{opened: now, limit: true}).apply(gpu, now).MessageNumberLimit; got != 1 {
		t.Errorf("a 1 chosen in the box gave the limit %d, want 1", got)
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

func TestDialogEditsLeavesNoProcessor(t *testing.T) {
	cores := func(d Device, mask Bits) Device { d.AssignmentSetOverride = mask; return d }

	tests := []struct {
		name    string
		devices []Device
		edit    func(d *Device)
		want    bool
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
			want:    true,
		},
		{
			name:    "specified processors chosen and none ticked",
			devices: []Device{usb},
			edit:    func(d *Device) { d.DevicePolicy = 4 },
			want:    true,
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
			want:    true,
		},
		{
			name:    "specified processors chosen with processors ticked",
			devices: []Device{gpu, usb},
			edit:    func(d *Device) { d.DevicePolicy, d.AssignmentSetOverride = 4, 0b0001 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := opened(tt.devices...)
			edits := dialogEdits{opened: now}
			tt.edit(&now)

			if got := edits.leavesNoProcessor(tt.devices, now); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
