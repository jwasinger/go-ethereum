package types

import (
	"bytes"
	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
	"sort"
)

//go:generate go run github.com/ferranbt/fastssz/sszgen --path . --objs PerTxAccess,SlotAccess,AccountAccess,BlockAccessList,BalanceDelta,BalanceChange,AccountBalanceDiff,CodeChange,AccountCodeDiff,AccountNonce,NonceDiffs,encoderBal --output bal_encoding.go

// encoder types

type PerTxAccess struct {
	TxIdx      uint64 `ssz-size:"2"`
	ValueAfter [32]byte
}

type SlotAccess struct {
	Slot     [32]byte      `ssz-size:"32"`
	Accesses []PerTxAccess `ssz-max:"30000"`
}

type AccountAccess struct {
	Address  [20]byte     `ssz-size:"32"`
	Accesses []SlotAccess `ssz-max:"300000"`
	code     []byte       `ssz-max:"24576"` // this is currently a union in the EIP spec, but unions aren't used anywhere in practice so I implement it as a list here.
}

type accountAccessList []AccountAccess

type BalanceDelta [12]byte // {}-endian signed integer

type BalanceChange struct {
	TxIdx uint64 `ssz-size:"2"`
	Delta BalanceDelta
}

type AccountBalanceDiff struct {
	Address [40]byte
	Changes []BalanceChange `ssz-max:"30000"`
}

// TODO: implement encoder/decoder manually on this, as we can't specify tags for a type declaration
type balanceDiffs = []AccountBalanceDiff

type CodeChange struct {
	TxIdx   uint64 `ssz-size:"2"`
	NewCode []byte `ssz-max:"24576"`
}

type AccountCodeDiff struct {
	Address [40]byte
	Changes []CodeChange `ssz-max:"30000"`
}

// TODO: implement encoder/decoder manually on this, as we can't specify tags for a type declaration
type codeDiffs []AccountCodeDiff

type AccountNonce struct {
	Address    [40]byte
	NonceAfter uint64
}

// TODO: implement encoder/decoder manually on this, as we can't specify tags for a type declaration
type nonceDiffs []AccountNonce

type encoderBAL struct {
	AccountAccesses accountAccessList
	BalanceDiffs    balanceDiffs
	CodeDiffs       codeDiffs
	NonceDiffs      nonceDiffs
}

type slotAccess struct {
	writes map[uint64]common.Hash // map of tx index to post-tx slot value
}

type accountAccess struct {
	address  common.Address
	accesses map[common.Hash]slotAccess // map of slot key to all post-tx values where that slot was read/written
	code     []byte
}

func (a *accountAccess) MarkRead(key common.Hash) {
	if _, ok := a.accesses[key]; !ok {
		a.accesses[key] = slotAccess{
			make(map[uint64]common.Hash),
		}
	}
}

func (a *accountAccess) MarkWrite(txIdx uint64, key, value common.Hash) {
	if _, ok := a.accesses[key]; !ok {
		a.accesses[key] = slotAccess{
			make(map[uint64]common.Hash),
		}
	}

	a.accesses[key].writes[txIdx] = value
}

// map of transaction idx to the new code
type codeDiff struct {
	txIdx uint64
	code  []byte
}

type balanceDiff map[uint64]*uint256.Int

// map of tx-idx to pre-state nonce
type nonceDiff map[uint64]uint64

type BlockAccessList struct {
	accountAccesses map[common.Address]*accountAccess
	codeChanges     map[common.Address]codeDiff
	prestateNonces  map[common.Address]nonceDiff
	balanceChanges  map[common.Address]balanceDiff
}

func NewBlockAccessList() *BlockAccessList {
	return &BlockAccessList{
		make(map[common.Address]*accountAccess),
		make(map[common.Address]codeDiff),
		make(map[common.Address]nonceDiff),
		make(map[common.Address]balanceDiff),
	}
}

func (b *BlockAccessList) Eq(other *BlockAccessList) bool {

	// check that the account accesses are equal (consider moving this into its own function)

	if len(b.accountAccesses) != len(other.accountAccesses) {
		return false
	}
	for address, aa := range b.accountAccesses {
		otherAA, ok := other.accountAccesses[address]
		if !ok {
			return false
		}
		if len(aa.accesses) != len(otherAA.accesses) {
			return false
		}
		for key, vals := range aa.accesses {
			otherAccesses, ok := otherAA.accesses[key]
			if !ok {
				return false
			}
			if len(vals.writes) != len(otherAccesses.writes) {
				return false
			}

			for i, writeVal := range vals.writes {
				otherWriteVal, ok := otherAccesses.writes[i]
				if !ok {
					return false
				}
				if writeVal != otherWriteVal {
					return false
				}
			}
		}
	}

	// check that the code changes are equal

	if len(b.codeChanges) != len(other.codeChanges) {
		return false
	}
	for addr, codeCh := range b.codeChanges {
		otherCodeCh, ok := other.codeChanges[addr]
		if !ok {
			return false
		}
		if codeCh.txIdx != otherCodeCh.txIdx {
			return false
		}
		if bytes.Compare(codeCh.code, otherCodeCh.code) != 0 {
			return false
		}
	}

	if len(b.prestateNonces) != len(other.prestateNonces) {
		return false
	}
	for addr, nonces := range b.prestateNonces {
		otherNonces, ok := other.prestateNonces[addr]
		if !ok {
			return false
		}

		if len(nonces) != len(otherNonces) {
			return false
		}

		for txIdx, nonce := range nonces {
			otherNonce, ok := otherNonces[txIdx]
			if !ok {
				return false
			}
			if nonce != otherNonce {
				return false
			}
		}
	}

	if len(b.balanceChanges) != len(other.balanceChanges) {
		return false
	}

	for addr, balanceChanges := range b.balanceChanges {
		otherBalanceChanges, ok := other.balanceChanges[addr]
		if !ok {
			return false
		}

		if len(balanceChanges) != len(otherBalanceChanges) {
			return false
		}

		for txIdx, balanceCh := range balanceChanges {
			otherBalanceCh, ok := otherBalanceChanges[txIdx]
			if !ok {
				return false
			}

			if balanceCh != otherBalanceCh {
				return false
			}
		}
	}
	return true
}

// called during tx finalisation for each dirty account with changed nonce (whether by being the sender of a tx or calling CREATE)
func (b *BlockAccessList) NonceDiff(txIdx uint64, address common.Address, originNonce uint64) {
	if _, ok := b.prestateNonces[address]; !ok {
		b.prestateNonces[address] = make(nonceDiff)
	}
	b.prestateNonces[address][txIdx] = originNonce
}

// called during tx finalisation for each
func (b *BlockAccessList) BalanceChange(txIdx uint64, address common.Address, balance *uint256.Int) {
	if _, ok := b.balanceChanges[address]; !ok {
		b.balanceChanges[address] = make(balanceDiff)
	}
	b.balanceChanges[address][txIdx] = balance.Clone()
}

// TODO for eip:  specify that storage slots which are read/modified for accounts that are created/selfdestructed
// in same transaction aren't included in teh BAL (?)

// TODO for eip:  specify that storage slots of newly-created accounts which are only read are not included in the BAL (?)

// called during tx execution every time a storage slot is read
func (b *BlockAccessList) StorageRead(address common.Address, key common.Hash) {
	if _, ok := b.accountAccesses[address]; !ok {
		b.accountAccesses[address] = &accountAccess{
			address,
			make(map[common.Hash]slotAccess),
			nil,
		}
	}
	b.accountAccesses[address].MarkRead(key)
}

// called every time a mutated storage value is committed upon transaction finalization
func (b *BlockAccessList) StorageWrite(txIdx uint64, address common.Address, key, value common.Hash) {
	if _, ok := b.accountAccesses[address]; !ok {
		b.accountAccesses[address] = &accountAccess{
			address,
			make(map[common.Hash]slotAccess),
			nil,
		}
	}
	b.accountAccesses[address].MarkWrite(txIdx, key, value)
}

// called during tx finalisation for each dirty account with mutated code
func (b *BlockAccessList) CodeChange(txIdx uint64, address common.Address, code []byte) {
	if _, ok := b.codeChanges[address]; !ok {
		b.codeChanges[address] = codeDiff{}
	}
	b.codeChanges[address] = codeDiff{
		txIdx,
		bytes.Clone(code),
	}
}

func (b *BlockAccessList) EncodeSSZ(result []byte) {
	var (
		accountAccessesAddrs   []common.Address
		encoderAccountAccesses accountAccessList
	)

	for addr, _ := range b.accountAccesses {
		accountAccessesAddrs = append(accountAccessesAddrs, addr)
	}
	sort.Slice(accountAccessesAddrs, func(i, j int) bool {
		return bytes.Compare(accountAccessesAddrs[i][:], accountAccessesAddrs[j][:]) < 0
	})
	for _, addr := range accountAccessesAddrs {
		encoderAccountAccesses = append(encoderAccountAccesses, AccountAccess{
			Address:  addr,
			Accesses: nil,
			code:     b.accountAccesses[addr].code,
		})
		// sort the accesses lexicographically by key, and the occurance of each key ascending by tx idx
		// then encode them
		var storageAccessKeys []common.Hash
		for key, _ := range b.accountAccesses[addr].accesses {
			storageAccessKeys = append(storageAccessKeys, key)
		}
		sort.Slice(storageAccessKeys, func(i, j int) bool {
			return bytes.Compare(storageAccessKeys[i][:], storageAccessKeys[j][:]) < 0
		})
		var accesses []SlotAccess
		for _, accessSlot := range storageAccessKeys {
			var access slotAccess
		}
	}
	encoderObj := encoderBAL{
		AccountAccesses: nil,
		BalanceDiffs:    nil,
		CodeDiffs:       nil,
		NonceDiffs:      nil,
	}
}
