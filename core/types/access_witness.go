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

type VerkleTrie interface {
    TryGet(key []byte) ([]byte, error)
}

// AccessWitness lists the locations of the state that are being accessed
// during the production of a block.
// TODO(@gballet) this doesn't fully support deletions
type AccessWitness struct {
	// Branches flags if a given branch has been loaded
	ReadBranches map[[31]byte]struct{}

	// Chunks contains the initial value of each address
	ReadChunks map[common.Hash][]byte

    WriteBranches map[[31]byte]struct{}
    WriteChunks map[common.Hash][]byte

	// The initial value isn't always available at the time an
	// address is touched, this map references addresses that
	// were touched but can not yet be put in Chunks.
	Undefined map[common.Hash]struct{}
}

func NewAccessWitness() *AccessWitness {
	return &AccessWitness{
		ReadBranches:  make(map[[31]byte]struct{}),
		ReadChunks:    make(map[common.Hash][]byte),
		WriteBranches:  make(map[[31]byte]struct{}),
		WriteChunks:    make(map[common.Hash][]byte),
		Undefined: make(map[common.Hash]struct{}),
	}
}

func (aw *AccessWitness) TouchAddressOnRead(addr, value []byte) (bool, bool) {
    return aw.touchAddress(addr, value, false)
}

func (aw *AccessWitness) TouchAddressOnWrite(addr, value []byte) (bool, bool) {
    return aw.touchAddress(addr, value, true)
}

// TouchAddress adds any missing addr to the witness and returns respectively
// true if the stem or the stub weren't arleady present.
func (aw *AccessWitness) touchAddress(addr, value []byte, isWrite bool) (bool, bool) {
    var (
        stem        [31]byte
        newStem     bool
        newSelector bool
    )

	var branches map[[31]byte]struct{}
    var chunks map[common.Hash][]byte

    if isWrite {
        branches = aw.WriteBranches
        chunks = aw.WriteChunks
    } else {
        branches = aw.ReadBranches
        chunks = aw.ReadChunks
    }

    copy(stem[:], addr[:31])

    // Check for the presence of the stem
    if _, newStem := branches[stem]; !newStem {
        branches[stem] = struct{}{}
    }

    // Check for the presence of the selector
    if _, newSelector := chunks[common.BytesToHash(addr)]; !newSelector {
        if value == nil {
            aw.Undefined[common.BytesToHash(addr)] = struct{}{}
        } else {
            if _, ok := aw.Undefined[common.BytesToHash(addr)]; !ok {
                delete(aw.Undefined, common.BytesToHash(addr))
            }
            chunks[common.BytesToHash(addr)] = value
        }
    }

    return newStem, newSelector
}


// TouchAddressAndChargeGas checks if a location has already been touched in
// the current witness, and charge extra gas if that isn't the case. This is
// meant to only be called on a tx-context access witness (i.e. before it is
// merged), not a block-context witness: witness costs are charged per tx.
func (aw *AccessWitness) TouchAddressOnReadAndChargeGas(addr, value []byte) uint64 {
	var gas uint64

	nstem, nsel := aw.TouchAddressOnRead(addr, value)
	if nstem {
		gas += params.WitnessBranchReadCost
	}
	if nsel {
		gas += params.WitnessChunkReadCost
	}
	return gas
}

// TODO provide an interface to lookup if the chunk was nil and apply CHUNK_EDIT_COST in here?
func (aw *AccessWitness) TouchAddressOnWriteAndChargeGas(vtr VerkleTrie, addr, value []byte) uint64 {
	var gas uint64

	nstem, nsel := aw.TouchAddressOnWrite(addr, value)
	if nstem {
		gas += params.WitnessBranchWriteCost
	}

	if nsel {
		gas += params.WitnessChunkWriteCost
        if _, err := vtr.TryGet(addr); err != nil {
            gas += params.WitnessChunkFillCost
        }
	}

	return gas
}

// Merge is used to merge the witness that got generated during the execution
// of a tx, with the accumulation of witnesses that were generated during the
// execution of all the txs preceding this one in a given block.
func (aw *AccessWitness) Merge(other *AccessWitness) {
	// catch unresolved touched addresses
	if len(other.Undefined) != 0 {
		panic("undefined value in witness")
	}

	for k := range other.ReadBranches {
		if _, ok := aw.ReadBranches[k]; !ok {
			aw.ReadBranches[k] = struct{}{}
		}
	}

	for k, chunk := range other.ReadChunks {
		if _, ok := aw.ReadChunks[k]; !ok {
			aw.ReadChunks[k] = chunk
		}
	}

	for k := range other.WriteBranches {
		if _, ok := aw.WriteBranches[k]; !ok {
			aw.WriteBranches[k] = struct{}{}
		}
	}

	for k, chunk := range other.WriteChunks {
		if _, ok := aw.WriteChunks[k]; !ok {
			aw.WriteChunks[k] = chunk
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
