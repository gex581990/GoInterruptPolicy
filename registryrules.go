package main

import "fmt"

// What Microsoft documents for the values this program writes. OK, Export
// and the command line refuse anything else rather than write it. Export is
// held to the same rules as a write, because a .reg file is imported to apply
// its settings without this program.
//
// https://learn.microsoft.com/en-us/windows-hardware/drivers/kernel/interrupt-affinity-and-priority
// https://learn.microsoft.com/en-us/windows-hardware/drivers/kernel/enabling-message-signaled-interrupts-in-the-registry
// DevicePriority, which those pages no longer cover, is in the white paper
// "Interrupt Architecture Enhancements in Microsoft Windows Vista":
// https://download.microsoft.com/download/5/7/7/577a5684-8a83-43ae-9272-ff260a9c20e2/MSI.doc

// IrqPriorityHigh is the last IRQ_PRIORITY, after Undefined, Low and Normal.
const IrqPriorityHigh = 3

// msiXBit is MSI-X in a device's InterruptTypeMap.
const msiXBit = Bits(4)

// limitTaken says whether Microsoft documents limit as the MessageNumberLimit
// of a device with the interrupt types in types: 1, 2, 4, 8 or 16 for MSI, and
// any number up to 2048 for MSI-X. 0 is no limit, which is written by
// deleting the value.
func limitTaken(limit uint32, types Bits) bool {
	if limit == 0 {
		return true
	}
	if Has(types, msiXBit) {
		return limit <= 2048
	}

	switch limit {
	case 1, 2, 4, 8, 16:
		return true
	}

	return false
}

// absentProcessors is the part of a mask that names no processor this
// machine has: every bit from threads up, threads being how many processors
// a mask can name here, numbered from 0. It is empty when threads is not
// known.
func absentProcessors(threads int) Bits {
	if threads <= 0 {
		return ZeroBit
	}

	return ^(Bits(1)<<threads - 1)
}

// affinityRefusals gives the reasons Microsoft's documentation does not cover
// d's Affinity Policy key. The key is written whole, so a value nobody changed
// counts as much as one somebody did. absent is the part of a mask that names
// no processor this machine has, for which a KAFFINITY's bits "are always
// zero".
func affinityRefusals(d *Device, absent Bits) []string {
	var refused []string
	switch {
	case d.DevicePolicy > IrqPolicySpreadMessagesAcrossAllProcessors:
		refused = append(refused, fmt.Sprintf("Device Policy %d is not one Microsoft documents for a device. Choose a policy from the list.", d.DevicePolicy))
	case d.DevicePolicy == IrqPolicySpecifiedProcessors && d.AssignmentSetOverride&^absent == ZeroBit:
		// The white paper: "If IrqPolicySpecifiedProcessors is specified,
		// AssignmentSetOverride ... must be nonzero." A mask of processors
		// this machine does not have is no more use.
		refused = append(refused, "The affinity mask must contain at least one processor.")
	case d.DevicePolicy == IrqPolicySpecifiedProcessors && d.AssignmentSetOverride&absent != ZeroBit:
		// Set on a machine with more of them, or before SMT was turned off.
		// The dialog has no box for them, and only All Off clears them.
		refused = append(refused, "The affinity mask names processors this machine does not have. Press All Off, then tick the processors to use.")
	}
	if d.DevicePriority > IrqPriorityHigh {
		refused = append(refused, fmt.Sprintf("Device Priority %d is not one Microsoft documents. Choose a priority from the list.", d.DevicePriority))
	}

	return refused
}

// msiRefusals gives the reasons Microsoft's documentation does not cover d's
// MessageSignaledInterruptProperties key. Only MSI on writes a limit; MSI off
// deletes it.
func msiRefusals(d *Device) []string {
	if d.MsiSupported != MSI_On || limitTaken(d.MessageNumberLimit, d.InterruptTypeMap) {
		return nil
	}
	if Has(d.InterruptTypeMap, msiXBit) {
		return []string{fmt.Sprintf("An MSI limit of %d is more than Microsoft documents for MSI-X. Choose up to 2048, or 0 for no limit.", d.MessageNumberLimit)}
	}

	return []string{fmt.Sprintf("An MSI limit of %d is not one Microsoft documents for a device without MSI-X. Choose 1, 2, 4, 8 or 16, or 0 for no limit.", d.MessageNumberLimit)}
}
