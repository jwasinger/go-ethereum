package types

import (
	"bytes"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
	"io"
	"maps"
	"sort"
	"strings"
)

//go:generate go run github.com/ferranbt/fastssz/sszgen --path . --objs encodingPerTxAccess,encodingSlotAccess,encodingAccountAccess,encodingAccountAccessList,encodingBlockAccessList,encodingBalanceDelta,encodingBalanceChange,encodingAccountBalanceDiff,encodingCodeChange,encodingAccountNonce,encodingNonceDiffs,encodingBlockAccessList --output bal_encoding_generated.go

// encoder types

type encodingPerTxAccess struct {
	TxIdx      uint64   `ssz-size:"2"`
	ValueAfter [32]byte `ssz-size:"32"`
}

type encodingSlotAccess struct {
	Slot     [32]byte              `ssz-size:"32"`
	Accesses []encodingPerTxAccess `ssz-max:"30000"`
}

type encodingAccountAccess struct {
	Address  [20]byte             `ssz-size:"20"`
	Accesses []encodingSlotAccess `ssz-max:"300000"`
	Code     []byte               `ssz-max:"24576"`
}

type encodingAccountAccessList []encodingAccountAccess

// TODO: this is 12 bytes in the spec
// TODO: verify that Geth encodes the endianess according to the spec
type encodingBalanceDelta [32]byte

type encodingBalanceChange struct {
	TxIdx uint64 `ssz-size:"2"`
	Delta encodingBalanceDelta
}

type encodingAccountBalanceDiff struct {
	Address [20]byte
	Changes []encodingBalanceChange `ssz-max:"30000"`
}

// TODO: implement encoder/decoder manually on this, as we can't specify tags for a type declaration
type encodingBalanceDiffs = []encodingAccountBalanceDiff

type encodingAccountCodeDiff struct {
	Address [20]byte
	TxIdx   uint64 `ssz-size:"2"`
	NewCode []byte `ssz-max:"24576"`
}

// TODO: implement encoder/decoder manually on this, as we can't specify tags for a type declaration
type encodingCodeDiffs []encodingAccountCodeDiff

type encodingAccountNonce struct {
	Address     [20]byte
	NonceBefore uint64
}

// TODO: implement encoder/decoder manually on this, as we can't specify tags for a type declaration
type encodingNonceDiffs []encodingAccountNonce

type encodingBlockAccessList struct {
	AccountAccesses encodingAccountAccessList `ssz-max:"100"`
	BalanceDiffs    encodingBalanceDiffs      `ssz-max:"100"`
	CodeDiffs       encodingCodeDiffs         `ssz-max:"100"`
	NonceDiffs      encodingNonceDiffs        `ssz-max:"100"`
}

func (c encodingCodeDiffs) toMap() (map[common.Address]codeDiff, error) {
	var prevAddr *common.Address
	res := make(map[common.Address]codeDiff)
	for _, diff := range c {
		if prevAddr != nil {
			if bytes.Compare(diff.Address[:], (*prevAddr)[:]) <= 0 {
				return nil, fmt.Errorf("code diffs not in lexicographic order")
			}
		}
		res[diff.Address] = codeDiff{
			diff.TxIdx,
			bytes.Clone(diff.NewCode),
		}
		var p common.Address = diff.Address
		prevAddr = &p
	}
	return res, nil
}

func (c *encodingAccountBalanceDiff) toMap() (balanceDiff, error) {
	var prevIdx *uint64
	res := make(balanceDiff)
	for _, diff := range c.Changes {
		if prevIdx != nil {
			if *prevIdx > diff.TxIdx {
				return nil, fmt.Errorf("not in lexicographic ordering")
			}
		}
		res[diff.TxIdx] = new(uint256.Int).SetBytes(diff.Delta[:])
	}
	return res, nil
}

// TODO: make this a function on the parameter tpye
func encodingBalanceDiffsToMap(c encodingBalanceDiffs) (map[common.Address]balanceDiff, error) {
	var prevAddr *common.Address
	res := make(map[common.Address]balanceDiff)
	for _, diff := range c {
		if prevAddr != nil {
			if bytes.Compare(diff.Address[:], (*prevAddr)[:]) <= 0 {
				return nil, fmt.Errorf("balance diffs not in lexicographic order")
			}
		}
		mp, err := diff.toMap()
		if err != nil {
			return nil, err
		}
		res[diff.Address] = mp
		var p common.Address = diff.Address
		prevAddr = &p
	}
	return res, nil
}

func (a *encodingSlotAccess) toSlotAccess() (*slotAccess, error) {
	var prevIdx *uint64
	res := slotAccess{make(map[uint64]common.Hash)}
	for _, diff := range a.Accesses {
		if prevIdx != nil {
			if *prevIdx > diff.TxIdx {
				return nil, fmt.Errorf("not in lexicographic ordering")
			}
		}
		res.writes[diff.TxIdx] = diff.ValueAfter
		prevIdx = &diff.TxIdx
	}
	return &res, nil
}

func (a *encodingAccountAccess) toAccountAccess() (*accountAccess, error) {
	res := accountAccess{
		a.Address,
		make(map[common.Hash]slotAccess),
		bytes.Clone(a.Code),
	}
	var prevSlot *[32]byte
	for _, diff := range a.Accesses {
		if prevSlot != nil {
			if bytes.Compare(diff.Slot[:], (*prevSlot)[:]) <= 0 {
				return nil, fmt.Errorf("storage slots not in lexicographic order")
			}
		}
		mp, err := diff.toSlotAccess()
		if err != nil {
			return nil, err
		}
		res.accesses[diff.Slot] = *mp
		prevSlot = &diff.Slot
	}
	return &res, nil
}

func encodingAccountAccessListToMap(al encodingAccountAccessList) (map[common.Address]*accountAccess, error) {
	var prevAddr *common.Address
	res := make(map[common.Address]*accountAccess)
	for _, diff := range al {
		if prevAddr != nil {
			if bytes.Compare(diff.Address[:], (*prevAddr)[:]) < 0 {
				return nil, fmt.Errorf("accounts not in lexicographic order")
			}
		}
		mp, err := diff.toAccountAccess()
		if err != nil {
			return nil, err
		}
		res[diff.Address] = mp
		var p common.Address = diff.Address
		prevAddr = &p
	}
	return res, nil
}

func (n encodingNonceDiffs) toMap() (map[common.Address]uint64, error) {
	var prevAddr *common.Address
	res := make(map[common.Address]uint64)
	for _, diff := range n {
		if prevAddr != nil {
			if bytes.Compare(diff.Address[:], (*prevAddr)[:]) < 0 {
				return nil, fmt.Errorf("nonce diff accounts not in lexicographic order")
			}
		}
		res[diff.Address] = diff.NonceBefore
		var p common.Address = diff.Address
		prevAddr = &p
	}
	return res, nil
}

func (b *encodingBlockAccessList) ToBlockAccessList() (*BlockAccessList, error) {
	accountAccesses, err := encodingAccountAccessListToMap(b.AccountAccesses)
	if err != nil {
		return nil, err
	}
	balanceChanges, err := encodingBalanceDiffsToMap(b.BalanceDiffs)
	if err != nil {
		return nil, err
	}
	codeChanges, err := b.CodeDiffs.toMap()
	if err != nil {
		return nil, err
	}
	nonceDiffs, err := b.NonceDiffs.toMap()
	if err != nil {
		return nil, err
	}
	return &BlockAccessList{
		accountAccesses,
		balanceChanges,
		codeChanges,
		nonceDiffs,
		common.Hash{},
	}, nil
}

// non-encoder objects

func nonceDiffsToEncoderObj(nonceDiffs map[common.Address]uint64) (res encodingNonceDiffs) {
	var addrs []common.Address
	for addr, _ := range nonceDiffs {
		addrs = append(addrs, addr)
	}

	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i][:], addrs[j][:]) > 0
	})

	for _, addr := range addrs {
		res = append(res, encodingAccountNonce{
			Address:     addr,
			NonceBefore: nonceDiffs[addr],
		})
	}
	return res
}

type slotAccess struct {
	writes map[uint64]common.Hash // map of tx index to post-tx slot value
}

func (s slotAccess) toEncoderObj(key common.Hash) (res encodingSlotAccess) {
	var (
		slotIdxs []uint64
	)
	res.Slot = key
	for sIdx, _ := range s.writes {
		slotIdxs = append(slotIdxs, sIdx)
	}
	sort.Slice(slotIdxs, func(i, j int) bool {
		return slotIdxs[i] < slotIdxs[j]
	})
	for _, slotIdx := range slotIdxs {
		res.Accesses = append(res.Accesses, encodingPerTxAccess{
			slotIdx,
			s.writes[slotIdx],
		})
	}
	return
}

type accountAccess struct {
	address  common.Address
	accesses map[common.Hash]slotAccess // map of slot key to all post-tx values where that slot was read/written
	code     []byte
}

func (a *accountAccess) Copy() *accountAccess {
	accesses := make(map[common.Hash]slotAccess)
	for key, access := range a.accesses {
		accesses[key] = slotAccess{maps.Clone(access.writes)}
	}

	return &accountAccess{
		a.address,
		accesses,
		bytes.Clone(a.code),
	}
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

const maxValBytes = 32 // TODO: change this...

func (b *encodingBalanceDelta) Set(val *uint256.Int) *encodingBalanceDelta {
	valBytes := val.Bytes()
	if len(valBytes) > maxValBytes {
		panic("can't encode value that is greater than 12 bytes in size")
	}
	copy(b[maxValBytes-len(valBytes):], valBytes[:])
	return b
}

type balanceDiff map[uint64]*uint256.Int

func (b balanceDiff) Copy() balanceDiff {
	res := make(map[uint64]*uint256.Int)
	for idx, balance := range b {
		res[idx] = balance.Clone()
	}
	return res
}

func (b balanceDiff) toEncoderObj(addr common.Address) (res encodingAccountBalanceDiff) {
	res.Address = addr
	var diffIdxs []uint64
	for txIdx, _ := range b {
		diffIdxs = append(diffIdxs, txIdx)
	}
	sort.Slice(diffIdxs, func(i, j int) bool {
		return diffIdxs[i] < diffIdxs[j]
	})

	for _, idx := range diffIdxs {
		res.Changes = append(res.Changes, encodingBalanceChange{
			TxIdx: idx,
			Delta: *new(encodingBalanceDelta).Set(b[idx]),
		})
	}
	return res
}

type codeDiff struct {
	txIdx uint64
	code  []byte
}

func (c *codeDiff) Copy() codeDiff {
	return codeDiff{
		c.txIdx,
		bytes.Clone(c.code),
	}
}

type BlockAccessList struct {
	accountAccesses map[common.Address]*accountAccess
	balanceChanges  map[common.Address]balanceDiff
	codeChanges     map[common.Address]codeDiff
	prestateNonces  map[common.Address]uint64
	hash            common.Hash
}

// Copy deep-copies the access list
func (b *BlockAccessList) Copy() *BlockAccessList {
	accountAccesses := make(map[common.Address]*accountAccess)
	balanceChanges := make(map[common.Address]balanceDiff)
	codeChanges := make(map[common.Address]codeDiff)

	for addr, aa := range b.accountAccesses {
		accountAccesses[addr] = aa.Copy()
	}
	for addr, bd := range b.balanceChanges {
		balanceChanges[addr] = bd.Copy()
	}
	for addr, cd := range b.codeChanges {
		codeChanges[addr] = cd.Copy()
	}

	return &BlockAccessList{
		accountAccesses,
		balanceChanges,
		codeChanges,
		maps.Clone(b.prestateNonces),
		b.hash,
	}
}

func codeDiffsToEncoderObj(codeChanges map[common.Address]codeDiff) (res encodingCodeDiffs) {
	var codeChangeAddrs []common.Address

	for addr, _ := range codeChanges {
		codeChangeAddrs = append(codeChangeAddrs, addr)
	}
	sort.Slice(codeChangeAddrs, func(i, j int) bool {
		return bytes.Compare(codeChangeAddrs[i][:], codeChangeAddrs[j][:]) < 0
	})

	for _, addr := range codeChangeAddrs {
		res = append(res, encodingAccountCodeDiff{
			addr,
			codeChanges[addr].txIdx,
			bytes.Clone(codeChanges[addr].code),
		})
	}
	return
}

func NewBlockAccessList() *BlockAccessList {
	return &BlockAccessList{
		make(map[common.Address]*accountAccess),
		make(map[common.Address]balanceDiff),
		make(map[common.Address]codeDiff),
		make(map[common.Address]uint64),
		common.Hash{},
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
		if bytes.Compare(codeCh.code, otherCodeCh.code) != 0 {
			return false
		}
		if codeCh.txIdx != otherCodeCh.txIdx {
			return false
		}
	}

	if !maps.Equal(b.prestateNonces, other.prestateNonces) {
		return false
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

// TODO: this should be called once per account per block for every account that sent txs in that block.
// the value is the prestate nonce before the start of the first tx execution from that account in the block.
func (b *BlockAccessList) NonceDiff(address common.Address, originNonce uint64) {
	b.prestateNonces[address] = originNonce
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

func (b *BlockAccessList) toEncoderObj() *encodingBlockAccessList {
	var (
		accountAccessesAddrs   []common.Address
		encoderAccountAccesses encodingAccountAccessList

		balanceDiffsAddrs   []common.Address
		encoderBalanceDiffs encodingBalanceDiffs
	)

	for addr, _ := range b.accountAccesses {
		accountAccessesAddrs = append(accountAccessesAddrs, addr)
	}
	sort.Slice(accountAccessesAddrs, func(i, j int) bool {
		return bytes.Compare(accountAccessesAddrs[i][:], accountAccessesAddrs[j][:]) < 0
	})
	for _, addr := range accountAccessesAddrs {
		encoderAccountAccesses = append(encoderAccountAccesses, encodingAccountAccess{
			Address:  addr,
			Accesses: nil,
			Code:     b.accountAccesses[addr].code,
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
		var accesses []encodingSlotAccess
		for _, accessSlot := range storageAccessKeys {
			accesses = append(accesses, b.accountAccesses[addr].accesses[accessSlot].toEncoderObj(accessSlot))
		}
		encoderAccountAccesses = append(encoderAccountAccesses, encodingAccountAccess{
			Address:  addr,
			Accesses: accesses,
			Code:     b.accountAccesses[addr].code,
		})
	}

	// encode balance diffs
	for addr, _ := range b.balanceChanges {
		balanceDiffsAddrs = append(balanceDiffsAddrs, addr)
	}
	sort.Slice(balanceDiffsAddrs, func(i, j int) bool {
		return bytes.Compare(balanceDiffsAddrs[i][:], balanceDiffsAddrs[j][:]) < 0
	})

	for _, addr := range balanceDiffsAddrs {
		encoderBalanceDiffs = append(encoderBalanceDiffs, b.balanceChanges[addr].toEncoderObj(addr))
	}

	encoderObj := encodingBlockAccessList{
		AccountAccesses: encoderAccountAccesses,
		BalanceDiffs:    encoderBalanceDiffs,
		CodeDiffs:       codeDiffsToEncoderObj(b.codeChanges),
		NonceDiffs:      nonceDiffsToEncoderObj(b.prestateNonces),
	}
	return &encoderObj
}

func (b *BlockAccessList) encodeSSZ() ([]byte, error) {
	encoderObj := b.toEncoderObj()
	dst, err := encoderObj.MarshalSSZTo(nil)
	if err != nil {
		return nil, err
	}
	return dst, nil
}

func (e *encodingBlockAccessList) PrettyPrint() string {
	var res bytes.Buffer
	printWithIndent := func(indent int, text string) {
		fmt.Fprintf(&res, "%s%s\n", strings.Repeat("    ", indent), text)
	}
	fmt.Fprintf(&res, "accounts:\n")
	for _, accountDiff := range e.AccountAccesses {
		printWithIndent(1, fmt.Sprintf("address: %x", accountDiff.Address))
		printWithIndent(1, fmt.Sprintf("code:    %x", accountDiff.Code)) // TODO: code shouldn't be in account accesses (?)

		printWithIndent(1, "slots:")
		for _, slot := range accountDiff.Accesses {
			printWithIndent(2, fmt.Sprintf("%x", slot))
			printWithIndent(2, "accesses:")
			for _, access := range slot.Accesses {
				printWithIndent(3, fmt.Sprintf("idx: %d", access.TxIdx))
				printWithIndent(3, fmt.Sprintf("post: %x", access.ValueAfter))
			}
		}
	}
	printWithIndent(0, "code:")
	for _, codeDiff := range e.CodeDiffs {
		printWithIndent(1, fmt.Sprintf("address: %x", codeDiff.Address))
		printWithIndent(1, fmt.Sprintf("index:   %x", codeDiff.TxIdx))
		printWithIndent(1, fmt.Sprintf("code:    %x", codeDiff.NewCode))
	}
	printWithIndent(0, "balances:")
	for _, b := range e.BalanceDiffs {
		printWithIndent(1, fmt.Sprintf("%x:", b.Address))
		for _, change := range b.Changes {
			printWithIndent(2, fmt.Sprintf("index: %d", change.TxIdx))
			printWithIndent(2, fmt.Sprintf("balance: %s", new(uint256.Int).SetBytes(change.Delta[:]).String()))
		}
	}

	printWithIndent(0, "nonces:")
	for _, n := range e.NonceDiffs {
		printWithIndent(1, fmt.Sprintf("address: %x", n.Address))
		printWithIndent(1, fmt.Sprintf("nonce: %d", n.NonceBefore))
	}

	return res.String()
}

// human-readable representation
func (b *BlockAccessList) PrettyPrint() string {
	enc := b.toEncoderObj()
	return enc.PrettyPrint()
}

func (b *BlockAccessList) Hash() common.Hash {
	if b.hash == (common.Hash{}) {
		// TODO: cache the encoded bal
		encoded, err := b.encodeSSZ()
		if err != nil {
			panic(err)
		}
		b.hash = common.BytesToHash(crypto.Keccak256(encoded))
	}
	return b.hash
}

func (b *BlockAccessList) EncodeRLP(wr io.Writer) error {
	fmt.Printf("encoding bal %v\n", b)
	w := rlp.NewEncoderBuffer(wr)
	buf, err := b.encodeSSZ()
	if err != nil {
		return err
	}
	w.WriteBytes(buf)
	return w.Flush()
}

func (b *BlockAccessList) DecodeRLP(s *rlp.Stream) error {
	var enc encodingBlockAccessList
	encBytes, err := s.Bytes()
	if err != nil {
		return err
	}
	if err := enc.UnmarshalSSZ(encBytes); err != nil {
		return err
	}
	res, err := enc.ToBlockAccessList()
	if err != nil {
		return err
	}
	*b = *res
	return nil
}

var _ rlp.Encoder = &BlockAccessList{}
var _ rlp.Decoder = &BlockAccessList{}
