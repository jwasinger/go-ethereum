// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

type VerkleStem [31]byte

type ChunkValue struct {
	mode byte
	value []byte
}

// AccessWitness lists the locations of the state that are being accessed
// during the production of a block.
// TODO(@gballet) this doesn't fully support deletions
type AccessWitness struct {
	// Branches flags if a given branch has been loaded
	// for the byte value:
	//	the first bit is set if the branch has been edited
	//	the second bit is set if the branch has been read
	Branches map[VerkleStem]byte

	// Chunks contains the initial value of each address
	Chunks map[common.Hash]ChunkValue
}

func NewAccessWitness() *AccessWitness {
	return &AccessWitness{
		Branches:  make(map[VerkleStem]struct{}),
		Chunks:    make(map[common.Hash]ChunkValue),
	}
}

const (
	AccessWitnessReadFlag = 1
	AccessWitnessWriteFlag = 2
)

// because of the way Geth's EVM is implemented, the gas cost of an operation
// may be needed before the value of the leaf-key can be retrieved. Hence, we
// break witness access (for the purpose of gas accounting), and filling witness
// values into two methods
func (aw *AccessWitness) SetLeafValue(addr, value []byte) {
	if chunk, exists := aw.Chunks[addr]; exists {
		chunk.value = value
	} else {
		panic(fmt.Sprintf("address not in access witness: %x", addr))
	}
}

func (aw *AccessWitness) touchAddressOnWrite(addr, value []byte) (bool, bool, bool) {
	var stem        VerkleStem
	var stemWrite, chunkWrite, chunkFill bool
	copy(stem[:], addr[:31])

	// NOTE: stem, selector access flags already exist in their
	// respective maps because this function is called at the end of 
	// processing a read access event

	if !(aw.Branches[stem] & AccessWitnessWriteFlag) {
		stemWrite = true
		aw.Branches[stem] |= AccessWitnessWriteFlag
	}

	if aw.Chunks[common.BytesToHash(addr)] & AccessWitnessWriteFlag {
		chunkWrite = true
		aw.Chunks[common.BytesToHash(addr)].mode |= AccessWitnessWriteFlag
	}

	// TODO charge chunk filling costs if the leaf was previously empty in the state
	/*
	if chunkWrite {
		if _, err := verkleDb.TryGet(addr); err != nil {
			chunkFill = true
		}
	}
	*/

	return stemWrite, chunkWrite, chunkFill
}

// TouchAddress adds any missing addr to the witness and returns respectively
// true if the stem or the stub weren't arleady present.
func (aw *AccessWitness) touchAddress(addr []byte, isWrite bool) (bool, bool, bool, bool, bool) {
	var (
		stem        [31]byte
		stemRead bool
		selectorRead bool
	)
	copy(stem[:], addr[:31])

	// Check for the presence of the stem
	if _, hasStem := aw.Branches[stem]; !hasStem {
		stemRead = true
		aw.Branches[stem] = AccessWitnessReadFlag
	}

	// always charge read cost whether the access event is read/write
	// literal interpretation of the spec
	selectorRead = true

	// Check for the presence of the leaf selector
	if _, hasSelector := aw.Chunks[common.BytesToHash(addr)]; !hasSelector {
		aw.Chunks[common.BytesToHash(addr)] = ChunkValue{
			AccessWitnessReadFlag,
			nil,
		}
	}

	var stemWrite, chunkWrite, chunkFill bool

	if isWrite {
		stemWrite, selectorWrite, chunkFill := aw.touchAddressOnWrite(addr, value)
	}

	return stemRead, selectorRead, stemWrite, selectorWrite, chunkFill
}

func (aw *AccessWitness) touchAddressAndChargeGas(addr, value []byte, isWrite bool) uint64 {
	var gas uint64

	stemRead, selectorRead, stemWrite, selectorWrite, selectorFill := aw.TouchAddress(addr, value, isWrite)
	if stemRead {
		gas += params.WitnessBranchReadCost
	}
	if selectorRead {
		gas += params.WitnessChunkReadCost
	}
	if stemWrite {
		gas += params.WitnessBranchWriteCost
	}
	if selectorWrite {
		gas += params.WitnessChunkWriteCost
	}
	if selectorFill {
		gas += params.WitnessChunkFillCost
	}

	return gas
}

func (aw *AccessWitness) TouchAddressOnWriteAndChargeGas(addr []byte) uint64 {
	touchAddressAndChargeGas(addr, true)
}

func (aw *AccessWitness) TouchAddressOnReadAndChargeGas(addr []byte) uint64 {
	touchAddressAndChargeGas(addr, false)
}

// Merge is used to merge the witness that got generated during the execution
// of a tx, with the accumulation of witnesses that were generated during the
// execution of all the txs preceding this one in a given block.
func (aw *AccessWitness) Merge(other *AccessWitness) {
	for k := range other.Undefined {
		if _, ok := aw.Undefined[k]; !ok {
			aw.Undefined[k] = struct{}{}
		}
	}

	for k := range other.Branches {
		if _, ok := aw.Branches[k]; !ok {
			aw.Branches[k] = other.Branches[k]
		}
	}

	for k, chunk := range other.Chunks {
		if _, ok := aw.Chunks[k]; !ok {
			aw.Chunks[k] = chunk
		}
	}

	for k, leafAccessFlags := range other.LeafAccesses {
		if _, ok := aw.LeafAccesses[k]; !ok {
			aw.LeafAccesses[k] = leafAccessFlags
		}
	}
}

// Key returns, predictably, the list of keys that were touched during the
// buildup of the access witness.
func (aw *AccessWitness) Keys() [][]byte {
	keys := make([][]byte, 0, len(aw.Chunks))
	for key := range aw.Chunks {
		var k [32]byte
		copy(k[:], key[:])
		keys = append(keys, k[:])
	}
	return keys
}

func (aw *AccessWitness) KeyVals() map[common.Hash][]byte {
	return aw.Chunks
}

func (aw *AccessWitness) Copy() *AccessWitness {
	naw := &AccessWitness{
		Branches:  make(map[[31]byte]struct{}),
		Chunks:    make(map[common.Hash][]byte),
		Undefined: make(map[common.Hash]struct{}),
	}

	naw.Merge(aw)

	return naw
}
