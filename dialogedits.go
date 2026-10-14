package main

// dialogEdits is what the device policy dialog has changed, so that applying
// it changes nothing else.
//
// The dialog opens on the settings its devices have in common, and on a blank
// or a placeholder for any they do not: the Device Policy and Device Priority
// dropdowns hold 32, which is no policy and no priority, the processors none
// and MSI the third state of its checkbox. And it moves one setting itself
// while it is being built: the MSI limit box shows a device without a limit as
// 1, and copies that 1 into the device as it is filled in. Writing back
// everything the dialog holds wrote all of that over every device it was open
// for, so pressing OK without touching anything could change them.
//
// So the settings are noted once the dialog is built, and each device keeps
// its own value of every setting the user has not changed since.
type dialogEdits struct {
	opened Device // the dialog's settings once it was built

	// limit says whether the user changed the MSI limit, which is told apart
	// from its value: the box shows no limit as 1, so a 1 chosen in it would
	// look no different from the 1 it opened on.
	limit bool
}

// apply returns d with the settings the dialog has changed set to what now
// holds, and every other setting as d has it.
func (e dialogEdits) apply(d, now Device) Device {
	if now.DevicePolicy != e.opened.DevicePolicy {
		d.DevicePolicy = now.DevicePolicy
	}

	// The processors are only for the specified processors policy, and stay
	// as they are under any other. Ticked and then left behind by switching
	// back to another policy, they would be written for nothing and ask for a
	// restart.
	if now.AssignmentSetOverride != e.opened.AssignmentSetOverride && d.DevicePolicy == IrqPolicySpecifiedProcessors {
		d.AssignmentSetOverride = now.AssignmentSetOverride
	}

	if now.DevicePriority != e.opened.DevicePriority {
		d.DevicePriority = now.DevicePriority
	}

	// The dialog does not show its MSI settings for a device that reports no
	// interrupt types, so they are not its to change.
	if d.MsiSupported == MSI_Invalid {
		return d
	}

	if now.MsiSupported != e.opened.MsiSupported {
		d.MsiSupported = now.MsiSupported
	}

	// A limit only goes with MSI on, so it is applied only when the box ends
	// ticked, the one state in which the limit box can be used. Typed while MSI
	// was ticked for a moment and then left behind by unticking the box or
	// clicking it back to grey, it is not a limit the user chose for these
	// devices.
	if e.limit && now.MsiSupported == MSI_On {
		d.MessageNumberLimit = now.MessageNumberLimit
	}

	return d
}

// applyAll is apply for each of devices, in order.
func (e dialogEdits) applyAll(devices []Device, now Device) []Device {
	out := make([]Device, len(devices))
	for i := range devices {
		out[i] = e.apply(devices[i], now)
	}

	return out
}

// leavesNoProcessor says whether applying the dialog would leave any of
// devices on the specified processors policy with no processor to use.
func (e dialogEdits) leavesNoProcessor(devices []Device, now Device) bool {
	for i := range devices {
		if d := e.apply(devices[i], now); d.DevicePolicy == IrqPolicySpecifiedProcessors && d.AssignmentSetOverride == ZeroBit {
			return true
		}
	}

	return false
}
