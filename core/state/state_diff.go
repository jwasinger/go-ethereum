package state

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
)

// func: state_diff(from, to: txIdx)
// input: block w/ txs, BAL
// output: set of state mutations

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
