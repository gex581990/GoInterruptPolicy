//go:generate goversioninfo -64

package main

import (
	"fmt"
	"log"
	"os"
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
		if flagCPU != "" {
			for val := range strings.SplitSeq(flagCPU, ",") {
				i, err := strconv.Atoi(val)
				if err != nil {
					log.Println(err)
					continue
				}
				if i < 0 || i >= len(CPUBits) {
					log.Printf("-cpu %d is outside the %d processors an affinity mask can address", i, len(CPUBits))
					continue
				}
				assignmentSetOverride = Set(assignmentSetOverride, CPUBits[i])
			}
		}

		if flagMsiSupported != -1 || flagMessageNumberLimit != -1 {
			if flagMsiSupported != -1 {
				newItem.MsiSupported = uint32(flagMsiSupported)
			}
			if flagMessageNumberLimit != -1 {
				newItem.MessageNumberLimit = uint32(flagMessageNumberLimit)
			}
			orgItem.setMSIMode(newItem)
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
			orgItem.setAffinityPolicy(newItem)
		}

		changed := orgItem.MsiSupported != newItem.MsiSupported || orgItem.MessageNumberLimit != newItem.MessageNumberLimit || orgItem.DevicePolicy != newItem.DevicePolicy || orgItem.DevicePriority != newItem.DevicePriority || orgItem.AssignmentSetOverride != newItem.AssignmentSetOverride
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

			if DeviceInstallParams.Flags&DI_NEEDREBOOT != 0 {
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
								for _, selectionIndex := range SelectedIndexes {
									mw.tv.Model().(*Model).items[selectionIndex] = mw.SetNewDevice(&mw.tv.Model().(*Model).items[selectionIndex], &d)
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
							index := mw.tv.CurrentIndex()
							d := mw.tv.Model().(*Model).items[index]
							OpenRegistry(mw.Form(), d.reg)
						},
					},
				},
				Model:    mw.model,
				AssignTo: &mw.tv,
				OnKeyUp: func(key walk.Key) {
					i := mw.tv.CurrentIndex()
					if i == -1 {
						i = 0
					}
					for ; i < len(mw.model.items); i++ {
						item := &mw.model.items[i]
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
							return mw.model.items[i].AssignmentSetOverride < mw.model.items[j].AssignmentSetOverride
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
							return mw.model.items[i].InterruptTypeMap < mw.model.items[j].InterruptTypeMap
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
	if maxDeviceDesc < 150 {
		mw.tv.Columns().At(0).SetWidth(maxDeviceDesc)
	}

	mw.Show()
	mw.tv.SetFocus()
	mw.Run()
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
	mw.tv.Model().(*Model).items[currentIndex] = mw.SetNewDevice(&mw.tv.Model().(*Model).items[currentIndex], &d)

	mw.tv.UpdateItem(currentIndex)
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

			if DeviceInstallParams.Flags&DI_NEEDREBOOT != 0 {
				walk.MsgBox(mw.WindowBase.Form(), "Notice", "Device could not be restarted. Changes will take effect the next time you reboot.", walk.MsgBoxIconWarning)
			} else {
				walk.MsgBox(mw.WindowBase.Form(), "Notice", "Device successfully restarted.", walk.MsgBoxIconInformation)
			}

		}
	}
	return *orgItem
}

func (mw *MyMainWindow) TextWidthSize(text string) int {
	canvas, err := (*mw.tv).CreateCanvas()
	if err != nil {
		return 0
	}
	defer canvas.Dispose()

	bounds, _, err := canvas.MeasureTextPixels(text, (*mw.tv).Font(), walk.Rectangle{Width: 9999999}, walk.TextCalcRect)
	if err != nil {
		return 0
	}

	return bounds.Size().Width
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
