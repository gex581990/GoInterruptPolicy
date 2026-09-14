package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/tailscale/walk"

	//lint:ignore ST1001 standard behavior tailscale/walk
	. "github.com/tailscale/walk/declarative"
)

// deviceTextMinSize is the width the three device labels are never drawn
// narrower than, and groupNoteMinSize the width the processor group note wraps
// at. Widths only, so the height is left to the text.
//
// These are the one place a number of this sort is named, and both are named
// for the same reason: the string is Windows', of a length nobody here decides,
// and a label whose minimum is its own text would let one long string set the
// width of the whole dialog. A floor bounds what a string can do to the layout
// while leaving room to read it. Like every other size in the declarative
// layout they are 96 dpi units, which walk scales to the monitor.
var (
	deviceTextMinSize = Size{Width: 240}
	groupNoteMinSize  = Size{Width: 380}
)

type ComboBoxIntStruct struct {
	Enums int
	Name  string
}

type ComboBoxUintStruct struct {
	Enums uint32
	Name  string
}

func NewComboBoxModel(names []string) []*ComboBoxUintStruct {
	items := make([]*ComboBoxUintStruct, len(names))
	for i, n := range names {
		items[i] = &ComboBoxUintStruct{
			Enums: uint32(i),
			Name:  n,
		}
	}
	return items
}

type CheckBoxList struct {
	Widget []Widget // Unused: nothing reads or writes this field.
	// List holds the checkbox of every logical processor, indexed by its group
	// relative processor number.
	List      []*walk.CheckBox
	CoreIndex int
	// grids holds the composites whose grid layout carries the core boxes. The
	// declarative builder fills these in, which is why they are pointers to
	// pointers, and setGridRows changes how many columns each one uses.
	grids []**walk.Composite
}

// newGrid hands the builder somewhere to record a grid composite and keeps the
// handle so the columns can be changed later.
func (c *CheckBoxList) newGrid() **walk.Composite {
	holder := new(*walk.Composite)
	c.grids = append(c.grids, holder)

	return holder
}

// Grids returns the grid composites the builder actually created.
func (c *CheckBoxList) Grids() []*walk.Composite {
	out := make([]*walk.Composite, 0, len(c.grids))
	for _, holder := range c.grids {
		if *holder != nil {
			out = append(out, *holder)
		}
	}

	return out
}

func ListDevices(devices []Device) []*ComboBoxIntStruct {
	out := make([]*ComboBoxIntStruct, len(devices))
	for i := range devices {
		out[i] = &ComboBoxIntStruct{
			Enums: i,
			Name:  devices[i].DeviceDesc,
		}
	}

	return out
}

func RunDialog(owner walk.Form, devices []Device) (int, Device, error) {
	var dlg *walk.Dialog
	var db *walk.DataBinder
	var acceptPB, cancelPB *walk.PushButton
	var cpuArrayComView *walk.Composite
	var dialogBody *walk.Composite
	var dialogScroll *walk.ScrollView
	var grids coreGrids
	var devicePolicyCB, devicePriorityCB, openRegistryCB, openDeviceManagerCB *walk.ComboBox
	var MsiSupportedCB *walk.CheckBox
	var deviceMessageNumberLimitNE *walk.NumberEdit
	var checkBoxList = new(CheckBoxList)
	var title string
	var DevObjName, DeviceDesc, LocationInformation []string

	for i := range devices {
		DevObjName = append(DevObjName, devices[i].DevObjName)
		DeviceDesc = append(DeviceDesc, devices[i].DeviceDesc)
		if devices[i].LocationInformation == "" {
			LocationInformation = append(LocationInformation, "N/A")
		} else {
			LocationInformation = append(LocationInformation, devices[i].LocationInformation)
		}
	}

	if len(devices) == 1 {
		title = fmt.Sprintf("Device Policy - %s", devices[0].DeviceDesc)
	} else {
		title = fmt.Sprintf("Device Policy - %d devices", len(devices))
	}

	device := &Device{
		DevObjName:            strings.Join(DevObjName, ", "),
		DeviceDesc:            strings.Join(DeviceDesc, ", "),
		LocationInformation:   strings.Join(LocationInformation, ", "),
		DevicePolicy:          FindCommonValue(devices, 32, func(d Device) uint32 { return d.DevicePolicy }),   // Empty
		DevicePriority:        FindCommonValue(devices, 32, func(d Device) uint32 { return d.DevicePriority }), // Empty
		AssignmentSetOverride: FindCommonValue(devices, 0, func(d Device) Bits { return d.AssignmentSetOverride }),
		MsiSupported:          FindCommonValue(devices, MSI_Tristate, func(d Device) uint32 { return d.MsiSupported }), // Tristate
		MessageNumberLimit:    FindCommonValue(devices, 0, func(d Device) uint32 { return d.MessageNumberLimit }),
		InterruptTypeMap:      FindCommonValue(devices, 0, func(d Device) Bits { return d.InterruptTypeMap }),
		MaxMSILimit:           FindCommonValue(devices, 0, func(d Device) uint32 { return d.MaxMSILimit }),
	}

	dialog := Dialog{
		AssignTo:      &dlg,
		Title:         title,
		Icon:          2,
		DefaultButton: &acceptPB,
		CancelButton:  &cancelPB,
		DataBinder: DataBinder{
			AssignTo:       &db,
			Name:           "device",
			DataSource:     device,
			ErrorPresenter: ToolTipErrorPresenter{},
		},
		Layout: VBox{
			MarginsZero: true,
		},
		Children: []Widget{
			// Everything but the OK / Cancel row scrolls, so those two stay
			// reachable no matter how many processors the machine has.
			ScrollView{
				AssignTo: &dialogScroll,
				// One child, so the spacing would only ever pay for the trailing
				// spacer walk appends to a box layout inside a ScrollView, and
				// that would make the dialog a gap too short for its content.
				Layout: VBox{MarginsZero: true, SpacingZero: true},
				Children: []Widget{
					// The content is kept at the width it asks for and centred in
					// whatever is left, rather than stretched across the window.
					//
					// Stretching is what was cutting text off. A window wider than
					// the content has to put that width somewhere, and a layout
					// shares it out a column at a time against bounds it works out
					// per column, so at some widths a control ends up with less
					// than its text needs and is clipped mid word. At its own
					// width there is nothing to share out and every control gets
					// exactly what it asked for, at every window size.
					//
					// The size the content is drawn at is chosen once, by
					// fitDialogAtOpen, and the two spacers take whatever the
					// window has over and above it.
					Composite{
						Layout: HBox{MarginsZero: true, SpacingZero: true},
						Children: []Widget{
							HSpacer{},
							Composite{
								AssignTo: &dialogBody,
								Layout:   VBox{},
								Children: []Widget{
									// These three carry whatever Windows calls the device,
									// which is a string of no known length, and they are
									// why the grid has no spacer across its two columns.
									//
									// The floor and the ellipsis are a pair, and it takes
									// both. A label allowed to ellipsise reports a minimum
									// width of zero, and a layout hands out minimums first
									// and shares out what is left afterwards, so a bare
									// ellipsising label is cut whenever the sharing out
									// falls short, which is what cut these three at sizes
									// where there was clearly room. A label not allowed to
									// ellipsise has its whole text as its minimum instead,
									// and since the content column is held at its own
									// minimum, one long device name would then set the
									// width of the whole dialog and everything in it would
									// be scaled down to fit that name. So the floor is the
									// width they are never cut below, the ellipsis is what
									// a name too long for the dialog does instead of
									// widening it, and the tooltip carries the whole
									// string either way.
									//
									// The spacer went for a reason of its own: walk bounds
									// a column at the widest thing in it, but reads that
									// bound from a spanning child as unbounded, so a
									// spacer laid across both columns left the label
									// column free to swallow the window and push the
									// values into the middle of it.
									Composite{
										Layout: Grid{
											Columns: 2,
										},
										Children: []Widget{
											Label{
												Text: "Name:",
											},
											Label{
												EllipsisMode: EllipsisEnd,
												MinSize:      deviceTextMinSize,
												ToolTipText:  strings.Join(DeviceDesc, "\n"),
												Text:         Bind("device.DeviceDesc == '' ? 'N/A' : device.DeviceDesc"),
											},

											Label{
												Text: "Location Info:",
											},
											Label{
												EllipsisMode: EllipsisEnd,
												MinSize:      deviceTextMinSize,
												ToolTipText:  strings.Join(LocationInformation, "\n"),
												Text:         Bind("device.LocationInformation == '' ? 'N/A' : device.LocationInformation"),
											},

											Label{
												Text: "DevObj Name:",
											},
											Label{
												EllipsisMode: EllipsisEnd,
												MinSize:      deviceTextMinSize,
												ToolTipText:  strings.Join(DevObjName, "\n"),
												Text:         Bind("device.DevObjName == '' ? 'N/A' : device.DevObjName"),
											},
										},
									},

									GroupBox{
										Title:   "Message Signaled-Based Interrupts",
										Visible: device.MsiSupported != MSI_Invalid,
										Layout:  Grid{Columns: 1},
										Children: []Widget{

											CheckBox{
												AssignTo:       &MsiSupportedCB,
												Name:           "MsiSupported",
												Text:           "MSI Mode:",
												TextOnLeftSide: true,
												Tristate:       device.MsiSupported == MSI_Tristate,
												Checked:        device.MsiSupported == MSI_On,
												OnClicked: func() {
													if MsiSupportedCB.Checked() {
														device.MsiSupported = MSI_On
														deviceMessageNumberLimitNE.SetEnabled(true)
														device.MessageNumberLimit = uint32(deviceMessageNumberLimitNE.Value())
													} else {
														device.MsiSupported = MSI_Off
														deviceMessageNumberLimitNE.SetEnabled(false)
													}
												},
											},

											Composite{
												Layout: Grid{
													Columns:     3,
													MarginsZero: true,
												},
												Children: []Widget{
													LinkLabel{
														Text: `MSI Limit: <a href="https://forums.guru3d.com/threads/windows-line-based-vs-message-signaled-based-interrupts-msi-tool.378044/">?</a>`,
														OnLinkActivated: func(link *walk.LinkLabelLink) {
															// https://stackoverflow.com/a/12076082
															exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", link.URL()).Start()
														},
													},
													NumberEdit{
														SpinButtonsVisible: true,
														AssignTo:           &deviceMessageNumberLimitNE,
														Enabled:            device.MsiSupported == MSI_On,
														MinValue:           0,
														MaxValue:           hasMsiX(device.InterruptTypeMap),
														Value:              Bind("device.MessageNumberLimit < 1.0 ? 1.0 : device.MessageNumberLimit"),
														OnValueChanged: func() {
															device.MessageNumberLimit = uint32(deviceMessageNumberLimitNE.Value())
														},
													},
												},
											},

											Label{
												Text: "Interrupt Type: " + interruptType(device.InterruptTypeMap),
											},

											Label{
												Text: Bind("device.MaxMSILimit == 0 ? '' : 'Max MSI Limit: ' + device.MaxMSILimit"),
											},
										},
									},

									GroupBox{
										Title:  "Advanced Policies",
										Layout: VBox{},
										Children: []Widget{
											Composite{
												Layout: Grid{
													Columns:     2,
													MarginsZero: true,
												},
												Children: []Widget{
													LinkLabel{
														Text: `Device Priority: <a href="https://docs.microsoft.com/en-us/windows-hardware/drivers/ddi/miniport/ne-miniport-_irq_priority">?</a>`,
														OnLinkActivated: func(link *walk.LinkLabelLink) {
															// https://stackoverflow.com/a/12076082
															exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", link.URL()).Start()
														},
													},
													ComboBox{
														AssignTo:      &devicePriorityCB,
														Value:         device.DevicePriority,
														BindingMember: "Enums",
														DisplayMember: "Name",
														Model:         NewComboBoxModel([]string{"Undefined", "Low", "Normal", "High"}),
														OnCurrentIndexChanged: func() {
															device.DevicePriority = uint32(devicePriorityCB.CurrentIndex())
														},
													},

													LinkLabel{
														Text: `Device Policy: <a href="https://docs.microsoft.com/en-us/windows-hardware/drivers/ddi/miniport/ne-miniport-_irq_device_policy">?</a>`,
														OnLinkActivated: func(link *walk.LinkLabelLink) {
															// https://stackoverflow.com/a/12076082
															exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", link.URL()).Start()
														},
													},
													ComboBox{
														AssignTo:      &devicePolicyCB,
														Value:         device.DevicePolicy,
														BindingMember: "Enums",
														DisplayMember: "Name",
														Model: NewComboBoxModel([]string{
															"IrqPolicyMachineDefault",
															"IrqPolicyAllCloseProcessors",
															"IrqPolicyOneCloseProcessor",
															"IrqPolicyAllProcessorsInMachine",
															"IrqPolicySpecifiedProcessors",
															"IrqPolicySpreadMessagesAcrossAllProcessors",
														}),
														OnCurrentIndexChanged: func() {
															currentIndex := uint32(devicePolicyCB.CurrentIndex())
															if device.DevicePolicy == currentIndex {
																return
															}

															device.DevicePolicy = currentIndex

															// IrqPolicySpecifiedProcessors
															cpuArrayComView.SetVisible(device.DevicePolicy == 4)

															// The processor list is inside a ScrollView, so
															// showing or hiding it does not change the layout
															// minimum and walk leaves the dialog at its old
															// size. Lay it out again and grow or shrink it
															// here instead.
															fitDialogAtOpen(grids, workArea(dlg.Handle()).Size())
															fitDialogToContent(dlg, dialogScroll)
														},
													},
												},
											},

											Composite{
												AssignTo: &cpuArrayComView,
												Layout:   VBox{MarginsZero: true},
												Visible:  Bind("device.DevicePolicy == 4"), // IrqPolicySpecifiedProcessors
												Children: []Widget{
													// On a machine of more than one processor group, say why
													// the list stops at group 0. It is where an ordinary
													// device's interrupts are delivered, so this is a note
													// about the machine and not a warning.
													//
													// A TextLabel with a width set wraps, a Label does not,
													// and a sentence this long on one line would be the
													// widest thing in the dialog and would set the width of
													// everything else. Which would land on exactly the
													// machines this note is for, since they are the ones
													// with the most cores to lay out already.
													TextLabel{
														Visible:     cs.Skipped != 0,
														MinSize:     groupNoteMinSize,
														Text:        cs.otherGroupsText(),
														ToolTipText: "Only a group aware driver can put a device's interrupts in another processor group, and it does that for itself. The Affinity Policy registry key holds a single group mask and has no value for a group number.",
													},

													Composite{
														Layout: HBox{
															Alignment:   AlignHCenterVNear,
															MarginsZero: true,
														},
														Children: checkBoxList.create(&device.AssignmentSetOverride),
													},
													GroupBox{
														Title:  "Presets for Specified Processors:",
														Layout: HBox{},
														Children: []Widget{
															PushButton{
																Text: "All On",
																OnClicked: func() {
																	checkBoxList.allOn(&device.AssignmentSetOverride)
																},
															},

															PushButton{
																Text: "All Off",
																OnClicked: func() {
																	checkBoxList.allOff(&device.AssignmentSetOverride)
																},
															},

															PushButton{
																Text:    "HT Off",
																Visible: cs.HyperThreading,
																OnClicked: func() {
																	checkBoxList.htOff(&device.AssignmentSetOverride)
																},
															},

															PushButton{
																Text:    "P-Core Only",
																Visible: cs.EfficiencyClass,
																OnClicked: func() {
																	checkBoxList.pCoreOnly(&device.AssignmentSetOverride)
																},
															},

															PushButton{
																Text:    "E-Core Only",
																Visible: cs.EfficiencyClass,
																OnClicked: func() {
																	checkBoxList.eCoreOnly(&device.AssignmentSetOverride)
																},
															},

															PushButton{
																Text:    checkBoxList.LastLevelCacheName(0),
																Visible: cs.LastLevelCache,
																OnClicked: func() {
																	checkBoxList.LLC(&device.AssignmentSetOverride, 0)
																},
															},

															PushButton{
																Text:    checkBoxList.LastLevelCacheName(1),
																Visible: cs.LastLevelCache,
																OnClicked: func() {
																	checkBoxList.LLC(&device.AssignmentSetOverride, 1)
																},
															},

															HSpacer{},
														},
													},
												},
											},
										},
									},

									GroupBox{
										Title:  "Registry",
										Layout: HBox{},

										Children: []Widget{
											PushButton{
												Text:    "Open Device",
												Visible: len(devices) == 1,
												OnClicked: func() {
													OpenRegistry(dlg.Form(), devices[0].reg)
												},
											},

											Label{
												Visible: len(devices) != 1,
												Text:    "Open Device:",
											},
											ComboBox{
												ToolTipText:   "Open Device",
												AssignTo:      &openRegistryCB,
												Visible:       len(devices) != 1,
												BindingMember: "Enums",
												DisplayMember: "Name",
												Model:         ListDevices(devices),
												OnCurrentIndexChanged: func() {
													i := openRegistryCB.CurrentIndex()
													OpenRegistry(dlg, devices[i].reg)
												},
											},

											PushButton{
												Text: "Export current settings",
												OnClicked: func() {
													var reg_file_value strings.Builder

													for i := range devices {
														regPath, err := GetRegistryLocation(uintptr(devices[i].reg))
														if err != nil {
															walk.MsgBox(dlg, "Error", err.Error(), walk.MsgBoxIconError)
														}

														reg_file_value.WriteString(createRegFile(dlg, regPath, *device))
													}

													path, err := os.Getwd()
													if err != nil {
														log.Println(err)
													}

													// NOTE: The file name can be improved.
													filePath, cancel, err := saveFileExplorer(dlg, path, strings.ReplaceAll(devices[0].DeviceDesc, " ", "_")+".reg", "Save current settings", "Registry File (*.reg)|*.reg")

													// Both, not either. saveFileExplorer reports a failed
													// dialog as cancelled as well as failed, and asking only
													// whether it was cancelled let a failure through to be
													// written to the empty path it came back with, where the
													// error was swallowed and the button did nothing at all.
													if err != nil {
														walk.MsgBox(dlg, "Error", err.Error(), walk.MsgBoxIconError)
														return
													}
													if cancel {
														return
													}

													if err := os.WriteFile(filePath, regFileDocument(reg_file_value.String()), 0o666); err != nil {
														walk.MsgBox(dlg, "Error", err.Error(), walk.MsgBoxIconError)
													}
												},
											},
											HSpacer{},
										},
									},

									GroupBox{
										Title:  "Device Manager",
										Layout: HBox{},
										Children: []Widget{
											PushButton{
												Visible: len(devices) == 1,
												Text:    "Open Device",
												OnClicked: func() {
													if id, err := devices[0].getInstanceID(); err == nil {
														showDeviceProperties(dlg.Handle(), id)
													}
												},
											},

											Label{
												Visible: len(devices) != 1,
												Text:    "Open Device:",
											},
											ComboBox{
												ToolTipText:   "Open Device",
												AssignTo:      &openDeviceManagerCB,
												Visible:       len(devices) != 1,
												BindingMember: "Enums",
												DisplayMember: "Name",
												Model:         ListDevices(devices),
												OnCurrentIndexChanged: func() {
													i := openDeviceManagerCB.CurrentIndex()
													if id, err := devices[i].getInstanceID(); err == nil {
														showDeviceProperties(dlg.Handle(), id)
													}
												},
											},

											HSpacer{},
										},
									},
								},
								// Unused: a Functions entry is reached by name from a Bind("...")
								// expression, and none of the six Bind expressions in this dialog
								// names checkIrqPolicy, viewAsHex or eq, so all three are
								// unreachable. Left as found - they read like they were written
								// for a column or a label that is not here yet.
								Functions: map[string]func(args ...any) (any, error){
									"checkIrqPolicy": func(args ...any) (any, error) {
										for _, v := range NewComboBoxModel([]string{"Undefined", "Low", "Normal", "High"}) {
											if v.Enums == args[0].(uint32) {
												return v.Name, nil
											}
										}
										return "", nil
									},
									"viewAsHex": func(args ...any) (any, error) {
										if args[0].(Bits) == ZeroBit {
											return "N/A", nil
										}
										bits := args[0].(Bits)
										var result []string
										for bit, cpu := range CPUMap {
											if Has(bit, bits) {
												result = append(result, cpu)
											}
										}
										return strings.Join(result, ", "), nil
									},
									"eq": func(args ...any) (any, error) {
										if len(args) != 2 {
											return false, nil
										}
										switch v := args[0].(type) {
										case float64:
											if v == args[1].(float64) {
												return true, nil
											}
										case Bits:
											if v == Bits(args[1].(float64)) {
												return true, nil
											}
										default:
											log.Printf("I don't know about type %T!\n", v)
										}

										return false, nil
									},
								},
							},
							HSpacer{},
						},
					},
				},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo: &acceptPB,
						Text:     "OK",
						OnClicked: func() {
							if device.DevicePolicy == 4 && device.AssignmentSetOverride == ZeroBit {
								walk.MsgBox(dlg, "Invalid Option", "The affinity mask must contain at least one processor.", walk.MsgBoxIconError)
							} else {
								if err := db.Submit(); err != nil {
									return
								}
								dlg.Accept()
							}
						},
					},
					PushButton{
						AssignTo:  &cancelPB,
						Text:      "Cancel",
						OnClicked: func() { dlg.Cancel() },
					},
				},
			},
		},
	}

	// Build and run the dialog in two steps instead of calling Run, so the
	// wanted size is known before (*walk.Dialog).Show picks one.
	if err := dialog.Create(owner); err != nil {
		return 0, *device, err
	}

	screen := dlg.Handle()
	if owner != nil {
		screen = owner.Handle()
	}

	// Draw the dialog at a size the screen can hold, before anything measures
	// it. The font the system chose is the largest this will use here, so a
	// dialog that already fits opens the size its author drew it and only one
	// that does not is scaled down.
	grids = newCoreGrids(dlg, dialogScroll, dialogBody, checkBoxList.Grids())

	// The one and only time the content is laid out. There is deliberately
	// nothing watching the window after this: resizing shows more of the
	// content or less of it and changes nothing else, for the reasons
	// fitDialogAtOpen gives.
	fitDialogAtOpen(grids, workArea(screen).Size())

	startDialogAtContentSize(dlg, dialogScroll, owner)

	return dlg.Run(), *device, nil
}

func EffName(efficiencyClass int) string {
	var title string
	if isIntel() && cs.EfficiencyClass {
		if efficiencyClass == 0 {
			title = "E-Cores"
		} else {
			title = "P-Cores"
		}
	} else { // AMD
		title = fmt.Sprintf("EfficiencyClass %d", efficiencyClass)
	}
	return title
}

func (c *CheckBoxList) createThreads(bits *Bits, threads []int) []Widget {
	var widgets []Widget
	for _, threadValue := range threads {
		c.List[threadValue] = new(walk.CheckBox)
		widgets = append(widgets, CheckBox{
			Text:     fmt.Sprintf("Thread %d", threadValue),
			AssignTo: &c.List[threadValue],
			Checked:  Has(*bits, CPUBits[threadValue]),
			OnClicked: func() {
				*bits = Toggle(CPUBits[threadValue], *bits)
			},
		})
	}
	return widgets
}

func (c *CheckBoxList) createCore(bits *Bits, coreIdx int, threads []int) Widget {
	threadWidgets := c.createThreads(bits, threads)

	return GroupBox{
		Title: fmt.Sprintf("Core %d", coreIdx),
		Layout: VBox{
			Margins: CalculateMargins(cs.MaxThreadsPerCore, len(threadWidgets)),
		},
		Children: threadWidgets,
	}
}

func (c *CheckBoxList) createEffClass(bits *Bits, effIdx, effLen int, cores [][]int) Widget {
	var coreWidgets []Widget

	for coreIdx, threads := range cores {
		if len(threads) == 0 {
			continue
		}
		_ = coreIdx
		coreWidgets = append(coreWidgets, c.createCore(bits, c.CoreIndex, threads))
		c.CoreIndex++
	}

	grid := Composite{
		AssignTo: c.newGrid(),
		Layout: Grid{
			Alignment:   AlignHCenterVCenter,
			MarginsZero: true,
			Columns:     mathCeilInInt(len(coreWidgets), cs.CoreGroups[effIdx].Rows),
		},
		Children: coreWidgets,
	}

	if effLen == 1 {
		return grid
	}

	// Keep the grid on a composite of its own even here, so that every grid
	// setGridRows has to lay out again is the same kind of thing.
	return GroupBox{
		Title:    EffName(effIdx),
		Layout:   VBox{MarginsZero: true},
		Children: []Widget{grid},
	}
}

func (c *CheckBoxList) createCCD(bits *Bits, ccdIdx, ccdLen int, ccd CcdItem) Widget {
	var effWidgets []Widget
	for i := len(ccd.Eff.Nums) - 1; i >= 0; i-- {
		if cores, ok := ccd.Eff.Nums[i]; ok {
			effWidgets = append(effWidgets, c.createEffClass(bits, i, len(ccd.Eff.Nums), cores))
		}
	}

	if ccdLen == 1 {
		return Composite{
			Layout: HBox{
				MarginsZero: true,
			},
			Children: effWidgets,
		}
	}

	return GroupBox{
		Title:       c.LastLevelCacheName(ccdIdx),
		ToolTipText: ToolTipTextNumaNode,
		Layout:      Grid{Columns: 4},
		Children:    effWidgets,
	}
}

func (checkboxlist *CheckBoxList) create(bits *Bits) []Widget {
	// Indexed by logical processor number, which is what createThreads and the
	// preset buttons look up, so it has to be as long as the highest number and
	// not as long as the processor count.
	checkboxlist.List = make([]*walk.CheckBox, cs.Threads)
	var partNUMA []Widget
	for numaIdx, numa := range cs.CoreLayout.Numa {
		var partCache []Widget
		ccdIdx := 0
		for _, ccd := range numa.Ccd {
			if ccd.Eff.isNil() {
				continue
			}

			partCache = append(partCache, checkboxlist.createCCD(bits, ccdIdx, len(numa.Ccd), ccd))
			ccdIdx++
		}

		if len(cs.CoreLayout.Numa) == 1 {
			return partCache
		}

		partNUMA = append(partNUMA, GroupBox{
			Title:       fmt.Sprintf("NUMA %d", numaIdx),
			ToolTipText: ToolTipTextNumaNode,
			Layout:      HBox{},
			Children:    partCache,
		})
	}

	return partNUMA
}

func (checkboxlist *CheckBoxList) allOn(bits *Bits) {
	for i := 0; i < len(checkboxlist.List); i++ {
		// A processor number the CPU set information never reported has no
		// checkbox, so do not claim it in the mask either.
		if checkboxlist.List[i] == nil {
			continue
		}
		*bits = Set(CPUBits[i], *bits)
		checkboxlist.List[i].SetChecked(true)
	}
}
func (checkboxlist *CheckBoxList) allOff(bits *Bits) {
	for i := 0; i < len(checkboxlist.List); i++ {
		if checkboxlist.List[i] == nil {
			continue
		}
		checkboxlist.List[i].SetChecked(false)
	}
	*bits = Bits(0)
}

func (checkboxlist *CheckBoxList) htOff(bits *Bits) {
	for _, numa := range cs.CoreLayout.Numa {
		for _, ccd := range numa.Ccd {
			for i := len(ccd.Eff.Nums) - 1; i >= 0; i-- {
				if cores, ok := ccd.Eff.Nums[i]; ok {
					for _, threads := range cores {
						if len(threads) > 0 {
							for i := 1; i < len(threads); i++ {
								checkboxlist.List[threads[i]].SetChecked(false)
								if Has(CPUBits[threads[i]], *bits) {
									*bits = Toggle(CPUBits[threads[i]], *bits)
								}
							}
						}
					}
				}
			}
		}
	}
}

func (checkboxlist *CheckBoxList) pCoreOnly(bits *Bits) {
	for _, numa := range cs.CoreLayout.Numa {
		for _, ccd := range numa.Ccd {
			if cores, ok := ccd.Eff.Nums[0]; ok {
				for _, CoreValue := range cores {
					for _, ThreadValue := range CoreValue {
						checkboxlist.List[ThreadValue].SetChecked(false)
						if Has(CPUBits[ThreadValue], *bits) {
							*bits = Toggle(CPUBits[ThreadValue], *bits)
						}
					}
				}
			}
			if cores, ok := ccd.Eff.Nums[1]; ok {
				for _, CoreValue := range cores {
					for _, ThreadValue := range CoreValue {
						checkboxlist.List[ThreadValue].SetChecked(true)
						*bits = Set(CPUBits[ThreadValue], *bits)
					}
				}
			}
		}
	}
}

func (checkboxlist *CheckBoxList) eCoreOnly(bits *Bits) {
	for _, numa := range cs.CoreLayout.Numa {
		for _, ccd := range numa.Ccd {
			if cores, ok := ccd.Eff.Nums[0]; ok {
				for _, CoreValue := range cores {
					for _, ThreadValue := range CoreValue {
						checkboxlist.List[ThreadValue].SetChecked(true)
						*bits = Set(CPUBits[ThreadValue], *bits)
					}
				}
			}
			if cores, ok := ccd.Eff.Nums[1]; ok {
				for _, CoreValue := range cores {
					for _, ThreadValue := range CoreValue {
						checkboxlist.List[ThreadValue].SetChecked(false)
						if Has(CPUBits[ThreadValue], *bits) {
							*bits = Toggle(CPUBits[ThreadValue], *bits)
						}
					}
				}
			}
		}
	}
}

func (checkboxlist *CheckBoxList) LastLevelCacheName(llcCount int) string {
	if isAMD() {
		return fmt.Sprintf("CCD %d", llcCount)
	} else {
		return fmt.Sprintf("LLC %d", llcCount)
	}
}

func (checkboxlist *CheckBoxList) LLC(bits *Bits, idx int) {
	for _, numa := range cs.CoreLayout.Numa {
		ccdIdx := 0
		for _, ccd := range numa.Ccd {
			if ccd.Eff.isNil() {
				continue
			}
			for i := len(ccd.Eff.Nums) - 1; i >= 0; i-- {
				if cores, ok := ccd.Eff.Nums[i]; ok {
					for _, CoreValue := range cores {
						for _, ThreadIdx := range CoreValue {
							if ccdIdx == idx {
								checkboxlist.List[ThreadIdx].SetChecked(true)
								*bits = Set(CPUBits[ThreadIdx], *bits)
							} else if Has(CPUBits[ThreadIdx], *bits) {
								checkboxlist.List[ThreadIdx].SetChecked(false)
								*bits = Toggle(CPUBits[ThreadIdx], *bits)
							}
						}
					}
				}
			}
			ccdIdx += 1
		}
	}
}

// https://docs.microsoft.com/de-de/windows-hardware/drivers/kernel/enabling-message-signaled-interrupts-in-the-registry
func hasMsiX(b Bits) float64 {
	if Has(b, Bits(4)) {
		return 2048 // MSIX
	} else {
		return 16 // MSI
	}
}

func interruptType(b Bits) string {
	if b == ZeroBit {
		return ""
	}
	var types []string
	for bit, name := range InterruptTypeMap {
		if Has(b, bit) {
			types = append(types, name)
		}
	}
	sort.Strings(types)
	return strings.Join(types, ", ")
}

// CalculateMargins pads a core box holding fewer threads than the fattest core
// has, so every box comes out the same height and the row of them lines up.
// The counts are arguments rather than read from the package wide cs so that a
// table of them can be checked, and a core of no threads takes the even
// margins since the division below would otherwise be by zero.
func CalculateMargins(maxThreadsPerCore, value int) Margins {
	if maxThreadsPerCore == value || value < 1 {
		return Margins{
			Left:   9,
			Top:    9,
			Right:  9,
			Bottom: 9,
		}
	} else {
		part := (11.75 * float64(maxThreadsPerCore) / float64(value))
		return Margins{
			Left:   9,
			Top:    int(math.Floor(part)),
			Right:  9,
			Bottom: int(math.Ceil(part)),
		}
	}
}

func FindCommonValue[T comparable](devices []Device, defaultValue T, selector func(Device) T) T {
	if len(devices) == 0 {
		return defaultValue
	}

	firstValue := selector(devices[0])
	for i := 0; i < len(devices[1:]); i++ {
		if selector(devices[1:][i]) != firstValue {
			return defaultValue
		}
	}

	return firstValue
}
