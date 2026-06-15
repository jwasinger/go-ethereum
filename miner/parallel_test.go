// Copyright 2025 The go-ethereum Authors
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
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/holiman/uint256"
)

var (
	addrCoinbase = common.BytesToAddress([]byte{0xcb})
	addrA        = common.BytesToAddress([]byte{0xa})
	addrB        = common.BytesToAddress([]byte{0xb})
	addrC        = common.BytesToAddress([]byte{0xc})
	slot1        = common.BytesToHash([]byte{0x1})
	slot2        = common.BytesToHash([]byte{0x2})
)

// mkResult builds a FinaliseResult by applying the given mutator to a fresh
// construction access list.
func mkResult(coinbaseRead bool, build func(al *bal.ConstructionBlockAccessList)) *bal.FinaliseResult {
	al := bal.NewConstructionBlockAccessList()
	if build != nil {
		build(al)
	}
	return bal.NewFinaliseResult(al, coinbaseRead)
}

func TestFootprintExcludesCoinbase(t *testing.T) {
	// A transaction that only mutates the coinbase balance (as the fee payment
	// does for every transaction) must produce an empty footprint, otherwise
	// all transactions would conflict with one another.
	res := mkResult(false, func(al *bal.ConstructionBlockAccessList) {
		al.AccountRead(addrCoinbase)
		al.BalanceChange(1, addrCoinbase, uint256.NewInt(1000))
	})
	fp := footprintFor(res, addrCoinbase)
	if len(fp.writeAccounts) != 0 || len(fp.readAccounts) != 0 || len(fp.writeSlots) != 0 || len(fp.readSlots) != 0 {
		t.Fatalf("coinbase fee payment leaked into footprint: %+v", fp)
	}
}

func TestConflictDetection(t *testing.T) {
	storageWriter := func() *footprint {
		return footprintFor(mkResult(false, func(al *bal.ConstructionBlockAccessList) {
			al.StorageWrite(1, addrC, slot1, common.BytesToHash([]byte{0x9}))
		}), addrCoinbase)
	}
	storageReaderSame := func() *footprint {
		return footprintFor(mkResult(false, func(al *bal.ConstructionBlockAccessList) {
			al.StorageRead(addrC, slot1)
		}), addrCoinbase)
	}
	storageReaderOther := func() *footprint {
		return footprintFor(mkResult(false, func(al *bal.ConstructionBlockAccessList) {
			al.StorageRead(addrC, slot2)
		}), addrCoinbase)
	}
	balanceWriter := func() *footprint {
		return footprintFor(mkResult(false, func(al *bal.ConstructionBlockAccessList) {
			al.AccountRead(addrA)
			al.BalanceChange(1, addrA, uint256.NewInt(5))
		}), addrCoinbase)
	}
	accountReader := func() *footprint {
		return footprintFor(mkResult(false, func(al *bal.ConstructionBlockAccessList) {
			al.AccountRead(addrA)
		}), addrCoinbase)
	}

	tests := []struct {
		name string
		a, b *footprint
		want bool
	}{
		{"write-read same slot", storageWriter(), storageReaderSame(), true},
		{"read-write same slot", storageReaderSame(), storageWriter(), true},
		{"write-write same slot", storageWriter(), storageWriter(), true},
		{"read-read same slot", storageReaderSame(), storageReaderSame(), false},
		{"disjoint slots", storageWriter(), storageReaderOther(), false},
		{"account write vs account read", balanceWriter(), accountReader(), true},
		{"account read vs account read", accountReader(), accountReader(), false},
		{"account write vs storage read disjoint", balanceWriter(), storageReaderSame(), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			acc := newFootprint()
			acc.merge(tc.a)
			if got := acc.conflicts(tc.b); got != tc.want {
				t.Fatalf("conflicts = %v, want %v", got, tc.want)
			}
		})
	}
}

// makeSpec wraps a FinaliseResult into a specResult attributed to a synthetic
// sender, so selection can be driven without executing real transactions.
func makeSpec(from common.Address, res *bal.FinaliseResult) *specResult {
	return &specResult{cand: &candidate{from: from}, result: res}
}

func TestSelectNonConflictingPacksDisjoint(t *testing.T) {
	// Three transactions touching disjoint state should all be selected.
	results := []*specResult{
		makeSpec(addrA, mkResult(false, func(al *bal.ConstructionBlockAccessList) { al.StorageWrite(1, addrA, slot1, slot1) })),
		makeSpec(addrB, mkResult(false, func(al *bal.ConstructionBlockAccessList) { al.StorageWrite(1, addrB, slot1, slot1) })),
		makeSpec(addrC, mkResult(false, func(al *bal.ConstructionBlockAccessList) { al.StorageWrite(1, addrC, slot1, slot1) })),
	}
	selected, shift, drop := selectNonConflicting(results, addrCoinbase)
	if len(selected) != 3 {
		t.Fatalf("expected all 3 disjoint txs selected, got %d", len(selected))
	}
	if len(shift) != 0 || len(drop) != 0 {
		t.Fatalf("expected no shifts/drops, got shift=%v drop=%v", shift, drop)
	}
}

func TestSelectNonConflictingLeavesConflicts(t *testing.T) {
	// First (most profitable) writes slot1 of C; second reads it (conflict);
	// third is disjoint. Expect first and third selected, second left in place
	// (NOT dropped).
	results := []*specResult{
		makeSpec(addrA, mkResult(false, func(al *bal.ConstructionBlockAccessList) { al.StorageWrite(1, addrC, slot1, slot1) })),
		makeSpec(addrB, mkResult(false, func(al *bal.ConstructionBlockAccessList) { al.StorageRead(addrC, slot1) })),
		makeSpec(addrA, mkResult(false, func(al *bal.ConstructionBlockAccessList) { al.StorageWrite(1, addrA, slot2, slot2) })),
	}
	selected, shift, drop := selectNonConflicting(results, addrCoinbase)
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected, got %d", len(selected))
	}
	if selected[0].cand.from != addrA || selected[1].cand.from != addrA {
		t.Fatalf("unexpected selection order/senders: %v %v", selected[0].cand.from, selected[1].cand.from)
	}
	// The conflicting tx (sender B) must be neither shifted nor dropped: it
	// stays eligible for a later round.
	for _, d := range append(append([]common.Address{}, shift...), drop...) {
		if d == addrB {
			t.Fatalf("conflicting tx was advanced/dropped; it should be retained")
		}
	}
}

func TestSelectCoinbaseReaderPacksAlone(t *testing.T) {
	// A coinbase-reading tx at the head must be selected alone, even though the
	// following tx is otherwise disjoint.
	results := []*specResult{
		makeSpec(addrA, mkResult(true, func(al *bal.ConstructionBlockAccessList) { al.StorageWrite(1, addrA, slot1, slot1) })),
		makeSpec(addrB, mkResult(false, func(al *bal.ConstructionBlockAccessList) { al.StorageWrite(1, addrB, slot1, slot1) })),
	}
	selected, _, _ := selectNonConflicting(results, addrCoinbase)
	if len(selected) != 1 || selected[0].cand.from != addrA {
		t.Fatalf("expected only the coinbase reader selected, got %d", len(selected))
	}
}

func TestSelectCoinbaseReaderDeferredWhenNotFirst(t *testing.T) {
	// When a coinbase reader is not the most profitable, the batch packs the
	// non-coinbase-reading txs that precede it and stops at the reader (which is
	// left for a round in which it can stand alone). The reader must not be
	// dropped.
	results := []*specResult{
		makeSpec(addrA, mkResult(false, func(al *bal.ConstructionBlockAccessList) { al.StorageWrite(1, addrA, slot1, slot1) })),
		makeSpec(addrB, mkResult(true, func(al *bal.ConstructionBlockAccessList) { al.StorageWrite(1, addrB, slot1, slot1) })),
		makeSpec(addrC, mkResult(false, func(al *bal.ConstructionBlockAccessList) { al.StorageWrite(1, addrC, slot1, slot1) })),
	}
	selected, shift, drop := selectNonConflicting(results, addrCoinbase)
	if len(selected) != 1 || selected[0].cand.from != addrA {
		t.Fatalf("expected only sender A selected before the coinbase reader, got %d", len(selected))
	}
	for _, d := range append(append([]common.Address{}, shift...), drop...) {
		if d == addrB {
			t.Fatalf("coinbase reader must not be advanced/dropped")
		}
	}
}

func TestEffectiveTipOrdering(t *testing.T) {
	// Without a base fee, the effective tip equals the tip cap.
	tip, ok := effectiveTip(lazyTip(10, 10), nil)
	if !ok || !tip.Eq(uint256.NewInt(10)) {
		t.Fatalf("effectiveTip without basefee = %v, %v", tip, ok)
	}
	// With a base fee, tip is capped by feeCap-baseFee.
	base := uint256.NewInt(8)
	tip, ok = effectiveTip(lazyTip(10 /*tipCap*/, 12 /*feeCap*/), base)
	if !ok || !tip.Eq(uint256.NewInt(4)) { // min(10, 12-8) = 4
		t.Fatalf("effectiveTip capped = %v, %v", tip, ok)
	}
	// Fee cap below base fee is not includable.
	if _, ok := effectiveTip(lazyTip(10, 5), base); ok {
		t.Fatalf("expected tx with feeCap < baseFee to be unpayable")
	}
}

// lazyTip builds a minimal LazyTransaction carrying just the fee fields used by
// effectiveTip.
func lazyTip(tipCap, feeCap uint64) *txpool.LazyTransaction {
	return &txpool.LazyTransaction{
		GasTipCap: uint256.NewInt(tipCap),
		GasFeeCap: uint256.NewInt(feeCap),
	}
}
