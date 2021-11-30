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

// AccessWitness lists the locations of the state that are being accessed
// during the production of a block.
// TODO(@gballet) this doesn't fully support deletions
type AccessWitness struct {
	// Branches flags if a given branch has been loaded
	// for the byte value:
	//	the first bit is set if the branch has been edited
	//	the second bit is set if the branch has been read
	Branches map[[31]byte]byte

	LeavesAccesses map[common.Hash]byte

	// Chunks contains the initial value of each address
	Chunks map[common.Hash][]byte

	// The initial value isn't always available at the time an
	// address is touched, this map references addresses that
	// were touched but can not yet be put in Chunks.
	Undefined map[common.Hash]struct{}
}


func NewAccessWitness() *AccessWitness {
	return &AccessWitness{
		Branches:  make(map[[31]byte]struct{}),
		Chunks:    make(map[common.Hash][]byte),
		Undefined: make(map[common.Hash]struct{}),
	}
}

const (
	AccessWitnessReadFlag = 1
	AccessWitnessWriteFlag = 2
)

func (aw *AccessWitness) touchAddressOnWrite(addr, value []byte) (bool, bool) {
	var stem        [31]byte
	copy(stem[:], addr[:31])

	var stemWrite, chunkWrite, chunkFill bool

	// NOTE: stem, selector access flags already exist in their
	// respective maps because this function is called at the end of 
	// processing a read access event

	if !(aw.Branches[stem] & AccessWitnessWriteFlag) {
		stemWrite = true
		aw.Branches[stem] |= AccessWitnessWriteFlag
	}

	if aw.LeafAccesses[common.BytesToHash(addr)] & AccessWitnessWriteFlag {
		chunkWrite = true
		aw.LeafAccesses[common.BytesToHash(addr)] |= AccessWitnessWriteFlag
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
func (aw *AccessWitness) TouchAddress(addr, value []byte, isWrite bool) (bool, bool, bool, bool, bool) {
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

	// Check for the presence of the selector
	if _, hasSelector := aw.Chunks[common.BytesToHash(addr)]; !hasSelector {
		if value == nil {
			aw.Undefined[common.BytesToHash(addr)] = struct{}{}
		} else {
			if _, ok := aw.Undefined[common.BytesToHash(addr)]; !ok {
				delete(aw.Undefined, common.BytesToHash(addr))
			}
			aw.Chunks[common.BytesToHash(addr)] = value
		}

		if accessFlags, hasAccessFlags = :aw.LeafAccesses; hasAccessFlags {
			if !(aw.LeafAccesses[common.BytesToHash(addr)] & AccessWitnessRead)
				aw.LeafAccesses[common.BytesToHash(addr)] |= AccessWitnessRead
			}
		} else {
			aw.LeafAccesses[common.BytesToHash(addr)] = AccessWitnessRead

		}
	}

	var stemWrite, chunkWrite, chunkFill bool

	if isWrite {
		stemWrite, selectorWrite, chunkFill := aw.touchAddressOnWrite(addr, value)
	}

	return stemRead, selectorRead, stemWrite, selectorWrite, chunkFill
}

// TouchAddressAndChargeGas checks if a location has already been touched in
// the current witness, and charge extra gas if that isn't the case. This is
// meant to only be called on a tx-context access witness (i.e. before it is
// merged), not a block-context witness: witness costs are charged per tx.
func (aw *AccessWitness) TouchAddressAndChargeGas(addr, value []byte, isWrite bool) uint64 {
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
