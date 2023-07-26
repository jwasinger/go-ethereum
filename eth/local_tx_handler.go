package eth

// TODO: port relevant parts of debug logging previously in eth/handler.go here

import (
	"time"
	"math"
	"sync"
	
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/lru"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

const (
	broadcastWaitTime = 1 * time.Millisecond
	announceWaitTime = 1 * time.Millisecond
)

// localAccountStatus represents the state of local transaction broadcast for a given peer
// and sender Ethereum address
type localAccountStatus struct {
	// TODO: maybe change the below (and associated logic) to
	// "highest" to avoid confusion
	// the lowest nonce that has not had a transaction sent to the peer from us
	lowestUnsentNonce uint64
	// lowest 
	lowestUnannouncedNonce uint64
	// the last time we sent a transaction from this account to the associated peer
	lastBroadcastTime *time.Time
	// the last time we announced a transaction from this account to the associated peer
	lastAnnounceTime *time.Time
}

// peerStateCache implements a 2-level lookup keyed by peer network ID and address and mapping to a 
// localAccountStatus that corresponds (if this node had direct-broadcasted local
// transactions to that peer in the past)
type peerStateCache struct {
	// TODO: should have some eviction mechanism for the addresses in the second-level.
	// e.g. if we haven't broadcasted from some account in a while, we remove each entry
	// for it from the lookup
	internal lru.BasicLRU[string, map[common.Address]localAccountStatus]
}

// TODO: remove "get" as prefix

func (p *peerStateCache) getOrNew(peerID string, addr common.Address) localAccountStatus {
	var peerEntry map[common.Address]localAccountStatus

	peerEntry, ok := p.internal.Get(peerID); 
	if !ok {
		peerEntry = make(map[common.Address]localAccountStatus)
	}

	accountEntry, ok := peerEntry[addr]
	if !ok {
		accountEntry = localAccountStatus{}
		peerEntry[addr] = accountEntry
		p.internal.Add(peerID, peerEntry)
	}
	return accountEntry
}

func (p *peerStateCache) set(peerID string, addr common.Address, l *localAccountStatus) {
	as, _ := p.internal.Get(peerID)
	as[addr] = *l
	p.internal.Add(peerID, as)
}

// getLowestUnsentNonce returns the lowest unsent nonce for a peer/addr, creating an entry
// if none previously existed and returning 0.
func (p *peerStateCache) getLowestUnsentNonce(peerID string, addr common.Address) uint64 {
	accountEntry := p.getOrNew(peerID, addr)
	return accountEntry.lowestUnsentNonce
}

func (p *peerStateCache) getLowestUnannouncedNonce(peerID string, addr common.Address) uint64 {
	accountEntry := p.getOrNew(peerID, addr)
	return accountEntry.lowestUnannouncedNonce
}

// setBroadcastTime sets the latest broadcast time for a peer/addr, creating an entry
// if none previously existed.
func (p *peerStateCache) setLastBroadcastTime(peerID string, addr common.Address, time time.Time) {
	accountEntry := p.getOrNew(peerID, addr)
	accountEntry.lastBroadcastTime = &time
	p.set(peerID, addr, &accountEntry)
}

// setBroadcastTime sets the latest broadcast time for a peer/addr, creating an entry
// if none previously existed.
func (p *peerStateCache) setLastAnnounceTime(peerID string, addr common.Address, time time.Time) {
	accountEntry := p.getOrNew(peerID, addr)
	accountEntry.lastAnnounceTime = &time
	p.set(peerID, addr, &accountEntry)
}

// setLowestUnsentNonce sets the last unsent nonce for a peer/addr, creating an entry
// if none previously existed.
func (p *peerStateCache) setLowestUnsentNonce(peerID string, addr common.Address, nonce uint64) {
	accountEntry := p.getOrNew(peerID, addr)
	accountEntry.lowestUnsentNonce = nonce
	p.set(peerID, addr, &accountEntry)
}

// setLowestUnsentNonce sets the last unsent nonce for a peer/addr, creating an entry
// if none previously existed.
func (p *peerStateCache) setLowestUnannouncedNonce(peerID string, addr common.Address, nonce uint64) {
	accountEntry := p.getOrNew(peerID, addr)
	accountEntry.lowestUnannouncedNonce = nonce
	p.set(peerID, addr, &accountEntry)
}

// getBroadcastTime returns the latest broadcast time for a given peer/sender, creating
// an entry if none previously existed and returning nil.
func (p *peerStateCache) getBroadcastTime(peerID string, addr common.Address) *time.Time {
	accountEntry := p.getOrNew(peerID, addr)
	return accountEntry.lastBroadcastTime
}

// localTxHandler implements new logic for broadcast/announcement of local transactions:
// transactions are broadcast nonce-ordered to a square root of the peerset,
// a delay of 1 second is added between sending consecutive transactions from the
// same account.
type localTxHandler struct {
	// map of local sender address to a nonce-ordered array of transactions
	accounts map[common.Address][]*types.Transaction
	peersStatus peerStateCache
	chain *core.BlockChain
	peers *peerSet
	chainHeadCh <-chan core.ChainHeadEvent
	chainHeadSub event.Subscription
	localTxsCh <-chan core.NewTxsEvent
	localTxsSub event.Subscription
	broadcastTrigger *time.Ticker
	announceTrigger *time.Ticker
	signer types.Signer
}

func newLocalsTxBroadcaster(txpool txPool, chain *core.BlockChain, peers *peerSet) *localTxHandler {
	chainHeadCh := make(chan core.ChainHeadEvent, 10)
	chainHeadSub := chain.SubscribeChainHeadEvent(chainHeadCh)
	localTxsCh := make(chan core.NewTxsEvent, 10)
	localTxsSub := txpool.SubscribeNewLocalTxsEvent(localTxsCh)

	// TODO: this maxPeers is randomly chosen (and generous).
	// figure out how to choose proper value based on node configuration
	maxPeers := 64
	l := localTxHandler{
		make(map[common.Address][]*types.Transaction),
		peerStateCache{lru.NewBasicLRU[string, map[common.Address]localAccountStatus](maxPeers)},
		chain,
		peers,
		chainHeadCh,
		chainHeadSub,
		localTxsCh,
		localTxsSub,
		// use super low default times to exhaust the ticker initially
		time.NewTicker(1 * time.Nanosecond),
		time.NewTicker(1 * time.Nanosecond),
		// TODO: I feel like initiating the signer once with
		// LatestSigner might be problematic but I can't explain why :)
		types.LatestSigner(chain.Config()),
	}

	<-l.broadcastTrigger.C
	<-l.announceTrigger.C
	return &l
}

// get the nonce of an account at the head block
func (l *localTxHandler) GetNonce(addr common.Address) uint64 {
	curState, err := l.chain.State()
	if err != nil {
		// TODO figure out what this could be
		panic(err)
	}
	return curState.GetNonce(addr)
}

// returns whether or not we sent a given peer a local transaction from sender
func (l *localTxHandler) announcedRecently(peerID string, sender common.Address) bool {
	bt := l.peersStatus.getBroadcastTime(peerID, sender)
	if bt != nil && time.Since(*bt) <= broadcastWaitTime + 100 * time.Millisecond {
		return true
	}
	return false
} 

// returns whether or not we sent a given peer a local transaction from sender
func (l *localTxHandler) sentRecently(peerID string, sender common.Address) bool {
	bt := l.peersStatus.getBroadcastTime(peerID, sender)
	if bt != nil && time.Since(*bt) <= broadcastWaitTime + 100 * time.Millisecond {
		return true
	}
	return false
} 

// nextTxToBroadcast retrieves the next unsent transaction from an account with the lowest nonce and returns it.  The internal state
// of localTxHandler is modified to reflect the tx as being sent to the peer.
func (l *localTxHandler) nextTxToBroadcast(peer *ethPeer, sender common.Address) *common.Hash {
	lowestUnsentNonce := l.peersStatus.getLowestUnsentNonce(peer.ID(), sender)
	txs := l.accounts[sender]
	for i, tx := range txs {
		if tx.Nonce() > lowestUnsentNonce {
			// we set lowestUnsentNonce even if we weren't the ones sending the transaction to the other node
			l.peersStatus.setLowestUnsentNonce(peer.ID(), sender, tx.Nonce() + 1)

			if !peer.KnownTransaction(tx.Hash()) {
				l.peersStatus.setLastBroadcastTime(peer.ID(), sender, time.Now())

				if i != len(txs) - 1 {
					// there is a higher nonce transaction on the queue
					// after this one.  reset the timer to ensure it will be sent
					l.broadcastTrigger.Reset(broadcastWaitTime)
				}

				res := tx.Hash()
				return &res
			}
		}
	}

	return nil
}

// 
func (l *localTxHandler) nextTxToAnnounce(peer *ethPeer, sender common.Address) *common.Hash {
	lowestUnannouncedNonce := l.peersStatus.getLowestUnannouncedNonce(peer.ID(), sender)
	txs := l.accounts[sender]
	for i, tx := range txs {
		if tx.Nonce() > lowestUnannouncedNonce {
			// we set lowestUnsentNonce even if we weren't the ones sending the transaction to the other node
			l.peersStatus.setLowestUnannouncedNonce(peer.ID(), sender, tx.Nonce() + 1)
			l.peersStatus.setLastAnnounceTime(peer.ID(), sender, time.Now())

			if i != len(txs) - 1 {
				// there is a higher nonce transaction on the queue
				// after this one.  reset the timer to ensure it will be announced
				l.announceTrigger.Reset(announceWaitTime)
			}

			res := tx.Hash()
			return &res
		}
	}

	return nil
}

// maybeBroadcast sends all transactions to a peer where:
//	1) another transaction from the same sender has not been sent to the peer recently.
//	2) the peer does not already have the transaction
func (l *localTxHandler) maybeBroadcast() {
	allPeers := l.peers.allEthPeers()
	directBroadcastPeers := allPeers[:int(math.Sqrt(float64(len(allPeers))))]
	for _, peer := range directBroadcastPeers {
		var txsToBroadcast []common.Hash

		for addr, _ := range l.accounts {
			if l.sentRecently(peer.ID(), addr) {
				continue
			}
			if tx := l.nextTxToBroadcast(peer, addr); tx != nil {
				txsToBroadcast = append(txsToBroadcast, *tx)
			}
		}
		if len(txsToBroadcast) > 0 {
			peer.AsyncSendTransactions(txsToBroadcast)
		}
	}
}

func (l *localTxHandler) maybeAnnounce() {
	allPeers := l.peers.allEthPeers()
	announcePeers := allPeers[int(math.Sqrt(float64(len(allPeers)))):]
	for _, peer := range announcePeers {
		var txsToAnnounce []common.Hash

		for addr, _ := range l.accounts {
			if l.announcedRecently(peer.ID(), addr) {
				continue
			}
			if tx := l.nextTxToAnnounce(peer, addr); tx != nil {
				txsToAnnounce = append(txsToAnnounce, *tx)
			}
		}
		if len(txsToAnnounce) > 0 {
			peer.AsyncSendPooledTransactionHashes(txsToAnnounce)
		}
	}
}

// trimLocals removes transactions from monitoring queues
// if their nonce falls below the account nonce (stale transactions).
func (l *localTxHandler) trimLocals() {
	for addr, txs := range l.accounts {
		var cutPoint int
		currentNonce := l.GetNonce(addr)
		for i, tx := range txs {
			if tx.Nonce() < currentNonce {
				cutPoint = i
			} else {
				break
			}
		}

		l.accounts[addr] = l.accounts[addr][cutPoint:]

		if len(l.accounts[addr]) == 0 {
			delete(l.accounts, addr)
		}
	}
}

// TODO: randomly noting edge-case/food-for-thought here...:  if we receive replacement transaction(s), we should broadcast them out ASAP
// to increase the chances they get included?

// addLocalsFromSender inserts a list of nonce-ordered transactions into the tracking
// queue for the associated account.
func (l *localTxHandler) addLocalsFromSender(sender common.Address, txs []*types.Transaction) {
	var curTxs []*types.Transaction

	for _, peer := range l.peers.allEthPeers() {
		lastUnsentNonce := l.peersStatus.getLowestUnsentNonce(peer.ID(), sender)
		if txs[0].Nonce() <= lastUnsentNonce {
			// this is either a reorg which re-injected txs into the pool or
			// a known transaction has been replaced
			l.peersStatus.setLowestUnsentNonce(peer.ID(), sender, txs[0].Nonce())
		}
	}

	curTxs, ok := l.accounts[sender]
	if !ok {
		l.accounts[sender] = txs
		return
	}

	// insert txs into the sender's queue, keeping the resulting array nonce-ordered and
	// replacing pre-existing txs if there is a tx with same nonce

	// this code is super gross.  I'm not sure how to improve it atm
	res := types.Transactions{}
	curIdx, txsIdx := 0, 0
	for ; curIdx < len(curTxs) || txsIdx < len(txs) ; {
		if curIdx < len(curTxs) {
			res = append(res, curTxs[curIdx])
			curIdx++
			continue
		}
		if txsIdx < len(txs) {
			res = append(res, txs[txsIdx])
			txsIdx++
			continue
		}

		curTx := l.accounts[sender][curIdx]
		tx := txs[txsIdx]
		if curTx.Nonce() > tx.Nonce() {
			res = append(res, tx)
			txsIdx++
		} else if curTx.Nonce() < tx.Nonce() {
			res = append(res, curTx)
			curIdx++
		} else {
			res = append(res, tx)
			curIdx++
			txsIdx++
		}
	}
	l.accounts[sender] = res
}

// add a set of locals into the tracking queues
// assumes:
//	1) txs is a nonce-ordered, account-grouped list of transactions
//	2) there are no nonce gaps, multiple calls to addLocals pass transactions
//	   with nonces that are contiguous/overlapping with values from previous calls.
func (l *localTxHandler) addLocals(txs []*types.Transaction) {
	acctTxs := make(map[common.Address][]*types.Transaction)
	lastSender := common.Address{}
	for _, tx := range txs {
		sender, _ := types.Sender(l.signer, tx)
		if sender != lastSender {
			acctTxs[sender] = types.Transactions{tx}
		}
		acctTxs[sender] = append(acctTxs[sender], tx)
	}

	for acct, txs := range acctTxs {
		l.addLocalsFromSender(acct, txs)
	}
}

// Stop stops the long-running go-routine event loop for the localTxHandler
func (l *localTxHandler) Stop() {
	l.chainHeadSub.Unsubscribe()
	l.localTxsSub.Unsubscribe()
}

// Run starts the long-running go-routine event loop for the localTxHandler
func (l *localTxHandler) Run(wg *sync.WaitGroup) {
	defer wg.Done()
	l.loop()
}

// loop is a long-running method which manages the life-cycle for the localTxHandler
func (l *localTxHandler) loop() {
	for {
		select {
		case <-l.chainHeadCh:
			l.trimLocals()
		case evt := <-l.localTxsCh:
			l.addLocals(evt.Txs)
			l.maybeBroadcast()
			l.maybeAnnounce()
		case <-l.broadcastTrigger.C:
			l.maybeBroadcast()
		case <-l.announceTrigger.C:
			l.maybeAnnounce()
		case <-l.localTxsSub.Err():
			return
		}
	}
}
