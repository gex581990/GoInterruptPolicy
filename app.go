//go:generate goversioninfo -64

package main

import (
	"fmt"
	"log"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	//lint:ignore ST1001 standard behavior tailscale/walk
	. "github.com/tailscale/walk/declarative"
)

var cs CpuSets

func main() {
	cs.Init()

	var devices []Device
	devices, handle = FindAllDevices()

	if CLIMode {
		var newItem *Device
		for i := 0; i < len(devices); i++ {
			if devices[i].DevObjName == flagDevObjName {
				newItem = &devices[i]
				break
			}
		}
		if newItem == nil {
			SetupDiDestroyDeviceInfoList(handle)
			os.Exit(1)
		}
		orgItem := *newItem

		var assignmentSetOverride Bits
		// The -cpu entries that name none of this machine's addressable
		// processors. Any of them refuses the run: the rest alone are not the
		// processors asked for.
		var badCPU []string
		if flagCPU != "" {
			for val := range strings.SplitSeq(flagCPU, ",") {
				// An empty entry, as a trailing comma leaves, names nothing.
				if val = strings.TrimSpace(val); val == "" {
					continue
				}
				i, err := strconv.Atoi(val)
				if err != nil {
					log.Println(err)
					badCPU = append(badCPU, val)
					continue
				}
				// Against the machine, not against the mask. CPUBits is always
				// 64 wide because that is how many bits the mask has, so
				// checking it only catches a processor number no mask could
				// hold and lets through one this machine does not have.
				if i < 0 || i >= cs.Threads {
					badCPU = append(badCPU, val)
					continue
				}
				assignmentSetOverride = Set(assignmentSetOverride, CPUBits[i])
			}
		}

		// Refused before anything is written, so a refused run changes nothing.
		request := cliRequest{
			policy:   flagDevicePolicy,
			priority: flagDevicePriority,
			msi:      flagMsiSupported,
			limit:    flagMessageNumberLimit,
			cpu:      assignmentSetOverride,
			badCPU:   badCPU,
		}
		if refused := cliRefusals(newItem, request, cs.Threads); len(refused) != 0 {
			for _, reason := range refused {
				log.Println(reason)
			}
			SetupDiDestroyDeviceInfoList(handle)
			os.Exit(1)
		}

		// Whether anything was written, as the setters report it, the way
		// SetNewDevice does for the GUI. Comparing orgItem with newItem
		// afterwards cannot tell: each setter copies what it wrote into
		// orgItem, so by then the two are the same.
		var changed bool

		if flagMsiSupported != -1 || flagMessageNumberLimit != -1 {
			if flagMsiSupported != -1 {
				newItem.MsiSupported = uint32(flagMsiSupported)
			}
			if flagMessageNumberLimit != -1 {
				newItem.MessageNumberLimit = uint32(flagMessageNumberLimit)
			}
			if orgItem.MsiSupported != newItem.MsiSupported || orgItem.MessageNumberLimit != newItem.MessageNumberLimit {
				changed = orgItem.setMSIMode(newItem)
			}
		}

		if flagDevicePolicy != -1 || flagDevicePriority != -1 || assignmentSetOverride != ZeroBit {
			if flagDevicePolicy != -1 {
				newItem.DevicePolicy = uint32(flagDevicePolicy)
			}
			if flagDevicePriority != -1 {
				newItem.DevicePriority = uint32(flagDevicePriority)
			}
			if assignmentSetOverride != ZeroBit {
				newItem.AssignmentSetOverride = assignmentSetOverride
			}
			if orgItem.DevicePolicy != newItem.DevicePolicy || orgItem.DevicePriority != newItem.DevicePriority || orgItem.AssignmentSetOverride != newItem.AssignmentSetOverride {
				changed = orgItem.setAffinityPolicy(newItem) || changed
			}
		}

		if flagRestart || (flagRestartOnChange && changed) {
			propChangeParams := PropChangeParams{
				ClassInstallHeader: *MakeClassInstallHeader(DIF_PROPERTYCHANGE),
				StateChange:        DICS_PROPCHANGE,
				Scope:              DICS_FLAG_GLOBAL,
			}

			if err := SetupDiSetClassInstallParams(handle, &newItem.Idata, &propChangeParams.ClassInstallHeader, uint32(unsafe.Sizeof(propChangeParams))); err != nil {
				log.Println(err)
				return
			}

			if err := SetupDiCallClassInstaller(DIF_PROPERTYCHANGE, handle, &newItem.Idata); err != nil {
				log.Println(err)
				return
			}

			if err := SetupDiSetClassInstallParams(handle, &newItem.Idata, &propChangeParams.ClassInstallHeader, uint32(unsafe.Sizeof(propChangeParams))); err != nil {
				log.Println(err)
				return
			}

			if err := SetupDiCallClassInstaller(DIF_PROPERTYCHANGE, handle, &newItem.Idata); err != nil {
				log.Println(err)
				return
			}

			DeviceInstallParams, err := SetupDiGetDeviceInstallParams(handle, &newItem.Idata)
			if err != nil {
				log.Println(err)
				return
			}

			if DeviceInstallParams.Flags&(DI_NEEDREBOOT|DI_NEEDRESTART) != 0 {
				fmt.Println("Device could not be restarted. Changes will take effect the next time you reboot.")
			} else {
				fmt.Println("Device successfully restarted.")
			}
		}
		SetupDiDestroyDeviceInfoList(handle)
		os.Exit(0)
	}
	defer SetupDiDestroyDeviceInfoList(handle)

	// Sortiert das Array nach Namen
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].DeviceDesc < devices[j].DeviceDesc
	})

	AllDevices := devices

	var LineEditSearch *walk.LineEdit
	// The row the last right click on the table's rows landed on. A right
	// click on the column header opens the same menu without changing it.
	// -1 when there is none to go by.
	menuRow := -1
	mw := &MyMainWindow{
		model: &Model{items: devices},
		tv:    &walk.TableView{},
	}
	if err := (MainWindow{
		AssignTo: &mw.MainWindow,
		Title:    "GoInterruptPolicy - " + Version,
		Icon:     2,
		MinSize: Size{
			Width:  240,
			Height: 320,
		},
		Size: Size{
			Width:  750,
			Height: 600,
		},
		Layout: VBox{
			MarginsZero: true,
			SpacingZero: true,
		},
		Children: []Widget{
			Composite{
				Layout: VBox{},
				Children: []Widget{
					LineEdit{
						AssignTo:  &LineEditSearch,
						CueBanner: "Search",
						OnTextChanged: func() {
							text := strings.ToLower(LineEditSearch.Text())
							if text == "" {
								mw.tv.SetModel(&Model{items: AllDevices})
								mw.sbi.SetText(fmt.Sprintf("%d Devices Found", len(devices)))
							} else {
								newDevices := []Device{}
								for i := range AllDevices {
									if strings.Contains(strings.ToLower(AllDevices[i].DeviceDesc), text) ||
										strings.Contains(strings.ToLower(AllDevices[i].DevObjName), text) ||
										strings.Contains(strings.ToLower(AllDevices[i].LocationInformation), text) ||
										strings.Contains(strings.ToLower(AllDevices[i].FriendlyName), text) {
										newDevices = append(newDevices, AllDevices[i])
									}
								}
								mw.tv.SetModel(&Model{items: newDevices})
								mw.sbi.SetText(fmt.Sprintf("%d Devices Found", len(newDevices)))
							}
						},
					},
				},
			},
			TableView{
				OnItemActivated:     mw.lb_ItemActivated,
				AlternatingRowBG:    true,
				ColumnsOrderable:    true,
				ColumnsSizable:      true,
				LastColumnStretched: true,
				MultiSelection:      true,
				ContextMenuItems: []MenuItem{
					Action{
						Text: "&Device policy",
						OnTriggered: func() {
							SelectedIndexes := mw.tv.SelectedIndexes()
							if len(SelectedIndexes) != 0 {
								deviceList := make([]Device, len(SelectedIndexes))
								for arrayIndex, selectionIndex := range SelectedIndexes {
									deviceList[arrayIndex] = mw.tv.Model().(*Model).items[selectionIndex]
								}

								result, d, err := RunDialog(mw, deviceList)
								if err != nil {
									log.Print(err)
									return
								}

								if result == 0 || result == win.IDCANCEL {
									return
								}

								mw.restartPopup = true
								for arrayIndex, selectionIndex := range SelectedIndexes {
									mw.tv.Model().(*Model).items[selectionIndex] = mw.SetNewDevice(&mw.tv.Model().(*Model).items[selectionIndex], &d[arrayIndex])
									mw.updateFullList(mw.tv.Model().(*Model).items[selectionIndex])
								}

								for _, selectionIndex := range SelectedIndexes {
									mw.tv.UpdateItem(selectionIndex)
								}
							}
						},
					},

					Action{
						Text: "&Hardware properties",
						OnTriggered: func() {
							SelectedIndexes := mw.tv.SelectedIndexes()
							if len(SelectedIndexes) != 0 {
								for _, index := range SelectedIndexes {
									d := mw.tv.Model().(*Model).items[index]
									if id, err := d.getInstanceID(); err == nil {
										go showDeviceProperties(win.HWND(0), id)
									}
								}
							}
						},
					},

					Action{
						Text: "&Registry parameters",
						OnTriggered: func() {
							// The row the menu was opened on if it is selected,
							// else the current row if it is, else the first one
							// that is. A right click on a row that is already
							// selected keeps the selection and leaves walk's
							// current row where it was, so with several rows
							// selected the current row could be another one. The
							// current row is -1 at the start and after every
							// search, and a right click below the rows clears the
							// selection but leaves it, so it alone cannot be
							// trusted either.
							selected := mw.tv.SelectedIndexes()
							index := menuRow
							if !slices.Contains(selected, index) {
								index = mw.tv.CurrentIndex()
							}
							if !slices.Contains(selected, index) {
								if len(selected) == 0 {
									return
								}
								index = selected[0]
							}
							d := mw.tv.Model().(*Model).items[index]
							OpenRegistry(mw.Form(), d.reg)
						},
					},
				},
				Model:    mw.model,
				AssignTo: &mw.tv,
				// walk reports a click on the table's rows in the
				// coordinates IndexAt takes. A key press in the table, which a
				// menu opened from the keyboard follows, forgets the row. walk
				// reports neither F10 nor a held key's repeats, so Shift+F10
				// with Shift held down since before the right click still goes
				// by that row, which is selected and the one last clicked.
				OnMouseDown: func(x, y int, button walk.MouseButton) {
					menuRow = -1
					if button == walk.RightButton {
						menuRow = mw.tv.IndexAt(x, y)
					}
				},
				OnKeyDown: func(walk.Key) {
					menuRow = -1
				},
				OnKeyUp: func(key walk.Key) {
					// The rows the table shows, which while a search is shown
					// are not mw.model's.
					items := mw.tv.Model().(*Model).items
					i := mw.tv.CurrentIndex()
					if i == -1 {
						i = 0
					}
					for ; i < len(items); i++ {
						item := &items[i]
						if item.DeviceDesc != "" && key.String() == item.DeviceDesc[0:1] {
							if err := mw.tv.SetCurrentIndex(i); err != nil {
								log.Println(err)
							}
							return
						}
					}
				},
				Columns: []TableViewColumn{
					{
						Name:  "DeviceDesc",
						Title: "Name",
						Width: 150,
					},
					{
						Name:  "FriendlyName",
						Title: "Friendly Name",
					},

					{
						Name: "Class",
					},
					{
						Name:  "LocationInformation",
						Title: "Location Info",
						Width: 150,
					},
					{
						Name:      "MsiSupported",
						Title:     "MSI Mode",
						Width:     60,
						Alignment: AlignCenter,
						FormatFunc: func(value any) string {
							switch value.(uint32) {
							case 0:
								return "✖"
							case 1:
								return "✔"
							}
							return ""
						},
					},
					{
						Name:  "DevicePolicy",
						Title: "Device Policy",
						FormatFunc: func(value any) string {
							// https://docs.microsoft.com/en-us/windows-hardware/drivers/kernel/interrupt-affinity-and-priority
							switch value.(uint32) {
							case IrqPolicyMachineDefault: // 0x00
								return "Default"
							case IrqPolicyAllCloseProcessors: // 0x01
								return "All Close Proc"
							case IrqPolicyOneCloseProcessor: // 0x02
								return "One Close Proc"
							case IrqPolicyAllProcessorsInMachine: // 0x03
								return "All Proc in Machine"
							case IrqPolicySpecifiedProcessors: // 0x04
								return "Specified Proc"
							case IrqPolicySpreadMessagesAcrossAllProcessors: // 0x05
								return "Spread Messages Across All Proc"
							default:
								return fmt.Sprintf("%d", value.(uint32))
							}
						},
					},
					{
						Name:  "AssignmentSetOverride",
						Title: "Specified Processor",
						FormatFunc: func(value any) string {
							if value == ZeroBit {
								return ""
							}
							bits := value.(Bits)
							var result []string
							for bit, cpu := range CPUMap {
								if Has(bit, bits) {
									result = append(result, cpu)
								}
							}

							result, err := sortNumbers(result)
							if err != nil {
								log.Println(err)
							}
							return strings.Join(result, ",")
						},
						LessFunc: func(i, j int) bool {
							items := mw.tv.Model().(*Model).items
							return items[i].AssignmentSetOverride < items[j].AssignmentSetOverride
						},
					},
					{
						Name:  "DevicePriority",
						Title: "Device Priority",
						FormatFunc: func(value any) string {
							switch value.(uint32) {
							case 0:
								return "Undefined"
							case 1:
								return "Low"
							case 2:
								return "Normal"
							case 3:
								return "High"
							default:
								return fmt.Sprintf("%d", value.(uint32))
							}
						},
					},
					{
						Name:  "InterruptTypeMap",
						Title: "Interrupt Type",
						Width: 120,
						FormatFunc: func(value any) string {
							return interruptType(value.(Bits))
						},
						LessFunc: func(i, j int) bool {
							items := mw.tv.Model().(*Model).items
							return items[i].InterruptTypeMap < items[j].InterruptTypeMap
						},
					},
					{
						Name:  "MessageNumberLimit",
						Title: "MSI Limit",
						FormatFunc: func(value any) string {
							switch value.(uint32) {
							case 0:
								return ""
							default:
								return fmt.Sprintf("%d", value.(uint32))
							}
						},
					},
					{
						Name:  "MaxMSILimit",
						Title: "Max MSI Limit",
						FormatFunc: func(value any) string {
							switch value.(uint32) {
							case 0:
								return ""
							default:
								return fmt.Sprintf("%d", value.(uint32))
							}
						},
					},

					{
						Name:  "IRQLanes",
						Title: "IRQ Lanes",
						FormatFunc: func(value any) string {
							return strings.Join(value.([]string), ", ")
						},
					},

					{
						Name:  "DevObjName",
						Title: "DevObj Name",
					},
				},
			},
		},
		StatusBarItems: []StatusBarItem{
			{
				AssignTo: &mw.sbi,
				Text:     fmt.Sprintf("%d Devices Found", len(devices)),
			},
		},
	}).Create(); err != nil {
		log.Println(err)
		return
	}

	var maxDeviceDesc int
	for i := range devices {
		newDeviceDesc := mw.TextWidthSize(devices[i].DeviceDesc)
		if maxDeviceDesc < newDeviceDesc {
			maxDeviceDesc = newDeviceDesc
		}
	}
	// Microsoft documents that a list view column only as wide as its text
	// cuts the text off, and that padding has to be added, without saying how
	// much.
	const nameColumnPadding = 16
	if maxDeviceDesc+nameColumnPadding < 150 {
		mw.tv.Columns().At(0).SetWidth(maxDeviceDesc + nameColumnPadding)
	}

	mw.Show()
	mw.tv.SetFocus()
	mw.Run()
}

// cliRequest is what the command line asks to write. A flag that was not
// given is -1.
type cliRequest struct {
	policy, priority, msi, limit int

	cpu    Bits     // the processors -cpu named that this machine has
	badCPU []string // the -cpu entries that name none of them
}

// cliRefusals says why the command line cannot write to dev what req asks, if
// it cannot. threads is how many processors this machine has that a mask can
// name.
//
// Each of these used to be written anyway, as something the user did not ask
// for or Microsoft does not document. A number below 0, or above what a
// registry DWORD holds, went to the registry wrapped round, and an
// -msisupported other than 0 or 1 wrote nothing. A -cpu naming processors this
// machine cannot address wrote the rest of them, or none. A policy of
// specified processors with no processor went to the registry as such,
// although the dialog refuses it. A limit with MSI not on, or processors with
// any other policy, is a setting the registry code drops for the state the
// device ends in, deleting the MSI key, or the processors value, in its place,
// and that counted as a change for -restart-on-change. And a policy, priority
// or MSI limit Microsoft does not document was written as given, as was one
// the device already had when its key was written again.
func cliRefusals(dev *Device, req cliRequest, threads int) []string {
	var refused []string
	for _, flag := range []struct {
		name  string
		value int
		most  int
	}{{"-policy", req.policy, IrqPolicySpreadMessagesAcrossAllProcessors}, {"-priority", req.priority, IrqPriorityHigh}, {"-msilimit", req.limit, 2048}} {
		if flag.value < -1 || flag.value > flag.most {
			refused = append(refused, fmt.Sprintf("%s %d: it takes a number from 0 to %d", flag.name, flag.value, flag.most))
		}
	}
	if req.msi < -1 || req.msi > 1 {
		refused = append(refused, fmt.Sprintf("-msisupported %d: it takes 0 for off or 1 for on", req.msi))
	}
	if len(req.badCPU) != 0 {
		refused = append(refused, fmt.Sprintf("-cpu %s: this machine's addressable processors are 0 to %d", strings.Join(req.badCPU, ","), threads-1))
	}

	// The rules below read the flags as settings, which these are not.
	if len(refused) != 0 {
		return refused
	}

	policy, priority, mask := dev.DevicePolicy, dev.DevicePriority, dev.AssignmentSetOverride
	msi, limit := dev.MsiSupported, dev.MessageNumberLimit
	if req.policy != -1 {
		policy = uint32(req.policy)
	}
	if req.priority != -1 {
		priority = uint32(req.priority)
	}
	if req.msi != -1 {
		msi = uint32(req.msi)
	}
	if req.limit != -1 {
		limit = uint32(req.limit)
	}
	if req.cpu != ZeroBit {
		mask = req.cpu
	}

	// A limit of 0 is no limit, which MSI off (0) leaves too. A device whose
	// MSISupported holds anything but 0 or 1, written by some other tool, gets
	// no MSI write at all from the registry code, so its stored limit would
	// stay as it is.
	if req.limit == 0 && msi != MSI_On && msi != MSI_Off && dev.MessageNumberLimit != 0 {
		refused = append(refused, fmt.Sprintf("-msilimit 0: the device's MSISupported is %d, which this program does not write: add -msisupported 0 or 1", msi))
	}
	if req.limit > 0 && msi != MSI_On {
		switch {
		case req.msi == 0:
			refused = append(refused, "-msilimit needs MSI on, and -msisupported 0 turns it off")
		case msi == MSI_Invalid:
			refused = append(refused, "-msilimit: the device reports no interrupt types, so it has no MSI settings here to limit")
		default:
			refused = append(refused, "-msilimit needs MSI on: add -msisupported 1")
		}
	}

	if req.cpu != ZeroBit && policy != IrqPolicySpecifiedProcessors {
		if req.policy != -1 {
			refused = append(refused, fmt.Sprintf("-cpu needs -policy 4, not -policy %d", req.policy))
		} else {
			refused = append(refused, "-cpu needs the specified processors policy: add -policy 4")
		}
	}

	// Only when -policy, -priority or -cpu is given, even one the device
	// already has and so would not write: a device left on specified
	// processors with none is refused any of them rather than passed over. A
	// run with none of them, such as one that changes only MSI, leaves the
	// policy as it is. A mask of only processors this machine does not have
	// counts as none: -cpu cannot name one, but the device's own mask can.
	absent := absentProcessors(threads)
	writesAffinity := req.policy != -1 || req.priority != -1 || req.cpu != ZeroBit
	if writesAffinity && policy == IrqPolicySpecifiedProcessors && mask&^absent == ZeroBit {
		if req.policy == IrqPolicySpecifiedProcessors {
			refused = append(refused, "-policy 4 needs at least one processor: add -cpu")
		} else {
			refused = append(refused, "the device is on the specified processors policy with no processor this machine has: add -cpu, or a -policy other than 4")
		}
	}

	// A value Microsoft does not document that the device already has is
	// refused when the run writes its key again, which it does whole, and
	// only then. One given here was refused above, and -cpu replaces the mask
	// with processors this machine has.
	rewritesAffinity := policy != dev.DevicePolicy || priority != dev.DevicePriority || mask != dev.AssignmentSetOverride
	if rewritesAffinity && policy == IrqPolicySpecifiedProcessors && mask&^absent != ZeroBit && mask&absent != ZeroBit {
		if req.policy == IrqPolicySpecifiedProcessors {
			refused = append(refused, "-policy 4: the device's mask names processors this machine does not have: add -cpu")
		} else {
			refused = append(refused, "the device's mask names processors this machine does not have: add -cpu, or a -policy other than 4")
		}
	}
	if rewritesAffinity && policy > IrqPolicySpreadMessagesAcrossAllProcessors {
		refused = append(refused, fmt.Sprintf("the device's DevicePolicy is %d, which Microsoft does not document for a device: add -policy", policy))
	}
	if rewritesAffinity && priority > IrqPriorityHigh {
		refused = append(refused, fmt.Sprintf("the device's DevicePriority is %d, which Microsoft does not document: add -priority", priority))
	}

	// MSI on writes the limit with it, the device's own when -msilimit is not
	// given, but only when it turns MSI on: -msisupported 1 for a device
	// already on writes nothing. A limit given with -msilimit is checked
	// always. Microsoft documents 1, 2, 4, 8 or 16 for MSI and up to 2048 for
	// MSI-X.
	writesMSI := req.limit != -1 || msi != dev.MsiSupported
	if writesMSI && msi == MSI_On && !limitTaken(limit, dev.InterruptTypeMap) {
		takes := "a device without MSI-X takes 1, 2, 4, 8 or 16"
		if Has(dev.InterruptTypeMap, msiXBit) {
			takes = "an MSI-X device takes up to 2048"
		}
		if req.limit != -1 {
			refused = append(refused, fmt.Sprintf("-msilimit %d: %s, or 0 for no limit", limit, takes))
		} else {
			refused = append(refused, fmt.Sprintf("the device's MSI limit is %d, and %s: add -msilimit", limit, takes))
		}
	}

	return refused
}

type MyMainWindow struct {
	*walk.MainWindow
	tv           *walk.TableView
	model        *Model
	sbi          *walk.StatusBarItem
	restartPopup bool
}

func (mw *MyMainWindow) lb_ItemActivated() {
	currentIndex := mw.tv.CurrentIndex()
	newItem := mw.tv.Model().(*Model).items[currentIndex]

	result, d, err := RunDialog(mw, []Device{newItem})
	if err != nil {
		log.Print(err)
		return
	}
	if result == 0 || result == win.IDCANCEL {
		return
	}

	mw.restartPopup = true
	mw.tv.Model().(*Model).items[currentIndex] = mw.SetNewDevice(&mw.tv.Model().(*Model).items[currentIndex], &d[0])
	mw.updateFullList(mw.tv.Model().(*Model).items[currentIndex])

	mw.tv.UpdateItem(currentIndex)
}

// updateFullList puts d into the full device list in place of the device it
// is. While a search is shown the table holds copies of the devices found, so
// an edit made there has to be copied back, or changing or clearing the search
// shows the device as it was, and the next edit of it starts from that and can
// write the old values back.
func (mw *MyMainWindow) updateFullList(d Device) {
	for i := range mw.model.items {
		if mw.model.items[i].Idata.DevInst == d.Idata.DevInst {
			mw.model.items[i] = d
			return
		}
	}
}

func (mw *MyMainWindow) SetNewDevice(orgItem *Device, newItem *Device) Device {
	var changed bool

	if orgItem.MsiSupported != newItem.MsiSupported || orgItem.MessageNumberLimit != newItem.MessageNumberLimit {
		changed = orgItem.setMSIMode(newItem)
	}

	if orgItem.DevicePolicy != newItem.DevicePolicy || orgItem.DevicePriority != newItem.DevicePriority || orgItem.AssignmentSetOverride != newItem.AssignmentSetOverride {
		changed = orgItem.setAffinityPolicy(newItem) || changed
	}

	if mw.restartPopup && changed {
		switch walk.MsgBox(mw.WindowBase.Form(), "Restart Device?", fmt.Sprintf("Your changes will not take effect until the device is restarted.\n\nWould you like to attempt to restart the device now?\n\n%s\n%s", orgItem.DeviceDesc, orgItem.DevObjName), walk.MsgBoxYesNoCancel|walk.MsgBoxIconQuestion) {
		case win.IDCANCEL:
			mw.restartPopup = false
			fallthrough
		case win.IDNO:
			mw.sbi.SetText("Restart required")
		case win.IDYES:
			propChangeParams := PropChangeParams{
				ClassInstallHeader: *MakeClassInstallHeader(DIF_PROPERTYCHANGE),
				StateChange:        DICS_PROPCHANGE,
				Scope:              DICS_FLAG_GLOBAL,
			}

			if err := SetupDiSetClassInstallParams(handle, &orgItem.Idata, &propChangeParams.ClassInstallHeader, uint32(unsafe.Sizeof(propChangeParams))); err != nil {
				log.Println(err)
				return *orgItem
			}

			if err := SetupDiCallClassInstaller(DIF_PROPERTYCHANGE, handle, &orgItem.Idata); err != nil {
				log.Println(err)
				return *orgItem
			}

			if err := SetupDiSetClassInstallParams(handle, &orgItem.Idata, &propChangeParams.ClassInstallHeader, uint32(unsafe.Sizeof(propChangeParams))); err != nil {
				log.Println(err)
				return *orgItem
			}

			if err := SetupDiCallClassInstaller(DIF_PROPERTYCHANGE, handle, &orgItem.Idata); err != nil {
				log.Println(err)
				return *orgItem
			}

			DeviceInstallParams, err := SetupDiGetDeviceInstallParams(handle, &orgItem.Idata)
			if err != nil {
				log.Println(err)
				return *orgItem
			}

			if DeviceInstallParams.Flags&(DI_NEEDREBOOT|DI_NEEDRESTART) != 0 {
				walk.MsgBox(mw.WindowBase.Form(), "Notice", "Device could not be restarted. Changes will take effect the next time you reboot.", walk.MsgBoxIconWarning)
			} else {
				walk.MsgBox(mw.WindowBase.Form(), "Notice", "Device successfully restarted.", walk.MsgBoxIconInformation)
			}

		}
	}
	return *orgItem
}

// TextWidthSize is how wide text is drawn in the table's font, in the 96 dpi
// units column widths are given in. Measured, it is in pixels, which at any
// scaling above 100% made a column that much wider than the text. It is
// measured with an & as an &, the way the table draws it, and not as the
// mark of an underlined letter, which takes no width.
func (mw *MyMainWindow) TextWidthSize(text string) int {
	canvas, err := (*mw.tv).CreateCanvas()
	if err != nil {
		return 0
	}
	defer canvas.Dispose()

	bounds, _, err := canvas.MeasureTextPixels(text, (*mw.tv).Font(), walk.Rectangle{Width: 9999999}, walk.TextCalcRect|walk.TextNoPrefix)
	if err != nil {
		return 0
	}

	return mw.tv.IntTo96DPI(bounds.Size().Width)
}

type Model struct {
	// walk.SortedReflectTableModelBase
	items []Device
}

func (m *Model) Items() any {
	return m.items
}

func sortNumbers(data []string) ([]string, error) {
	var lastErr error
	sort.Slice(data, func(i, j int) bool {
		a, err := strconv.ParseInt(data[i], 10, 64)
		if err != nil {
			lastErr = err
			return false
		}
		b, err := strconv.ParseInt(data[j], 10, 64)
		if err != nil {
			lastErr = err
			return false
		}
		return a < b
	})
	return data, lastErr
}
