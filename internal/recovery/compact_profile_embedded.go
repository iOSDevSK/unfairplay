package recovery

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
)

//go:embed profiles/macos26a428-compact-cfg.bin.gz
var compact26A428ProfileCompressed []byte

var (
	compact26A428AccountEntryBlocks map[uint64]compactBlock
	compact26A428BootstrapBlocks    map[uint64]compactBlock
	compact26A428SelectionBlocks    map[uint64]compactBlock
	compact26A428SINFEntryBlocks00  map[uint64]compactBlock
	compact26A428SINFEntryBlocks01  map[uint64]compactBlock
	compact26A428SINFEntryBlocks02  map[uint64]compactBlock
	compact26A428SINFEntryBlocks03  map[uint64]compactBlock
	compact26A428SINFEntryBlocks04  map[uint64]compactBlock
	compact26A428SINFEntryBlocks05  map[uint64]compactBlock
	compact26A428SupplementalBlocks map[uint64]compactBlock
)

func init() {
	stream, err := gzip.NewReader(bytes.NewReader(compact26A428ProfileCompressed))
	if err != nil {
		panic(fmt.Sprintf("decompress embedded FairPlay CFG profile: %v", err))
	}
	encoded, err := io.ReadAll(stream)
	closeErr := stream.Close()
	if err != nil || closeErr != nil {
		panic(fmt.Sprintf("decompress embedded FairPlay CFG profile: %v", err))
	}
	defer clear(encoded)
	sections, err := unmarshalCompactProfile(encoded)
	if err != nil {
		panic(fmt.Sprintf("load embedded FairPlay CFG profile: %v", err))
	}
	expected := map[uint8]int{1: 1086, 2: 953, 3: 1656, 4: 315, 5: 315, 6: 315, 7: 315, 8: 315, 9: 315, 10: 58318}
	if len(sections) != len(expected) {
		panic("embedded FairPlay CFG profile has an invalid section count")
	}
	for id, count := range expected {
		if len(sections[id]) != count {
			panic(fmt.Sprintf("embedded FairPlay CFG section %d has %d blocks, want %d", id, len(sections[id]), count))
		}
	}
	compact26A428AccountEntryBlocks = sections[1]
	compact26A428BootstrapBlocks = sections[2]
	compact26A428SelectionBlocks = sections[3]
	compact26A428SINFEntryBlocks00 = sections[4]
	compact26A428SINFEntryBlocks01 = sections[5]
	compact26A428SINFEntryBlocks02 = sections[6]
	compact26A428SINFEntryBlocks03 = sections[7]
	compact26A428SINFEntryBlocks04 = sections[8]
	compact26A428SINFEntryBlocks05 = sections[9]
	compact26A428SupplementalBlocks = sections[10]
}
