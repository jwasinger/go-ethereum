package state

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/holiman/uint256"
)

// func: state_diff(from, to: txIdx)
// input: block w/ txs, BAL
// output: set of state mutations

func preBlockStateDiff() {
	// calculate the diff from applying the
}

func postBlockStateDiff() {

}

// TODO: the bal iteration has two uses: create state diffs for parallel exec and state root calculation
// and to perform per-tx BAL verification.  The latter might be able to not instantiate a state diff for each tx increment.
func txStateDiff(db *StateDB, txIdx int, sender common.Address, tx *types.Transaction, balIt bal.BALIterator) *bal.StateDiff {
	diff := balIt.Iterate(uint16(txIdx) + 1)
	/*
		additional state changes not present in BAL:
		1) tx sender nonce increment
		2) 7702 delegations: code change and authority nonce bump
	*/

	// TODO: determine whether the delegation will succeed/fail based on account funds and adjust the mutation accordingly
	for _, auth := range tx.SetCodeAuthorizations() {
		targetCode := db.GetCode(auth.Address)
		authNonce := auth.Nonce
		if mut, ok := diff.Mutations[sender]; ok {
			mut.Code = &targetCode
			mut.Nonce = &authNonce
		} else {
			diff.Mutations[sender] = &bal.AccountState{
				Nonce: &authNonce,
				Code:  &targetCode,
			}
		}
	}

	txNonce := tx.Nonce()
	if mut, ok := diff.Mutations[sender]; ok {
		mut.Nonce = &txNonce
	} else {
		diff.Mutations[sender] = &bal.AccountState{
			Nonce: &txNonce,
		}
	}

	return diff
}

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
