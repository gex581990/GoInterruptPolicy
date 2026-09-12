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
	// List holds the checkbox of every logical processor, indexed by its group
	// relative processor number.
	List      []*walk.CheckBox
	CoreIndex int
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
	var cpuArrayComView, dialogBody *walk.Composite
	var dialogScroll *walk.ScrollView
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
					// The spacers either side take whatever width is left once
					// the body has had its own, which is what keeps the content
					// centred instead of stretched across the whole window.
					Composite{
						Layout: HBox{MarginsZero: true, SpacingZero: true},
						Children: []Widget{
							HSpacer{},
							Composite{
								AssignTo: &dialogBody,
								Layout:   VBox{},
								Children: []Widget{
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
												ToolTipText:  strings.Join(DeviceDesc, "\n"),
												Text:         Bind("device.DeviceDesc == '' ? 'N/A' : device.DeviceDesc"),
											},

											Label{
												Text: "Location Info:",
											},
											Label{
												EllipsisMode: EllipsisEnd,
												ToolTipText:  strings.Join(LocationInformation, "\n"),
												Text:         Bind("device.LocationInformation == '' ? 'N/A' : device.LocationInformation"),
											},

											Label{
												Text: "DevObj Name:",
											},
											Label{
												EllipsisMode: EllipsisEnd,
												ToolTipText:  strings.Join(DevObjName, "\n"),
												Text:         Bind("device.DevObjName == '' ? 'N/A' : device.DevObjName"),
											},
											HSpacer{
												ColumnSpan: 2,
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
															// size. Grow and shrink it here instead.
															pinContentWidth(dialogBody)
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
													// Windows keeps this as one KAFFINITY, which reaches a
													// single processor group, and the registry has no value for
													// saying which. Anything past group 0 is unreachable, so say
													// so rather than quietly show a subset of the machine.
													Label{
														Visible:     cs.Skipped != 0,
														TextColor:   walk.RGB(0xA0, 0x20, 0x00),
														Text:        skippedProcessorsText(),
														ToolTipText: "Windows keeps this setting as a single 64 bit group affinity mask and offers no registry value for the group number, so only group 0 can be addressed.",
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
													if !cancel || err != nil {
														file, err := os.Create(filePath)
														if err != nil {
															return
														}
														defer file.Close()

														file.WriteString(REG_FILE_HEADER + reg_file_value.String())
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

	pinContentWidth(dialogBody)
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

	if effLen == 1 {
		return Composite{
			Layout: Grid{
				Alignment:   AlignHCenterVCenter,
				MarginsZero: true,
				Columns:     mathCeilInInt(len(coreWidgets), cs.CoreGroups[effIdx].Rows),
			},
			Children: coreWidgets,
		}
	}

	return GroupBox{
		Title: EffName(effIdx),
		Layout: Grid{
			Columns: mathCeilInInt(len(coreWidgets), cs.CoreGroups[effIdx].Rows),
		},
		Children: coreWidgets,
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
	for i := range checkboxlist.List {
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
	for i := range checkboxlist.List {
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

// CalculateMargins returns the margins of a core group box holding threads
// checkboxes on a machine whose fattest core has maxThreadsPerCore of them.
// A core with fewer threads than that gets extra padding above and below so
// that every core box comes out the same height and the row of them lines up.
// All values are 96 dpi units, walk scales them to the current dpi.
func CalculateMargins(maxThreadsPerCore, threads int) Margins {
	const margin = 9

	if threads < 1 || maxThreadsPerCore == threads {
		return Margins{
			Left:   margin,
			Top:    margin,
			Right:  margin,
			Bottom: margin,
		}
	}

	part := (11.75 * float64(maxThreadsPerCore) / float64(threads))
	return Margins{
		Left:   margin,
		Top:    int(math.Floor(part)),
		Right:  margin,
		Bottom: int(math.Ceil(part)),
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
