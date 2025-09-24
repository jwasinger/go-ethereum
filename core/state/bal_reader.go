package state

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"maps"
	"sync"
	"time"
)

// storageOriginResolver is used for async loading the origin of modified storage
// slots which is necessary for committing the state update to pathdb (TODO: why?)
// TODO: probably will need to resolve code also
type storageOriginResolver struct {
	originSet map[common.Address]map[common.Hash]common.Hash
	ctx       context.Context
	cancel    func()
}

func (s *storageOriginResolver) resolve(r Reader, storageSet map[common.Address]map[common.Hash]struct{}) {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go func() {
		defer s.cancel()
		// resolve everything sequentially: these values were already loaded as
		// part of execution, so it's fine to assume that they will still be in
		// the cache at this point.
		for addr, tasks := range storageSet {
			for slot, _ := range tasks {
				val, err := r.Storage(addr, slot)
				if err != nil {
					panic("TODO: wat do here")
				}
				s.originSet[addr][slot] = val
			}
		}

		// resolve codes here too?
	}()
}

// TODO: probably unnecessary to cache the resolved state object here as it will already be in the db cache?
// ^ experiment with the performance of keeping this as-is vs just using the db cache.
type prestateStateObjectResolver struct {
	inProgress map[common.Address]chan struct{}
	resolved   sync.Map
	ctx        context.Context
	cancel     func()
}

// resolve loads the state accounts corresponding to the given addresses
// asynchronously.
func (p *prestateStateObjectResolver) resolve(r Reader, addrs []common.Address) {
	p.inProgress = make(map[common.Address]chan struct{})
	p.ctx, p.cancel = context.WithCancel(context.Background())

	for _, addr := range addrs {
		p.inProgress[addr] = make(chan struct{})
	}

	for _, addr := range addrs {
		resolveAddr := addr
		go func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}

			acct, err := r.Account(resolveAddr)
			if err != nil {
				// TODO: what do here?
			}
			p.resolved.Store(resolveAddr, acct)
			close(p.inProgress[resolveAddr])
		}()
	}
}

func (p *prestateStateObjectResolver) stop() {
	p.cancel()
}

// account loads the state account corresponding to the given address.  If the
// state account was never scheduled for retrieval or is non-existent, nil is returned.
func (p *prestateStateObjectResolver) account(addr common.Address) *types.StateAccount {
	if _, ok := p.inProgress[addr]; !ok {
		return nil
	}

	select {
	case <-p.inProgress[addr]:
	}
	res, exist := p.resolved.Load(addr)
	if !exist {
		return nil
	}
	return res.(*types.StateAccount)
}

func (r *BALReader) initObjFromDiff(db *StateDB, addr common.Address, a *types.StateAccount, diff *bal.AccountState) *stateObject {
	var acct *types.StateAccount
	if a == nil {
		acct = &types.StateAccount{
			Nonce:    0,
			Balance:  uint256.NewInt(0),
			Root:     types.EmptyRootHash,
			CodeHash: types.EmptyCodeHash[:],
		}
	} else {
		acct = a.Copy()
	}
	if diff == nil {
		return newObject(db, addr, acct)
	}

	if diff.Nonce != nil {
		acct.Nonce = *diff.Nonce
	}
	if diff.Balance != nil {
		acct.Balance = new(uint256.Int).Set(diff.Balance)
	}
	obj := newObject(db, addr, acct)
	if diff.Code != nil {
		obj.setCode(crypto.Keccak256Hash(diff.Code), diff.Code)
	}
	if diff.StorageWrites != nil {
		for key, val := range diff.StorageWrites {
			obj.pendingStorage[key] = val
		}
	}
	if obj.empty() {
		return nil
	}
	return obj
}

func (s *BALReader) initMutatedObjFromDiff(db *StateDB, addr common.Address, a *types.StateAccount, diff *bal.AccountState) *stateObject {
	var acct *types.StateAccount
	if a == nil {
		acct = &types.StateAccount{
			Nonce:    0,
			Balance:  uint256.NewInt(0),
			Root:     types.EmptyRootHash,
			CodeHash: types.EmptyCodeHash[:],
		}
	} else {
		acct = a.Copy()
	}
	obj := newObject(db, addr, acct)
	if diff.Nonce != nil {
		obj.SetNonce(*diff.Nonce)
	}
	if diff.Balance != nil {
		obj.SetBalance(new(uint256.Int).Set(diff.Balance))
	}
	if diff.Code != nil {
		obj.SetCode(crypto.Keccak256Hash(diff.Code), diff.Code)
	}
	if diff.StorageWrites != nil {
		for key, val := range diff.StorageWrites {
			obj.SetState(key, val)
		}
	}
	return obj
}

var IgnoredBALAddresses map[common.Address]struct{} = map[common.Address]struct{}{
	params.SystemAddress: {},
}

// BALReader provides methods for reading account state from a block access
// list.  State values returned from the Reader methods must not be modified.
type BALReader struct {
	block          *types.Block
	accesses       map[common.Address]*bal.AccountAccess
	prestateReader prestateStateObjectResolver
}

// NewBALReader constructs a new reader from an access list. db is expected to have been instantiated with a reader.
func NewBALReader(block *types.Block, db *StateDB) *BALReader {
	r := &BALReader{accesses: make(map[common.Address]*bal.AccountAccess), block: block}
	for _, acctDiff := range *block.Body().AccessList {
		r.accesses[acctDiff.Address] = &acctDiff
	}
	r.prestateReader.resolve(db.Reader(), r.ModifiedAccounts())
	return r
}

// ModifiedAccounts returns a list of all accounts with mutations in the access list
func (r *BALReader) ModifiedAccounts() (res []common.Address) {
	for addr, access := range r.accesses {
		if len(access.NonceChanges) != 0 || len(access.CodeChanges) != 0 || len(access.StorageChanges) != 0 || len(access.BalanceChanges) != 0 {
			res = append(res, addr)
		}
	}
	return res
}

func (r *BALReader) ValidateStateReads(allReads bal.StateAccesses) error {
	// 1. remove any slots from 'allReads' which were written
	// 2. validate that the read set in the BAL matches 'allReads' exactly
	for addr, reads := range allReads {
		balAcctDiff := r.readAccountDiff(addr, len(r.block.Transactions())+2)
		if balAcctDiff != nil {
			for writeSlot := range balAcctDiff.StorageWrites {
				delete(reads, writeSlot)
			}
		}

		expectedReads := r.accesses[addr].StorageReads
		if len(reads) != len(expectedReads) {
			return fmt.Errorf("mismatch between the number of computed reads and number of expected reads")
		}

		for _, slot := range expectedReads {
			if _, ok := reads[slot]; !ok {
				return fmt.Errorf("expected read is missing from BAL")
			}
		}
	}

	return nil
}

func (r *BALReader) AccessedState() (res map[common.Address]map[common.Hash]struct{}) {
	res = make(map[common.Address]map[common.Hash]struct{})
	for addr, accesses := range r.accesses {
		if len(accesses.StorageReads) > 0 {
			res[addr] = make(map[common.Hash]struct{})
			for _, slot := range accesses.StorageReads {
				res[addr][slot] = struct{}{}
			}
		} else if len(accesses.BalanceChanges) == 0 && len(accesses.NonceChanges) == 0 && len(accesses.StorageChanges) == 0 && len(accesses.CodeChanges) == 0 {
			res[addr] = make(map[common.Hash]struct{})
		}
	}
	return
}

// TODO: it feels weird that this modifies the prestate instance. However, it's needed because it will
// subsequently be used in Commit.
func (r *BALReader) StateRoot(prestate *StateDB) (root common.Hash, prestateLoadTime time.Duration, rootUpdateTime time.Duration) {
	lastIdx := len(r.block.Transactions()) + 1
	modifiedAccts := r.ModifiedAccounts()
	startPrestateLoad := time.Now()
	for _, addr := range modifiedAccts {
		diff := r.readAccountDiff(addr, lastIdx)
		acct := r.prestateReader.account(addr)
		obj := r.initMutatedObjFromDiff(prestate, addr, acct, diff)
		if obj != nil {
			prestate.setStateObject(obj)
		}
	}
	prestateLoadTime = time.Since(startPrestateLoad)
	rootUpdateStart := time.Now()
	root = prestate.IntermediateRoot(true)
	rootUpdateTime = time.Since(rootUpdateStart)
	return root, prestateLoadTime, rootUpdateTime
}

// changesAt returns all state changes at the given index.
func (r *BALReader) changesAt(idx int) *bal.StateDiff {
	res := &bal.StateDiff{make(map[common.Address]*bal.AccountState)}
	for addr, _ := range r.accesses {
		accountChanges := r.accountChangesAt(addr, idx)
		if accountChanges != nil {
			res.Mutations[addr] = accountChanges
		}
	}
	return res
}

// accountChangesAt returns the state changes of an account at a given index,
// or nil if there are no changes.
func (r *BALReader) accountChangesAt(addr common.Address, idx int) *bal.AccountState {
	acct, exist := r.accesses[addr]
	if !exist {
		return nil
	}

	var res bal.AccountState

	for i := len(acct.BalanceChanges) - 1; i >= 0; i-- {
		if acct.BalanceChanges[i].TxIdx == uint16(idx) {
			res.Balance = acct.BalanceChanges[i].Balance
		}
		if acct.BalanceChanges[i].TxIdx < uint16(idx) {
			break
		}
	}

	for i := len(acct.CodeChanges) - 1; i >= 0; i-- {
		if acct.CodeChanges[i].TxIdx == uint16(idx) {
			res.Code = acct.CodeChanges[i].Code
			break
		}
		if acct.CodeChanges[i].TxIdx < uint16(idx) {
			break
		}
	}

	for i := len(acct.NonceChanges) - 1; i >= 0; i-- {
		if acct.NonceChanges[i].TxIdx == uint16(idx) {
			res.Nonce = &acct.NonceChanges[i].Nonce
			break
		}
		if acct.NonceChanges[i].TxIdx < uint16(idx) {
			break
		}
	}

	for i := len(acct.StorageChanges) - 1; i >= 0; i-- {
		if res.StorageWrites == nil {
			res.StorageWrites = make(map[common.Hash]common.Hash)
		}
		slotWrites := acct.StorageChanges[i]

		for j := len(slotWrites.Accesses) - 1; j >= 0; j-- {
			if slotWrites.Accesses[j].TxIdx == uint16(idx) {
				res.StorageWrites[slotWrites.Slot] = slotWrites.Accesses[j].ValueAfter
				break
			}
			if slotWrites.Accesses[j].TxIdx < uint16(idx) {
				break
			}
		}
		if len(res.StorageWrites) == 0 {
			res.StorageWrites = nil
		}
	}

	if res.Code == nil && res.Nonce == nil && len(res.StorageWrites) == 0 && res.Balance == nil {
		return nil
	}
	return &res
}

func (r *BALReader) isModified(addr common.Address) bool {
	access, ok := r.accesses[addr]
	if !ok {
		return false
	}
	return len(access.StorageChanges) > 0 || len(access.BalanceChanges) > 0 || len(access.CodeChanges) > 0 || len(access.NonceChanges) > 0
}

func (r *BALReader) readStateObject(db *StateDB, addr common.Address, idx int) *stateObject {
	diff := r.readAccountDiff(addr, idx)
	prestate := r.prestateReader.account(addr)
	return r.initObjFromDiff(db, addr, prestate, diff)
}

// accountState is a slimmed-down version of stateObject.  It represents the post
// state of an account, omitting unmodified storage keys.
//
// it is used as input for the trie update
//
// TODO: rename this to reflect that it's the account state of a modified account used as input to the state root update procedure...
type accountState struct {
	address       common.Address
	nonce         uint64
	balance       *uint256.Int
	codeHash      common.Hash
	code          []byte
	storageWrites map[common.Hash]common.Hash

	mutated bool
}

func newAccountState(addr common.Address, prestate *types.StateAccount, diff *bal.AccountState) *accountState {
	var a accountState
	a.address = addr
	a.nonce = prestate.Nonce
	a.codeHash = common.BytesToHash(prestate.CodeHash)
	a.balance = prestate.Balance.Clone()

	if diff.Nonce != nil && *diff.Nonce != prestate.Nonce {
		a.nonce = *diff.Nonce
		a.mutated = true
	}

	if diff.Balance != nil && !diff.Balance.Eq(prestate.Balance) {
		a.balance = diff.Balance.Clone()
		a.mutated = true
	}

	if diff.Code != nil {
		diffCodeHash := crypto.Keccak256Hash(diff.Code)
		if diffCodeHash != common.BytesToHash(prestate.CodeHash) {
			a.codeHash = diffCodeHash
			a.code = bytes.Clone(diff.Code)
			a.mutated = true
		}
	}

	if len(diff.StorageWrites) > 0 {
		// ideally, we would only include storage writes which are known
		// to be different from the prestate, to avoid no-op trie lookups/hashing.
		//
		// However, it feels like the overhead of fetching the prestate of all
		// mutated storage just to occasionally avoid a write isn't worth it.
		//
		// ofc, we will fetch the prestate as part of executing the transaction that mutated
		// that slot, but we don't necessarily want to wait on that here (?)
		a.storageWrites = maps.Clone(diff.StorageWrites)
		a.mutated = true
	}

	return &a
}

func (a *accountState) isDeleted() bool {
	return a.code == nil && a.balance == nil && a.nonce == 0 && len(a.storageWrites) == 0
}

// readAccountPostState returns the post-state of a modified account, or nil
// if that account was not modified.
func (r *BALReader) readAccountPostState(addr common.Address) *accountState {
	lastIdx := len(r.block.Transactions()) + 1
	diff := r.readAccountDiff(addr, lastIdx)
	prestate := r.prestateReader.account(addr)
	return newAccountState(addr, prestate, diff)
}

// readAccountDiff returns the accumulated state changes of an account up through idx.
func (r *BALReader) readAccountDiff(addr common.Address, idx int) *bal.AccountState {
	diff, exist := r.accesses[addr]
	if !exist {
		return nil
	}

	var res bal.AccountState

	for i := 0; i < len(diff.BalanceChanges) && diff.BalanceChanges[i].TxIdx <= uint16(idx); i++ {
		res.Balance = diff.BalanceChanges[i].Balance
	}

	for i := 0; i < len(diff.CodeChanges) && diff.CodeChanges[i].TxIdx <= uint16(idx); i++ {
		res.Code = diff.CodeChanges[i].Code
	}

	for i := 0; i < len(diff.NonceChanges) && diff.NonceChanges[i].TxIdx <= uint16(idx); i++ {
		res.Nonce = &diff.NonceChanges[i].Nonce
	}

	if len(diff.StorageChanges) > 0 {
		res.StorageWrites = make(map[common.Hash]common.Hash)
		for _, slotWrites := range diff.StorageChanges {
			for i := 0; i < len(slotWrites.Accesses) && slotWrites.Accesses[i].TxIdx <= uint16(idx); i++ {
				res.StorageWrites[slotWrites.Slot] = slotWrites.Accesses[i].ValueAfter
			}
		}
	}

	return &res
}

// ValidateStateDiff returns an error if the computed state diff is not equal to
// diff reported from the access list at the given index.
func (r *BALReader) ValidateStateDiff(idx int, computedDiff *bal.StateDiff) error {
	balChanges := r.changesAt(idx)
	for addr, state := range balChanges.Mutations {
		computedAccountDiff, ok := computedDiff.Mutations[addr]
		if !ok {
			//fmt.Printf("bal changes:\n%s\ncomputed changes:\n%s\n", r.block.Body().AccessList.String(), computedDiff.String())
			return fmt.Errorf("BAL change not reported in computed")
		}

		if !state.Eq(computedAccountDiff) {
			return fmt.Errorf("unequal")
		}
	}

	if len(balChanges.Mutations) != len(computedDiff.Mutations) {
		fmt.Printf("tx idx %d. computed:\n%s\nbal:\n%s\n", idx, computedDiff.String(), r.block.Body().AccessList.String())
		return fmt.Errorf("computed diff contained additional mutations compared to BAL")
	}

	return nil
}
