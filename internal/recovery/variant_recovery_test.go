package recovery

import (
	"sync"
	"testing"
)

func TestLegacyCompactInstructions(t *testing.T) {
	t.Run("sub-lsr-32", func(t *testing.T) {
		s := newState(&model{pages: make(map[uint64][]byte)})
		s.x[0], s.x[1] = 0x80000000, 0x80000000
		s.c = true
		err := runCompactKernel(s, compactKernel{instructions: []compactInstruction{{
			op: compactSubLSR32, destination: 2, left: 0, right: 1, shift: 31,
		}}})
		if err != nil || s.x[2] != 0x7fffffff || !s.c {
			t.Fatalf("result=%#x flags.c=%v err=%v", s.x[2], s.c, err)
		}
	})
	t.Run("logical-mask-64", func(t *testing.T) {
		s := newState(&model{pages: make(map[uint64][]byte)})
		s.x[0] = 0x123456789abcdef0
		err := runCompactKernel(s, compactKernel{instructions: []compactInstruction{{
			op: compactLogicalMask64, destination: 0, left: 0, shift: 8, immediate: 2,
		}}})
		if err != nil || s.x[0] != 0x0200020002000200 {
			t.Fatalf("result=%#x err=%v", s.x[0], err)
		}
	})
	t.Run("test-register-64", func(t *testing.T) {
		s := newState(&model{pages: make(map[uint64][]byte)})
		s.x[0], s.x[1], s.c, s.v = 0x8000000000000000, ^uint64(0), true, true
		if err := runCompactKernel(s, compactKernel{instructions: []compactInstruction{{op: compactTestRegister64, left: 0, right: 1}}}); err != nil {
			t.Fatal(err)
		}
		if !s.n || s.z || s.c || s.v {
			t.Fatalf("wrong flags: n=%v z=%v c=%v v=%v", s.n, s.z, s.c, s.v)
		}
	})
	t.Run("multiply-negate-32", func(t *testing.T) {
		s := newState(&model{pages: make(map[uint64][]byte)})
		s.x[0], s.x[1] = 3, 5
		if err := runCompactKernel(s, compactKernel{instructions: []compactInstruction{{op: compactMNeg32, destination: 2, left: 0, right: 1}}}); err != nil {
			t.Fatal(err)
		}
		if s.x[2] != uint64(^uint32(14)) {
			t.Fatalf("result=%#x", s.x[2])
		}
	})
	t.Run("rotate-right-64", func(t *testing.T) {
		s := newState(&model{pages: make(map[uint64][]byte)})
		s.x[0] = 0x1122334455667788
		if err := runCompactKernel(s, compactKernel{instructions: []compactInstruction{{op: compactRORImm64, destination: 1, left: 0, shift: 8}}}); err != nil {
			t.Fatal(err)
		}
		if s.x[1] != 0x8811223344556677 {
			t.Fatalf("result=%#x", s.x[1])
		}
	})
	t.Run("xor-rotate-right-64", func(t *testing.T) {
		s := newState(&model{pages: make(map[uint64][]byte)})
		s.x[0], s.x[1] = 0x0102030405060708, 0x1122334455667788
		if err := runCompactKernel(s, compactKernel{instructions: []compactInstruction{{op: compactXorROR64, destination: 2, left: 0, right: 1, shift: 8}}}); err != nil {
			t.Fatal(err)
		}
		if s.x[2] != 0x891321374153617f {
			t.Fatalf("result=%#x", s.x[2])
		}
	})
	t.Run("subtract-shift-right-64", func(t *testing.T) {
		s := newState(&model{pages: make(map[uint64][]byte)})
		s.x[0], s.x[1] = 0x1000, 0x800
		if err := runCompactKernel(s, compactKernel{instructions: []compactInstruction{{op: compactSubLSR64, destination: 2, left: 0, right: 1, shift: 4}}}); err != nil {
			t.Fatal(err)
		}
		if s.x[2] != 0xf80 {
			t.Fatalf("result=%#x", s.x[2])
		}
	})
}

func TestRecoveryWorkersKeepMappingsPrivate(t *testing.T) {
	const base = uint64(0x710000014000)
	input := &model{allowDestinationPages: true, pages: map[uint64][]byte{base: make([]byte, pageSize)}}
	one, two := newState(input), newState(input)
	initial := one.digest()
	if err := one.storeValue(base-1, 0x42, 1); err != nil {
		t.Fatal(err)
	}
	if err := one.mapZero(0x710100000000, pageSize); err != nil {
		t.Fatal(err)
	}
	if err := one.storeValue(0x710100000000, 0x42, 1); err != nil {
		t.Fatal(err)
	}
	if len(input.pages) != 1 || two.digest() != initial {
		t.Fatal("worker changed shared recovery input")
	}
	if _, err := two.load(base-1, 1); err == nil {
		t.Fatal("worker inherited another worker's destination page")
	}
	one.reset()
	if one.digest() != initial {
		t.Fatal("reset did not restore the captured state")
	}

	var group sync.WaitGroup
	for _, worker := range []*state{one, two} {
		group.Add(1)
		go func(worker *state) {
			defer group.Done()
			for index := 0; index < 100; index++ {
				_, _, err := runCompactBlock(worker, nil, compactBlock{
					start: 0x1000, instructions: []compactInstruction{{op: compactAddImm64, destination: 0, left: 0, immediate: 1}},
					terminator: compactBlockReturn,
				})
				if err != nil {
					t.Error(err)
				}
			}
		}(worker)
	}
	group.Wait()
}

func TestDestinationPagesRequireWrites(t *testing.T) {
	const base = uint64(0x710000014000)
	s := newState(&model{allowDestinationPages: true, pages: map[uint64][]byte{base: make([]byte, pageSize)}})
	if _, err := s.load(base-1, 1); err == nil {
		t.Fatal("read uncaptured memory")
	}
	if err := s.storeValue(base-1, 0x42, 1); err != nil {
		t.Fatal(err)
	}
	if got, err := s.load(base-1, 1); err != nil || got != 0x42 {
		t.Fatalf("written byte=%#x err=%v", got, err)
	}
	if _, err := s.load(base-2, 1); err == nil {
		t.Fatal("invented an unwritten byte")
	}
	if err := s.storeValue(base-2*pageSize, 0, 1); err == nil {
		t.Fatal("accepted a distant missing page")
	}
}

func TestSupplementalProfileContainsIssueTwoSplit(t *testing.T) {
	for _, address := range []uint64{
		0x100105c7c, 0x10010ce1c, 0x10011d288,
		0x1001511f0, 0x100151304, 0x100193064,
	} {
		if _, ok := compact26A428SupplementalBlocks[address]; !ok {
			t.Fatalf("missing supplemental block %#x", address)
		}
	}
	if block := compact26A428SupplementalBlocks[0x1001511f0]; block.terminator != compactBlockCallRegister || block.register != 8 {
		t.Fatalf("indirect call block was not split correctly: %+v", block)
	}
	if block := compact26A428SupplementalBlocks[0x100151304]; block.terminator != compactBlockRegister || block.register != 10 {
		t.Fatalf("call continuation is incorrect: %+v", block)
	}
}

func TestSelectionBlockLimit(t *testing.T) {
	for _, test := range []struct {
		requested, want int
		bad             bool
	}{
		{0, 2_000_000, false}, {1, 1, false}, {20_000_000, 20_000_000, false},
		{-1, 0, true}, {20_000_001, 0, true},
	} {
		got, err := selectionBlockLimit(test.requested)
		if (err != nil) != test.bad || !test.bad && got != test.want {
			t.Fatalf("limit %d => %d, %v", test.requested, got, err)
		}
	}
}

func TestEmbeddedExecutionProfileGeometry(t *testing.T) {
	profile := embedded26A428ExecutionProfile()
	if profile.name != "macos26a428" || profile.binaryHash != rawSourceBinaryHash ||
		profile.accountEntryStart != 0x100cd1c18 || profile.accountEntryStop != 0x10057ae74 ||
		profile.bootstrapStart != 0x10057ae74 || profile.bootstrapStop != 0x1005b7438 ||
		profile.selectionStart != 0x1006b10a0 || profile.selectionStop != 0x10008a644 || profile.selectionStopHits != 2 ||
		profile.sinfEntryStart != 0x10008a644 || profile.sinfEntryStop != 0x100234cdc ||
		profile.selectionFrameCall != 0x1002fbb24 || len(profile.onceTokens) != 4 {
		t.Fatalf("unexpected embedded execution profile: %+v", profile)
	}
	if len(profile.accountEntryBlocks) != 1086 || len(profile.bootstrapBlocks) != 953 || len(profile.selectionBlocks) != 1656 || len(profile.supplementalBlocks) != 58318 {
		t.Fatal("embedded execution profile has incomplete CFG maps")
	}
	model := &model{binaryHash: rawSourceBinaryHash}
	selected, err := executionProfileForModel(model)
	if err != nil || selected.binaryHash != rawSourceBinaryHash {
		t.Fatalf("select embedded execution profile: %v", err)
	}
	bad := *selected
	bad.binaryHash[0] ^= 1
	model.profile = &bad
	if _, err := executionProfileForModel(model); err == nil {
		t.Fatal("accepted an execution profile for another binary")
	}
}
