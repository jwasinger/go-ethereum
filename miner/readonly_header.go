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

package miner

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type ReadOnlyHeader struct {
	header *types.Header
}

func (h *ReadOnlyHeader) ParentHash() common.Hash {
	return h.header.ParentHash
}

func (h *ReadOnlyHeader) UncleHash() common.Hash {
	return h.header.UncleHash
}

func (h *ReadOnlyHeader) Coinbase() common.Address {
	return h.header.Coinbase
}

func (h *ReadOnlyHeader) Root() common.Hash {
	return h.header.Root
}

func (h *ReadOnlyHeader) TxHash() common.Hash {
	return h.header.TxHash
}

func (h *ReadOnlyHeader) ReceiptHash() common.Hash {
	return h.header.ReceiptHash
}

func (h *ReadOnlyHeader) Bloom() types.Bloom {
	return h.header.Bloom
}

func (h *ReadOnlyHeader) Difficulty() *big.Int {
	var difficultyCopy *big.Int
	if h.header.Difficulty != nil {
		difficultyCopy = new(big.Int).Set(h.header.Difficulty)
	}

	return difficultyCopy
}

func (h *ReadOnlyHeader) Number() *big.Int {
	var numberCopy *big.Int
	if h.header.Number != nil {
		numberCopy = new(big.Int).Set(h.header.Number)
	}

	return numberCopy
}

func (h *ReadOnlyHeader) GasLimit() uint64 {
	return h.header.GasLimit
}

func (h *ReadOnlyHeader) GasUsed() uint64 {
	return h.header.GasUsed
}

func (h *ReadOnlyHeader) Time() uint64 {
	return h.header.Time
}

func (h *ReadOnlyHeader) Extra() []byte {
	extraCopy := make([]byte, len(h.header.Extra), len(h.header.Extra))
	copy(extraCopy[:], h.header.Extra[:])
	return extraCopy
}

func (h *ReadOnlyHeader) MixDigest() common.Hash {
	return h.header.MixDigest
}

func (h *ReadOnlyHeader) Nonce() types.BlockNonce {
	return h.header.Nonce
}

func (h *ReadOnlyHeader) BaseFee() *big.Int {
	var baseFeeCopy *big.Int
	if h.header.BaseFee != nil {
		baseFeeCopy = new(big.Int).Set(h.header.BaseFee)
	}

	return baseFeeCopy
}
