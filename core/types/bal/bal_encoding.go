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

package bal

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

//go:generate go run github.com/ethereum/go-ethereum/rlp/rlpgen -out bal_encoding_rlp_generated.go -type BlockAccessList -decoder

// These are objects used as input for the access list encoding. They mirror
// the spec format.

// BlockAccessList is the encoding format of ConstructionBlockAccessList.
type BlockAccessList struct {
	Accesses []AccountAccess `ssz-max:"300000" json:"accesses"`
}

// Validate returns an error if the contents of the access list are not ordered
// according to the spec or any code changes are contained which exceed protocol
// max code size.
func (e *BlockAccessList) Validate() error {
	if !slices.IsSortedFunc(e.Accesses, func(a, b AccountAccess) int {
		return bytes.Compare(a.Address[:], b.Address[:])
	}) {
		return errors.New("block access list accounts not in lexicographic order")
	}
	for _, entry := range e.Accesses {
		if err := entry.validate(); err != nil {
			return err
		}
	}
	return nil
}

// Hash computes the keccak256 hash of the access list
func (e *BlockAccessList) Hash() common.Hash {
	var enc bytes.Buffer
	err := e.EncodeRLP(&enc)
	if err != nil {
		// errors here are related to BAL values exceeding maximum size defined
		// by the spec. Hard-fail because these cases are not expected to be hit
		// under reasonable conditions.
		panic(err)
	}
	return crypto.Keccak256Hash(enc.Bytes())
}

// encodeBalance encodes the provided balance into 16-bytes.
func encodeBalance(val *uint256.Int) [16]byte {
	valBytes := val.Bytes()
	if len(valBytes) > 16 {
		panic("can't encode value that is greater than 16 bytes in size")
	}
	var enc [16]byte
	copy(enc[16-len(valBytes):], valBytes[:])
	return enc
}

// encodingBalanceChange is the encoding format of BalanceChange.
type encodingBalanceChange struct {
	TxIdx   uint16   `ssz-size:"2" json:"txIndex"`
	Balance [16]byte `ssz-size:"16" json:"balance"`
}

// encodingAccountNonce is the encoding format of NonceChange.
type encodingAccountNonce struct {
	TxIdx uint16 `ssz-size:"2" json:"txIndex"`
	Nonce uint64 `ssz-size:"8" json:"nonce"`
}

// encodingStorageWrite is the encoding format of StorageWrites.
type encodingStorageWrite struct {
	TxIdx      uint16      `json:"txIndex"`
	ValueAfter common.Hash `ssz-size:"32" json:"valueAfter"`
}

// encodingStorageWrite is the encoding format of SlotWrites.
type encodingSlotWrites struct {
	Slot     common.Hash            `ssz-size:"32" json:"slot"`
	Accesses []encodingStorageWrite `ssz-max:"300000" json:"accesses"`
}

// validate returns an instance of the encoding-representation slot writes in
// working representation.
func (e *encodingSlotWrites) validate() error {
	if slices.IsSortedFunc(e.Accesses, func(a, b encodingStorageWrite) int {
		return cmp.Compare[uint16](a.TxIdx, b.TxIdx)
	}) {
		return nil
	}
	return errors.New("storage write tx indices not in order")
}

// AccountAccess is the encoding format of ConstructionAccountAccess.
type AccountAccess struct {
	Address        common.Address          `ssz-size:"20" json:"address,omitempty"`           // 20-byte Ethereum address
	StorageWrites  []encodingSlotWrites    `ssz-max:"300000" json:"storageWrites,omitempty"`  // Storage changes (slot -> [tx_index -> new_value])
	StorageReads   []common.Hash           `ssz-max:"300000" json:"storageReads,omitempty"`   // Read-only storage keys
	BalanceChanges []encodingBalanceChange `ssz-max:"300000" json:"balanceChanges,omitempty"` // Balance changes ([tx_index -> post_balance])
	NonceChanges   []encodingAccountNonce  `ssz-max:"300000" json:"nonceChanges,omitempty"`   // Nonce changes ([tx_index -> new_nonce])
	Code           []CodeChange            `ssz-max:"1" json:"code,omitempty"`                // Code changes ([tx_index -> new_code])
}

// validate converts the account accesses out of encoding format.
// If any of the keys in the encoding object are not ordered according to the
// spec, an error is returned.
func (e *AccountAccess) validate() error {
	// Check the storage write slots are sorted in order
	if !slices.IsSortedFunc(e.StorageWrites, func(a, b encodingSlotWrites) int {
		return bytes.Compare(a.Slot[:], b.Slot[:])
	}) {
		return errors.New("storage writes slots not in lexicographic order")
	}
	for _, write := range e.StorageWrites {
		if err := write.validate(); err != nil {
			return err
		}
	}

	// Check the storage read slots are sorted in order
	if !slices.IsSortedFunc(e.StorageReads, func(a, b common.Hash) int {
		return bytes.Compare(a[:], b[:])
	}) {
		return errors.New("storage read slots not in lexicographic order")
	}

	// Check the balance changes are sorted in order
	if !slices.IsSortedFunc(e.BalanceChanges, func(a, b encodingBalanceChange) int {
		return cmp.Compare[uint16](a.TxIdx, b.TxIdx)
	}) {
		return errors.New("balance changes not in ascending order by tx index")
	}

	// Check the nonce changes are sorted in order
	if !slices.IsSortedFunc(e.NonceChanges, func(a, b encodingAccountNonce) int {
		return cmp.Compare[uint16](a.TxIdx, b.TxIdx)
	}) {
		return errors.New("nonce changes not in ascending order by tx index")
	}

	// Convert code change
	if len(e.Code) == 1 {
		if len(e.Code[0].Code) > params.MaxCodeSize {
			return fmt.Errorf("code change contained oversized code")
		}
	}
	return nil
}

// Copy returns a deep copy of the account access
func (e *AccountAccess) Copy() AccountAccess {
	res := AccountAccess{
		Address:        e.Address,
		StorageReads:   slices.Clone(e.StorageReads),
		BalanceChanges: slices.Clone(e.BalanceChanges),
		NonceChanges:   slices.Clone(e.NonceChanges),
	}
	for _, storageWrite := range e.StorageWrites {
		res.StorageWrites = append(res.StorageWrites, encodingSlotWrites{
			Slot:     storageWrite.Slot,
			Accesses: slices.Clone(storageWrite.Accesses),
		})
	}
	if len(e.Code) == 1 {
		res.Code = []CodeChange{
			{
				e.Code[0].TxIndex,
				bytes.Clone(e.Code[0].Code),
			},
		}
	}
	return res
}

// EncodeRLP returns the RLP-encoded access list
func (c *ConstructionBlockAccessList) EncodeRLP(wr io.Writer) error {
	return c.ToEncodingObj().EncodeRLP(wr)
}

var _ rlp.Encoder = &ConstructionBlockAccessList{}

// toEncodingObj creates an instance of the ConstructionAccountAccess of the type that is
// used as input for the encoding.
func (a *ConstructionAccountAccess) toEncodingObj(addr common.Address) AccountAccess {
	res := AccountAccess{
		Address:        addr,
		StorageWrites:  make([]encodingSlotWrites, 0),
		StorageReads:   make([]common.Hash, 0),
		BalanceChanges: make([]encodingBalanceChange, 0),
		NonceChanges:   make([]encodingAccountNonce, 0),
		Code:           nil,
	}

	// Convert write slots
	writeSlots := slices.Collect(maps.Keys(a.StorageWrites))
	slices.SortFunc(writeSlots, common.Hash.Cmp)
	for _, slot := range writeSlots {
		var obj encodingSlotWrites
		obj.Slot = slot

		slotWrites := a.StorageWrites[slot]
		obj.Accesses = make([]encodingStorageWrite, 0, len(slotWrites))

		indices := slices.Collect(maps.Keys(slotWrites))
		slices.SortFunc(indices, cmp.Compare[uint16])
		for _, index := range indices {
			obj.Accesses = append(obj.Accesses, encodingStorageWrite{
				TxIdx:      index,
				ValueAfter: slotWrites[index],
			})
		}
		res.StorageWrites = append(res.StorageWrites, obj)
	}

	// Convert read slots
	readSlots := slices.Collect(maps.Keys(a.StorageReads))
	slices.SortFunc(readSlots, common.Hash.Cmp)
	for _, slot := range readSlots {
		res.StorageReads = append(res.StorageReads, slot)
	}

	// Convert balance changes
	balanceIndices := slices.Collect(maps.Keys(a.BalanceChanges))
	slices.SortFunc(balanceIndices, cmp.Compare[uint16])
	for _, idx := range balanceIndices {
		res.BalanceChanges = append(res.BalanceChanges, encodingBalanceChange{
			TxIdx:   idx,
			Balance: encodeBalance(a.BalanceChanges[idx]),
		})
	}

	// Convert nonce changes
	nonceIndices := slices.Collect(maps.Keys(a.NonceChanges))
	slices.SortFunc(nonceIndices, cmp.Compare[uint16])
	for _, idx := range nonceIndices {
		res.NonceChanges = append(res.NonceChanges, encodingAccountNonce{
			TxIdx: idx,
			Nonce: a.NonceChanges[idx],
		})
	}

	// Convert code change
	if a.CodeChange != nil {
		res.Code = []CodeChange{
			{
				a.CodeChange.TxIndex,
				bytes.Clone(a.CodeChange.Code),
			},
		}
	}
	return res
}

// ToEncodingObj returns an instance of the access list expressed as the type
// which is used as input for the encoding/decoding.
func (c *ConstructionBlockAccessList) ToEncodingObj() *BlockAccessList {
	var addresses []common.Address
	for addr := range c.Accounts {
		addresses = append(addresses, addr)
	}
	slices.SortFunc(addresses, common.Address.Cmp)

	var res BlockAccessList
	for _, addr := range addresses {
		res.Accesses = append(res.Accesses, c.Accounts[addr].toEncodingObj(addr))
	}
	return &res
}

func (e *BlockAccessList) PrettyPrint() string {
	var res bytes.Buffer
	printWithIndent := func(indent int, text string) {
		fmt.Fprintf(&res, "%s%s\n", strings.Repeat("    ", indent), text)
	}
	for _, accountDiff := range e.Accesses {
		printWithIndent(0, fmt.Sprintf("%x:", accountDiff.Address))

		printWithIndent(1, "storage writes:")
		for _, sWrite := range accountDiff.StorageWrites {
			printWithIndent(2, fmt.Sprintf("%x:", sWrite.Slot))
			for _, access := range sWrite.Accesses {
				printWithIndent(3, fmt.Sprintf("%d: %x", access.TxIdx, access.ValueAfter))
			}
		}

		printWithIndent(1, "storage reads:")
		for _, slot := range accountDiff.StorageReads {
			printWithIndent(2, fmt.Sprintf("%x", slot))
		}

		printWithIndent(1, "balance changes:")
		for _, change := range accountDiff.BalanceChanges {
			balance := new(uint256.Int).SetBytes(change.Balance[:]).String()
			printWithIndent(2, fmt.Sprintf("%d: %s", change.TxIdx, balance))
		}

		printWithIndent(1, "nonce changes:")
		for _, change := range accountDiff.NonceChanges {
			printWithIndent(2, fmt.Sprintf("%d: %d", change.TxIdx, change.Nonce))
		}

		if len(accountDiff.Code) > 0 {
			printWithIndent(1, "code:")
			printWithIndent(2, fmt.Sprintf("%d: %x", accountDiff.Code[0].TxIndex, accountDiff.Code[0].Code))
		}
	}
	return res.String()
}

// Copy returns a deep copy of the access list
func (e *BlockAccessList) Copy() (res BlockAccessList) {
	for _, accountAccess := range e.Accesses {
		res.Accesses = append(res.Accesses, accountAccess.Copy())
	}
	return
}

type AccountState struct {
	Balance *[16]byte
	Nonce   *uint64

	// TODO: this can refer to the code of a delegated account.  as delegations
	// are not dependent on the code size of the delegation target, naively including
	// this in the state diff (done in statedb when we augment BAL state diffs to include
	// delegated accounts), could balloon the size of the state diff.
	//
	// Instead of having a pointer to the bytes here, we should have this refer to a resolver
	// that can load the code when needed (or it might be already loaded in some state object).
	Code *[]byte

	StorageWrites map[common.Hash]common.Hash
}

func (a *AccountState) Copy() (res AccountState) {
	if a.Balance != nil {
		var balanceCopy [16]byte
		copy(balanceCopy[:], (*a.Balance)[:])
		res.Balance = &balanceCopy
	}
	if a.Nonce != nil {
		res.Nonce = new(uint64)
		*res.Nonce = *a.Nonce
	}
	if a.Code != nil {
		res.Code = new([]byte)
		*res.Code = bytes.Clone(*a.Code)
	}
	if a.StorageWrites != nil {
		res.StorageWrites = maps.Clone(a.StorageWrites)
	}
	return
}

type StateDiff struct {
	Mutations map[common.Address]*AccountState
	// TODO: this diff will be augmented with 7702 delegations.  Do we store the delegation code directly in the diff or resolve it as needed?
	// I lean towards, resolve as needed (at least initially), or only resolve if the delegation code will be used further on in the block.
}

func (s *StateDiff) Merge(next *StateDiff) *StateDiff {
	// merge the future state from next into the current diff
	panic("not implemented!")
	return nil
}

func (s *StateDiff) Copy() *StateDiff {
	var res StateDiff
	for addr, accountDiff := range s.Mutations {
		cpy := accountDiff.Copy()
		res.Mutations[addr] = &cpy
	}
	return &res
}

type AccountIterator struct {
	slotWriteIndices [][]int
	balanceChangeIdx int
	nonceChangeIdx   int
	codeChangeIdx    int

	curIdx int
	maxIdx int
	aa     *AccountAccess
}

func NewAccountIterator(accesses *AccountAccess, txCount int) *AccountIterator {
	return &AccountIterator{
		slotWriteIndices: make([][]int, len(accesses.StorageWrites)),
		balanceChangeIdx: 0,
		nonceChangeIdx:   0,
		codeChangeIdx:    0,
		curIdx:           0,
		maxIdx:           txCount - 1,
		aa:               accesses,
	}
}

// increment the account iterator by one, returning only the mutated state by the new transaction
func (it *AccountIterator) Increment() (accountState *AccountState, mut bool) {
	if it.curIdx == it.maxIdx {
		return nil, false
	}

	layerMut := AccountState{
		Balance:       nil,
		Nonce:         nil,
		Code:          nil,
		StorageWrites: make(map[common.Hash]common.Hash),
	}
	it.curIdx++
	for i, slotIdxs := range it.slotWriteIndices {
		for _, curSlotIdx := range slotIdxs {
			if curSlotIdx == it.curIdx {
				storageWrite := it.aa.StorageWrites[i].Accesses[curSlotIdx]
				if storageWrite.TxIdx == uint16(it.curIdx) {
					layerMut.StorageWrites[it.aa.StorageWrites[i].Slot] = storageWrite.ValueAfter
				}
			}
		}
	}

	if it.aa.BalanceChanges[it.balanceChangeIdx].TxIdx == uint16(it.curIdx) {
		balance := it.aa.BalanceChanges[it.balanceChangeIdx].Balance
		layerMut.Balance = &balance
		it.balanceChangeIdx++
	}

	if it.aa.Code[it.codeChangeIdx].TxIndex == uint16(it.curIdx) {
		newCode := bytes.Clone(it.aa.Code[it.codeChangeIdx].Code)
		layerMut.Code = &newCode
		it.codeChangeIdx++
	}

	if it.aa.NonceChanges[it.nonceChangeIdx].TxIdx == uint16(it.curIdx) {
		layerMut.Nonce = new(uint64)
		*layerMut.Nonce = it.aa.NonceChanges[it.nonceChangeIdx].Nonce
		it.nonceChangeIdx++
	}

	isMut := len(layerMut.StorageWrites) > 0 || layerMut.Code != nil || layerMut.Nonce != nil || layerMut.Balance != nil
	return &layerMut, isMut
}

type BALIterator struct {
	bal           *BlockAccessList
	acctIterators map[common.Address]*AccountIterator
	curIdx        uint16
}

func NewIterator(b *BlockAccessList, txCount int) *BALIterator {
	accounts := make(map[common.Address]*AccountIterator)
	for _, aa := range b.Accesses {
		accounts[aa.Address] = NewAccountIterator(&aa, txCount)
	}
	return &BALIterator{
		b,
		accounts,
		0,
	}
}

// Iterate one transaction into the BAL, returning the state diff from that tx
func (it *BALIterator) Next() (mutations *StateDiff) {
	// TODO: maintain a single StateDiff, and use this method to update it and return a pointer to it.
	panic("implement me!!!")
	return nil
}

// return nil if there is no state diff (can this happen with base-fee burning, does the base-fee portion get burned when the tx is applied or at the end of the block when crediting the coinbase?)
func (it *BALIterator) BuildStateDiff(until uint16, onTx func(txIndex uint16, accumDiff, txDiff *StateDiff) error) (*StateDiff, error) {
	if until < it.curIdx {
		return nil, nil
	}

	var accumDiff *StateDiff

	for ; it.curIdx < until; it.curIdx++ {
		// update accumDiff based on the BAL

		layerMutations := StateDiff{
			make(map[common.Address]*AccountState),
		}
		for addr, acctIt := range it.acctIterators {
			if diff, mut := acctIt.Increment(); mut {
				layerMutations.Mutations[addr] = diff
			}
		}

		// callback to fill in state mutations that can't be sourced from the BAL:
		// * EOA tx sender nonce increments
		// * 7702 delegations
		if err := onTx(it.curIdx, accumDiff, &layerMutations); err != nil {
			return nil, err
		}

		accumDiff.Merge(&layerMutations)
	}

	// TODO: return a copy of the accumed diff
	return nil, nil
}
