package main

import (
	"fmt"
	"log"
	"sort"
)

var SystemCpuSets = []SYSTEM_CPU_SET_INFORMATION{}

const (
	ToolTipTextNumaNode        = "A group-relative value indicating which NUMA node a CPU Set is on. All CPU Sets in a given group that are on the same NUMA node will have the same value for this field."
	ToolTipTextLastLevelCache  = "A group-relative value indicating which CPU Sets share at least one level of cache with each other. This value is the same for all CPU Sets in a group that are on processors that share cache with each other."
	ToolTipTextEfficiencyClass = "A value indicating the intrinsic energy efficiency of a processor for systems that support heterogeneous processors (such as ARM big.LITTLE systems). CPU Sets with higher numerical values of this field have home processors that are faster but less power-efficient than ones with lower values."
)

type CpuSets struct {
	MaxThreadsPerCore int
	// Threads is one past the highest addressable logical processor number, so
	// it is the length of every per processor slice the dialog indexes by that
	// number. It is not the number of processors when the numbering has gaps.
	Threads int
	// Groups is how many processor groups the machine has. Windows makes one
	// per 64 logical processors, so anything above one means this tool cannot
	// reach every processor. Skipped counts the ones it left out.
	Groups          int
	Skipped         int
	CPU             []CpuSet
	CoreGroups      []CoreGroups
	CoreLayout      *CoreLayout
	HyperThreading  bool
	NumaNode        bool // A group-relative value indicating which NUMA node a CPU Set is on. All CPU Sets in a given group that are on the same NUMA node will have the same value for this field.
	LastLevelCache  bool // A group-relative value indicating which CPU Sets share at least one level of cache with each other. This value is the same for all CPU Sets in a group that are on processors that share cache with each other.
	EfficiencyClass bool // A value indicating the intrinsic energy efficiency of a processor for systems that support heterogeneous processors (such as ARM big.LITTLE systems). CPU Sets with higher numerical values of this field have home processors that are faster but less power-efficient than ones with lower values.
}

type CpuSet struct {
	Id                    uint32
	CoreIndex             byte
	LogicalProcessorIndex byte
	LastLevelCacheIndex   byte // A group-relative value indicating which CPU Sets share at least one level of cache with each other. This value is the same for all CPU Sets in a group that are on processors that share cache with each other.
	EfficiencyClass       byte // A value indicating the intrinsic energy efficiency of a processor for systems that support heterogeneous processors (such as ARM big.LITTLE systems). CPU Sets with higher numerical values of this field have home processors that are faster but less power-efficient than ones with lower values.
	NumaNodeIndex         byte // A group-relative value indicating which NUMA node a CPU Set is on. All CPU Sets in a given group that are on the same NUMA node will have the same value for this field.
}

type CoreGroups struct {
	Rows int
	Cols int
}

type CoreLayout struct {
	Numa []NumaItem
}

type NumaItem struct {
	Ccd []CcdItem
}

type CcdItem struct {
	Eff EffFields
}

type EffFields struct {
	Nums map[int][][]int
}

func (item *EffFields) isNil() bool {
	if item.Nums == nil {
		return true
	}
	return false
}

func (item *CoreLayout) add(numa, ccd, effClass, core, thread int) {
	if len(item.Numa) <= numa {
		item.Numa = append(item.Numa, make([]NumaItem, numa+1-len(item.Numa))...)
	}

	if len(item.Numa[numa].Ccd) <= ccd {
		item.Numa[numa].Ccd = append(item.Numa[numa].Ccd, make([]CcdItem, ccd+1-len(item.Numa[numa].Ccd))...)
	}

	if item.Numa[numa].Ccd[ccd].Eff.Nums == nil {
		item.Numa[numa].Ccd[ccd].Eff.Nums = make(map[int][][]int)
	}
	rows := item.Numa[numa].Ccd[ccd].Eff.Nums[effClass]

	if len(rows) <= core {
		rows = append(rows, make([][]int, core+1-len(rows))...)
	}

	rows[core] = append(rows[core], thread)

	item.Numa[numa].Ccd[ccd].Eff.Nums[effClass] = rows
}

func (cs *CpuSets) Init() {
	cs.initFrom(GetCpuInformation())
}

// initFrom is Init with the processor information passed in, so the topologies
// this has to cope with can be exercised by a test.
func (cs *CpuSets) initFrom(systemCpuSets []SYSTEM_CPU_SET_INFORMATION) {
	SystemCpuSets = systemCpuSets

	cs.CoreLayout = new(CoreLayout)

	if len(SystemCpuSets) == 0 || SystemCpuSets[0].Size == 0 {
		log.Println("no processor information available")
		return
	}

	var ClassGroup = []int{}
	var lastEfficiencyClass, lastLevelCache, lastNumaNodeIndex byte
	for i := 0; i < int(uint32(len(SystemCpuSets))/SystemCpuSets[0].Size); i++ {
		cpu := SystemCpuSets[i].CpuSet()

		if int(cpu.Group)+1 > cs.Groups {
			cs.Groups = int(cpu.Group) + 1
		}

		// A 64 bit affinity mask reaches processor group 0 and nothing else, so
		// leave the rest out rather than offer a checkbox that cannot be written
		// to the registry. LogicalProcessorIndex is group relative and therefore
		// below maxProcessors on every sane machine, but it indexes CPUBits and
		// the checkbox list, so do not take that on trust.
		if cpu.Group != 0 || int(cpu.LogicalProcessorIndex) >= maxProcessors {
			cs.Skipped++
			continue
		}

		if int(cpu.LogicalProcessorIndex) >= cs.Threads {
			cs.Threads = int(cpu.LogicalProcessorIndex) + 1
		}

		cs.CPU = append(cs.CPU, CpuSet{
			Id:                    cpu.Id,
			CoreIndex:             cpu.CoreIndex,
			LogicalProcessorIndex: cpu.LogicalProcessorIndex,
			EfficiencyClass:       cpu.EfficiencyClass,
			LastLevelCacheIndex:   cpu.LastLevelCacheIndex,
			NumaNodeIndex:         cpu.NumaNodeIndex,
		})

		fmt.Printf("(%02d) [%d/%x] %02d/%02d Eff%d CCD%d NUMA%d\n", i, cpu.Id, cpu.Id, cpu.CoreIndex, cpu.LogicalProcessorIndex, cpu.EfficiencyClass, cpu.LastLevelCacheIndex, cpu.NumaNodeIndex)

		cs.CoreLayout.add(int(cpu.NumaNodeIndex), int(cpu.LastLevelCacheIndex), int(cpu.EfficiencyClass), int(cpu.CoreIndex), int(cpu.LogicalProcessorIndex))

		// CoreIndex is the processor number of the first thread on the core, so
		// the distance from it to the home processor says how far into the core
		// this thread sits, and one past the widest of those is how many threads
		// the fattest core has. Comparing the distance rather than the count
		// used to leave this at 0 on a machine without SMT, which robbed every
		// core box of its vertical margins. Both are bytes, so do not subtract
		// them as bytes in case a core ever reports out of order.
		if cpu.LogicalProcessorIndex >= cpu.CoreIndex {
			if threads := int(cpu.LogicalProcessorIndex) - int(cpu.CoreIndex) + 1; cs.MaxThreadsPerCore < threads {
				cs.MaxThreadsPerCore = threads
			}
		}

		for len(ClassGroup) <= int(cpu.EfficiencyClass) {
			ClassGroup = append(ClassGroup, 0)
		}
		ClassGroup[int(cpu.EfficiencyClass)]++

		if !cs.HyperThreading && cpu.CoreIndex != cpu.LogicalProcessorIndex {
			cs.HyperThreading = true
		}

		if !cs.EfficiencyClass && lastEfficiencyClass != cpu.EfficiencyClass {
			cs.EfficiencyClass = true
		}

		if !cs.LastLevelCache && lastLevelCache != cpu.LastLevelCacheIndex {
			cs.LastLevelCache = true
		}

		if !cs.NumaNode && lastNumaNodeIndex != cpu.NumaNodeIndex {
			cs.NumaNode = true
		}
	}

	if cs.Skipped != 0 {
		log.Printf("%d of %d logical processors are outside processor group 0 and cannot be addressed by a 64 bit affinity mask, they are not shown", cs.Skipped, cs.Skipped+len(cs.CPU))
	}

	sort.Slice(cs.CPU, func(i, j int) bool {
		if cs.CPU[i].EfficiencyClass != cs.CPU[j].EfficiencyClass {
			return cs.CPU[i].EfficiencyClass < cs.CPU[j].EfficiencyClass
		}
		if cs.CPU[i].CoreIndex != cs.CPU[j].CoreIndex {
			return cs.CPU[i].CoreIndex < cs.CPU[j].CoreIndex
		}
		return cs.CPU[i].LogicalProcessorIndex < cs.CPU[j].LogicalProcessorIndex
	})

	rows, cols := getLayout(ClassGroup...)
	for _, col := range cols {
		cs.CoreGroups = append(cs.CoreGroups, CoreGroups{Rows: rows, Cols: col})
	}
}

// otherGroupsText explains, for the dialog, why the processor list is shorter
// than the machine. It is empty on a machine of a single processor group.
//
// Nothing is being withheld here. Windows delivers a device's interrupts to
// group 0 unless the driver itself asks for another group, so group 0 is where
// the interrupts of an ordinary device are, and the mask in the registry
// addresses exactly that group.
func otherGroupsText() string {
	if cs.Skipped == 0 {
		return ""
	}

	return fmt.Sprintf(
		"Windows delivers device interrupts to processor group 0 unless the driver asks for another group, and this setting is a mask over that one group. This machine has %d groups, so its other %d processors are not listed.",
		cs.Groups, cs.Skipped)
}
