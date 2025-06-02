package types

import "github.com/ethereum/go-ethereum/common"

//go:generate go run github.com/ferranbt/fastssz/sszgen --path . --objs PerTxAccess,SlotAccess,AccountAccess,BlockAccessList,BalanceDelta,BalanceChange,AccountBalanceDiff,BalanceDiffs,CodeChange,AccountCodeDiff,CodeDiffs,AccountNonce,NonceDiffs --output bal_encoding.go

type PerTxAccess struct {
	txIdx      uint `ssz-size:"2"`
	valueAfter common.Hash
}

type SlotAccess struct {
	slot     common.Hash
	accesses []PerTxAccess
}

type AccountAccess struct {
	address  common.Address
	accesses []SlotAccess
	code     []byte // this is currently a union in the EIP spec, but unions aren't used anywhere in practice so I implement it as a list here.
}

type BlockAccessList []AccountAccess

type BalanceDelta [12]byte // {}-endian signed integer

type BalanceChange struct {
	txIdx uint64 `ssz-size:"2"`
	delta BalanceDelta
}

type AccountBalanceDiff struct {
	address common.Address
	changes []BalanceChange
}

type BalanceDiffs = []AccountBalanceDiff

type CodeChange struct {
	txIdx   uint64 `ssz-size:"2"`
	newCode []byte
}

type AccountCodeDiff struct {
	address common.Address
	changes []CodeChange
}

type CodeDiffs []AccountCodeDiff

type AccountNonce struct {
	address    common.Address
	nonceAfter uint64
}

type NonceDiffs []AccountNonce
