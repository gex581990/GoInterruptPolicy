//go:build !debug

package main

import (
	"log"

	"golang.org/x/sys/windows"
)

func GetCpuInformation() []SYSTEM_CPU_SET_INFORMATION {
	var SystemCpuSets = make([]SYSTEM_CPU_SET_INFORMATION, 1)
	var length uint32
	var hProcess windows.Handle

	GetSystemCpuSetInformation(&SystemCpuSets[0], 1, &length, uintptr(hProcess), 0)
	if length == 0 {
		log.Println("GetSystemCpuSetInformation reported no processors")
		return nil
	}

	SystemCpuSets = make([]SYSTEM_CPU_SET_INFORMATION, length)
	if GetSystemCpuSetInformation(&SystemCpuSets[0], uint32(length), &length, uintptr(hProcess), 0) != 1 {
		log.Println("err")
	}
	return SystemCpuSets
}
