//go:build debug && Intel && FakeHybrid2LLC

package main

import "fmt"

// A made up hybrid processor whose efficiency classes do not all share one
// cache: 6 P-cores with hyperthreading and 8 E-cores on one last level cache,
// and 2 more E-cores on a cache of their own, roughly how Meteor Lake keeps
// its low power E-cores apart from the rest. It is for looking at a cache
// group that holds more than one efficiency class next to another cache
// group, which none of the real processors here has. What Windows reports for
// Meteor Lake itself has not been checked.
func GetCpuInformation() []SYSTEM_CPU_SET_INFORMATION {
	fmt.Println("FakeHybrid2LLC")
	size := 0x20 * 64
	cpuSet := make([]SYSTEM_CPU_SET_INFORMATION, size)
	cpuSet[0].Size = 32
	var lastCoreIndex byte
	var count = 704
	var index = 0x100
	for i := 0; i < count; i++ {
		cs := cpuSet[i].CpuSet()
		cs.Id = uint32(index + i)
		cs.LogicalProcessorIndex = byte(i)
		if i < 12 && i%2 != 0 {
			cs.CoreIndex = lastCoreIndex
		} else {
			cs.CoreIndex = byte(i)
			lastCoreIndex = byte(i)
		}
		if i < 12 {
			cs.EfficiencyClass = 1
		}
		if i >= 20 {
			cs.LastLevelCacheIndex = 20
		}
	}
	return cpuSet[:count]
}
