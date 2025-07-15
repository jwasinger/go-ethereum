package state

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/holiman/uint256"
)

type BALStateReader interface {
	GetState(common.Address, common.Hash) common.Hash
	GetNonce(common.Address) uint64
	GetCode(common.Address) []byte
	GetBalance(common.Address) *uint256.Int
}

type balStateReader struct {
	diff *bal.StateDiff
	db   *StateDB
}

func (s *balStateReader) GetState(address common.Address, slot common.Hash) common.Hash {
	if accountDiff, ok := s.diff.Mutations[address]; ok {
		if value, ok := accountDiff.StorageWrites[slot]; ok {
			return value
		}
	}

	return s.db.GetState(address, slot)
}

func (s *balStateReader) GetNonce(address common.Address) uint64 {
	if accountDiff, ok := s.diff.Mutations[address]; ok {
		if accountDiff.Nonce != nil {
			return *accountDiff.Nonce
		}
	}

	return s.db.GetNonce(address)
}

func (s *balStateReader) GetCode(address common.Address) []byte {
	if accountDiff, ok := s.diff.Mutations[address]; ok {
		if accountDiff.Code != nil {
			return *accountDiff.Code
		}
	}

	return s.db.GetCode(address)
}

func (s *balStateReader) GetBalance(address common.Address) *uint256.Int {
	if accountDiff, ok := s.diff.Mutations[address]; ok {
		if accountDiff.Balance != nil {
			return new(uint256.Int).SetBytes((*accountDiff.Balance)[:])
		}
	}

	return s.db.GetBalance(address)
}

func NewBALStateReader(db *StateDB, diff *bal.StateDiff) BALStateReader {
	return &balStateReader{
		diff,
		db,
	}
}
