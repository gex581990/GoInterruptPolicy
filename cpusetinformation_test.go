package main

import "testing"

// fakeCpuSets builds processor information the way the Windows API hands it
// over: the entry count is len(slice) / Size, not len(slice).
func fakeCpuSets(count int, fill func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet)) []SYSTEM_CPU_SET_INFORMATION {
	const size = 32

	set := make([]SYSTEM_CPU_SET_INFORMATION, count*size)
	set[0].Size = size
	for i := range count {
		fill(i, set[i].CpuSet())
	}

	return set[:count*size]
}

// A machine with more than one processor group repeats the group relative
// processor numbers in every group. Those numbers index CPUBits and the
// checkbox list, so keeping the extra groups would both collide and run off
// the end of a 64 bit affinity mask.
func TestInitKeepsProcessorGroupZeroOnly(t *testing.T) {
	const groups, perGroup = 2, 64

	var cs CpuSets
	cs.initFrom(fakeCpuSets(groups*perGroup, func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet) {
		c.Id = uint32(0x100 + i)
		c.Group = uint16(i / perGroup)
		c.LogicalProcessorIndex = byte(i % perGroup)
		c.CoreIndex = byte(i % perGroup / 2 * 2)
	}))

	if len(cs.CPU) != perGroup {
		t.Errorf("kept %d processors, want the %d of group 0", len(cs.CPU), perGroup)
	}
	if cs.Threads != perGroup {
		t.Errorf("cs.Threads = %d, want %d", cs.Threads, perGroup)
	}
}

// Whatever the topology, nothing the dialog indexes by processor number may
// run past CPUBits or past the checkbox list, which is cs.Threads long.
func TestInitStaysInsideTheAffinityMask(t *testing.T) {
	tests := []struct {
		name  string
		count int
		fill  func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet)
	}{
		{
			name:  "16 threads on 8 cores",
			count: 16,
			fill: func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet) {
				c.LogicalProcessorIndex = byte(i)
				c.CoreIndex = byte(i / 2 * 2)
			},
		},
		{
			name:  "exactly as many processors as the mask has bits",
			count: maxProcessors,
			fill: func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet) {
				c.LogicalProcessorIndex = byte(i)
				c.CoreIndex = byte(i / 2 * 2)
			},
		},
		{
			name:  "a processor number past the mask is dropped",
			count: 8,
			fill: func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet) {
				c.LogicalProcessorIndex = byte(i * 16) // 0, 16, ... 112
				c.CoreIndex = c.LogicalProcessorIndex
			},
		},
		{
			name:  "two nodes of a hybrid part",
			count: 32,
			fill: func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet) {
				c.LogicalProcessorIndex = byte(i)
				c.CoreIndex = byte(i / 2 * 2)
				c.EfficiencyClass = byte(i / 16)
				c.NumaNodeIndex = byte(i / 16)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cs CpuSets
			cs.initFrom(fakeCpuSets(tt.count, tt.fill))

			if cs.Threads > maxProcessors {
				t.Fatalf("cs.Threads = %d, want at most %d", cs.Threads, maxProcessors)
			}

			for _, c := range cs.CPU {
				if int(c.LogicalProcessorIndex) >= len(CPUBits) {
					t.Errorf("processor %d runs past CPUBits (%d)", c.LogicalProcessorIndex, len(CPUBits))
				}
				if int(c.LogicalProcessorIndex) >= cs.Threads {
					t.Errorf("processor %d runs past the checkbox list (%d)", c.LogicalProcessorIndex, cs.Threads)
				}
			}

			// The preset buttons walk the core layout rather than cs.CPU, so it
			// has to hold up to the same bound.
			for _, numa := range cs.CoreLayout.Numa {
				for _, ccd := range numa.Ccd {
					for _, cores := range ccd.Eff.Nums {
						for _, threads := range cores {
							for _, thread := range threads {
								if thread >= len(CPUBits) || thread >= cs.Threads {
									t.Errorf("layout thread %d runs past CPUBits (%d) or the checkbox list (%d)", thread, len(CPUBits), cs.Threads)
								}
							}
						}
					}
				}
			}
		})
	}
}

func TestInitSurvivesEmptyProcessorInformation(t *testing.T) {
	var cs CpuSets
	cs.initFrom(nil)

	if cs.CoreLayout == nil {
		t.Fatal("CoreLayout is nil, the dialog would panic walking it")
	}
	if len(cs.CPU) != 0 || cs.Threads != 0 {
		t.Errorf("got %d processors and Threads %d, want none", len(cs.CPU), cs.Threads)
	}
}
