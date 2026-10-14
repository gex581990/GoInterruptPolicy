package main

// dialogEdits is what the device policy dialog has changed, so that applying
// it changes nothing else.
//
// The dialog opens on the settings its devices have in common, and on a blank
// or a placeholder for any they do not: the Device Policy and Device Priority
// dropdowns hold 32, which is no policy and no priority, the processors none,
// MSI the grey third state of its checkbox and the MSI limit 0. Writing back
// everything the dialog holds wrote all of that over every device it was open
// for, so pressing OK without touching anything could change them.
//
// So the settings are noted once the dialog is built, and each device keeps
// its own value of every setting the user has not changed since.
type dialogEdits struct {
	opened Device // the dialog's settings once it was built

	// limit says whether the user changed the MSI limit. The value alone
	// cannot say. For devices whose limits differ the box opens on 0, which is
	// also no limit, so a 0 reached in it from another value looks the same
	// as the 0 it opened on. A 0 typed straight over that 0 is not seen at
	// all, since the box reports only a change of value, and its tooltip says
	// so (see limitHidden). And a limit above the most the box allows cannot
	// be shown, so the box stays at 0, and ticking MSI on copies that 0 into
	// the dialog's settings although nobody chose it.
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

// leavesNoProcessor says whether applying the dialog would write any of
// devices onto the specified processors policy with no processor to use.
//
// Only a device whose Affinity Policy key the dialog changes counts, which a
// change of its policy, processors or priority does: the key is written whole.
// One already on that policy with no processor, which the command line used
// to leave devices in, is not refused for being left alone, or for a change
// to its MSI settings only. OK writes nothing to its key, and a backup of it
// holds only what is there already.
func (e dialogEdits) leavesNoProcessor(devices []Device, now Device) bool {
	for i := range devices {
		d := e.apply(devices[i], now)
		changed := d.DevicePolicy != devices[i].DevicePolicy || d.AssignmentSetOverride != devices[i].AssignmentSetOverride || d.DevicePriority != devices[i].DevicePriority
		if changed && d.DevicePolicy == IrqPolicySpecifiedProcessors && d.AssignmentSetOverride == ZeroBit {
			return true
		}
	}

	return false
}
