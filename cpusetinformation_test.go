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

// A machine without SMT has one thread per core, not zero. Reporting zero left
// every core group box without vertical margins.
func TestInitCountsThreadsPerCore(t *testing.T) {
	tests := []struct {
		name  string
		count int
		fill  func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet)
		want  int
	}{
		{
			name:  "no SMT",
			count: 8,
			fill: func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet) {
				c.LogicalProcessorIndex = byte(i)
				c.CoreIndex = byte(i)
			},
			want: 1,
		},
		{
			name:  "two threads per core",
			count: 16,
			fill: func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet) {
				c.LogicalProcessorIndex = byte(i)
				c.CoreIndex = byte(i / 2 * 2)
			},
			want: 2,
		},
		{
			name:  "hybrid, SMT on the performance cores only",
			count: 24,
			fill: func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet) {
				c.LogicalProcessorIndex = byte(i)
				if i < 16 { // eight cores with two threads each
					c.CoreIndex = byte(i / 2 * 2)
					c.EfficiencyClass = 1
				} else { // eight cores with one thread each
					c.CoreIndex = byte(i)
				}
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cs CpuSets
			cs.initFrom(fakeCpuSets(tt.count, tt.fill))

			if cs.MaxThreadsPerCore != tt.want {
				t.Errorf("MaxThreadsPerCore = %d, want %d", cs.MaxThreadsPerCore, tt.want)
			}
		})
	}
}

func TestCalculateMargins(t *testing.T) {
	// Every core has the same number of threads, so no core box needs padding.
	for _, threads := range []int{1, 2, 4} {
		got := CalculateMargins(threads, threads)
		if got.Top != 9 || got.Bottom != 9 || got.Left != 9 || got.Right != 9 {
			t.Errorf("CalculateMargins(%d, %d) = %+v, want a plain 9 all round", threads, threads, got)
		}
	}

	// A one thread core next to two thread cores is padded to match their
	// height, so the extra has to land above and below rather than to the side.
	padded := CalculateMargins(2, 1)
	plain := CalculateMargins(2, 2)
	if padded.Top+padded.Bottom <= plain.Top+plain.Bottom {
		t.Errorf("a 1 thread core box got %d of vertical margin, a 2 thread one %d, want more for the smaller core",
			padded.Top+padded.Bottom, plain.Top+plain.Bottom)
	}
	if padded.Left != 9 || padded.Right != 9 {
		t.Errorf("CalculateMargins(2, 1) = %+v, want the side margins left alone", padded)
	}

	// Degenerate input must not divide by zero into a nonsense layout.
	if got := CalculateMargins(2, 0); got.Top != 9 || got.Bottom != 9 {
		t.Errorf("CalculateMargins(2, 0) = %+v, want a plain 9 all round", got)
	}
}
