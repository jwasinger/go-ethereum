package state

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/trie"
	"golang.org/x/sync/errgroup"
)

type accountTrieUpdate struct{}

func (a *accountTrieUpdate) Start() {}

// TODO: Resolve needs to somehow make the update nodeset available too
func (a *accountTrieUpdate) Resolve() *types.StateAccount {

}

// computeTrieRoot computes the new state trie root and returns the trie objects containing the updated nodesets
func computeTrieRoot(reader *BALReader) map[common.Hash]*trie.Trie {
	var workers errgroup.Group

	modifiedAccounts := reader.ModifiedAccounts()
	if len(modifiedAccounts) == 0 {
		// block without state change can't happen in practice since clique
		// removal.  but cover this case anyways for completeness
		return reader.block.ParentHash()
	}

	acctCh := make(chan *accountState)
	for _, addr := range modifiedAccounts {
		// account updates
		acctAddr := addr
		workers.Go(func() error {
			// resolve the prestate of the account
			acct := reader.readAccountPostState(acctAddr)

			// TODO: we will need the origin for all mutated storage keys, but we can do this async, because the origin
			// is only needed after the state root calculation during commit (TODO: I'm not exactly sure what pathdb does
			// with it.  presumably use for reverse-diff calculation).

			// TODO: we can signal to the state trie update that this account will be updated
			// and it can start loading the intermediate nodes for this account from disk simultaneous
			// to the updating of this account's storage trie.

			// if there were changes to the account trie, update it.
			if len(acct.storageWrites) > 0 {

			}

			// TODO: package the account update as a "resolvable" thingy that gets forwarded to the
			// main state trie updater.  Allow for the account trie hash to be computed while the
			// intermediate nodes are being fetched for that account in the main state trie.

			// TODO: I assume that a deleted account can never have storage writes, need to verify that this is the case
			acctCh <- acct
			return nil
		})
	}

	// main trie update:
	// * start updating each account as soon as the worker signals that the
	// account trie update is complete.
	for i := 0; i < len(modifiedAccounts); i++ {
		acct := <-acctCh

		// if there are no updates to the acct, continue
	}
}

// computeStateUpdate creates a stateUpdate object from the updated tries
// and resolves any other prestate values that are needed as part of the state update.
func computeStateUpdate(r *BALReader, tries map[common.Hash]*trie.Trie) *stateUpdate {
	return nil
}

// commitStateUpdate flushes the stateUpdate to the database
func commitStateUpdate(block uint64, update *stateUpdate, db Database) error {
	// Commit dirty contract code if any exists
	if db := db.TrieDB().Disk(); db != nil && len(update.codes) > 0 {
		batch := db.NewBatch()
		for _, code := range update.codes {
			rawdb.WriteCode(batch, code.hash, code.blob)
		}
		if err := batch.Write(); err != nil {
			return err
		}
	}
	if !update.empty() {
		// If snapshotting is enabled, update the snapshot tree with this new version
		if snap := db.Snapshot(); snap != nil && snap.Snapshot(update.originRoot) != nil {
			//start := time.Now()
			if err := snap.Update(update.root, update.originRoot, update.accounts, update.storages); err != nil {
				log.Warn("Failed to update snapshot tree", "from", update.originRoot, "to", update.root, "err", err)
			}
			// Keep 128 diff layers in the memory, persistent layer is 129th.
			// - head layer is paired with HEAD state
			// - head-1 layer is paired with HEAD-1 state
			// - head-127 layer(bottom-most diff layer) is paired with HEAD-127 state
			if err := snap.Cap(update.root, TriesInMemory); err != nil {
				log.Warn("Failed to cap snapshot tree", "root", update.root, "layers", TriesInMemory, "err", err)
			}

			// TODO: preserve this metric
			// s.SnapshotCommits += time.Since(start)
		}
		// If trie database is enabled, commit the state update as a new layer
		if db := db.TrieDB(); db != nil {
			//start := time.Now()
			if err := db.Update(update.root, update.originRoot, block, update.nodes, update.stateSet()); err != nil {
				return err
			}
			// TODO: preserve this metric
			//s.TrieDBCommits += time.Since(start)
		}
	}

	return nil
}
