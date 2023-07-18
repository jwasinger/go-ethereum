package eth

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

const broadcastWaitTime = 600 * time.Millisecond

type localAccountStatus struct {
	lowestUnsentNonce uint64
	lastBroadcastTime *time.Time
}

// pp implements a 2-level cache keyed by peer network ID and address and mapping to a 
// localAccountStatus that corresponds (if this node had direct-broadcasted local
// transactions to that peer in the past)
type pp struct {
	internal lru.BasicLRU[string, map[common.Address]localAccountStatus]
}

// getLowestUnsentNonce returns the lowest unsent nonce for a peer/addr, creating an entry
// if none previously existed and returning 0.
func (p *pp) getLowestUnsentNonce(peerID string, addr common.Address) uint64 {
	var peerEntry map[common.Address]localAccountStatus

	peerEntry, ok := p.internal.Get(peerID); 
	if !ok {
		return 0
	}

	accountEntry, ok := peerEntry[addr]
	if !ok {
		return 0
	}
	return accountEntry.lowestUnsentNonce
}

// setBroadcastTime sets the latest broadcast time for a peer/addr, creating an entry
// if none previously existed.
func (p *pp) setBroadcastTime(peerID string, addr common.Address, time time.Time) {
	var peerEntry map[common.Address]localAccountStatus
	peerEntry, ok := p.internal.Get(peerID)
	if !ok {
		peerEntry = make(map[common.Address]localAccountStatus)
	}

	accountEntry, ok := peerEntry[addr]
	if !ok {
		accountEntry = localAccountStatus{}
	}

	accountEntry.lastBroadcastTime = &time
	peerEntry[addr] = accountEntry
	p.internal.Add(peerID, peerEntry)
}

// setLastUnsentNonce sets the last unsent nonce for a peer/addr, creating an entry
// if none previously existed.
func (p *pp) setLastUnsentNonce(peerID string, addr common.Address, nonce uint64) {
	var peerEntry map[common.Address]localAccountStatus
	peerEntry, ok := p.internal.Get(peerID)
	if !ok {
		peerEntry = make(map[common.Address]localAccountStatus)
	}

	accountEntry, ok := peerEntry[addr]
	if !ok {
		accountEntry = localAccountStatus{}
	}

	accountEntry.lowestUnsentNonce = nonce
	peerEntry[addr] = accountEntry
	p.internal.Add(peerID, peerEntry)
}

// getBroadcastTime returns the latest broadcast time for a given peer/sender, creating
// an entry if none previously existed and returning nil.
func (p *pp) getBroadcastTime(peer string, addr common.Address) *time.Time {
	var peerEntry map[common.Address]localAccountStatus

	peerEntry, ok := p.internal.Get(peer); 
	if !ok {
		return nil
	}

	accountEntry, ok := peerEntry[addr]
	if !ok {
		return nil
	}
	return accountEntry.lastBroadcastTime
}

// localsTxBroadcaster implements new logic for direct broadcast of local transactions.
// previously they were directly-broadcasted in nonce-order to a square root of the peerset
// who didn't have the transaction.
// 
// now: transactions are broadcast nonce-ordered to a square root of the peerset,
// a delay of 1 second is added between sending consecutive transactions from the
// same account.
type localsTxBroadcaster struct {
	// map of local sender address to a nonce-ordered array of transactions
	accounts map[common.Address][]*types.Transaction
	// cache of a map for every peer with most recent direct broadcast time for a sender account
	peersStatus pp 
	chain *core.BlockChain
	peers *peerSet
	chainHeadCh <-chan core.ChainHeadEvent
	chainHeadSub event.Subscription
	localTxsCh <-chan core.NewTxsEvent
	localTxsSub event.Subscription
	broadcastTrigger *time.Ticker
	signer types.Signer
}

func newLocalsTxBroadcaster(txpool txPool, chain *core.BlockChain, peers *peerSet) *localsTxBroadcaster {
	chainHeadCh := make(chan core.ChainHeadEvent, 10)
	chainHeadSub := chain.SubscribeChainHeadEvent(chainHeadCh)
	localTxsCh := make(chan core.NewTxsEvent, 10)
	localTxsSub := txpool.SubscribeNewLocalTxsEvent(localTxsCh)

	// TODO: this is randomly chosen.  figure out how to choose 
	// proper value based on node configuration
	maxPeers := 16 
	l := localsTxState{
		make(map[common.Address][]*types.Transaction),
		pp{lru.NewBasicLRU[string, map[common.Address]localAccountStatus](maxPeers)},
		chain,
		peers,
		chainHeadCh,
		chainHeadSub,
		localTxsCh,
		localTxsSub,
		time.NewTicker(1 * time.Nanosecond),
		// TODO: this can never be pre-eip155 right?
		types.LatestSigner(chain.Config()),
	}

	<-l.broadcastTrigger.C
	return &l
}

// get the nonce of an account at the head block
func (l *localsTxBroadcaster) GetNonce(addr common.Address) uint64 {
	curState, err := l.chain.State()
	if err != nil {
		// TODO figure out what this could be
		panic(err)
	}
	return curState.GetNonce(addr)
}

// returns whether or not we sent a given peer a local transaction from sender
func (l *localsTxBroadcaster) sentRecently(peerID string, sender common.Address) bool {
	bt := l.peersStatus.getBroadcastTime(peerID, sender)
	if bt != nil && time.Since(*bt) >= broadcastWaitTime + 100 * time.Millisecond {
		return true
	}
	return false
} 

//
func (l *localsTxBroadcaster) nextTxToBroadcast(txs []*types.Transaction, peer *ethPeer, sender common.Address) *common.Hash {
	lowestUnsentNonce := l.peersStatus.getLowestUnsentNonce(peer.ID(), sender)
	for i, tx := range txs {
		if tx.Nonce() > lowestUnsentNonce {
			if !peer.KnownTransaction(tx.Hash()) {
				l.peersStatus.setBroadcastTime(peer.ID(), sender, time.Now())

				if i != len(txs) - 1 {
					// there is a higher nonce transaction on the queue
					// after this one.  reset the timer to ensure it will be sent
					l.broadcastTrigger.Reset(broadcastWaitTime)
				}

				res := tx.Hash()
				return &res
			}
			// we set lowestUnsentNonce even if we weren't the ones sending the transaction to the other node
			l.peersStatus.setLastUnsentNonce(peer.ID(), sender, tx.Nonce() + 1)
		}
	}

	return nil
}

func (l *localsTxBroadcaster) maybeBroadcast() {
	allPeers := l.peers.allEthPeers()
	directBroadcastPeers := allPeers[:int(math.Sqrt(float64(len(allPeers))))]
	for _, peer := range directBroadcastPeers {
		var txsToBroadcast []common.Hash

		for addr, txs := range l.accounts {
			if l.sentRecently(peer.ID(), addr) {
				continue
			}
			if tx := l.nextTxToBroadcast(txs, peer, addr); tx != nil {
				txsToBroadcast = append(txsToBroadcast, *tx)
			}
		}
		if len(txsToBroadcast) > 0 {
			peer.AsyncSendTransactions(txsToBroadcast)
		}
	}
}

func (l *localsTxBroadcaster) trimLocals() {
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

func (l *localsTxBroadcaster) addLocalsFromSender(sender common.Address, txs []*types.Transaction) {
	var curTxs []*types.Transaction

	for _, peer := range l.peers.allEthPeers() {
		l.peersStatus.setLastUnsentNonce(peer.ID(), sender, txs[0].Nonce())
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
		if curIdx > len(curTxs) {
			res = append(res, curTxs[curIdx])
			curIdx++
			continue
		}
		if txsIdx > len(txs) {
			res = append(res, curTxs[txsIdx])
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
func (l *localsTxBroadcaster) addLocals(txs []*types.Transaction) {
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

func (l *localsTxBroadcaster) Stop() {
	l.chainHeadSub.Unsubscribe()
	l.localTxsSub.Unsubscribe()
}

func (l *localsTxBroadcaster) Run(wg *sync.WaitGroup) {
	defer wg.Done()
	l.loop()
}

func (l *localsTxBroadcaster) loop() {
	for {
		select {
		case <-l.chainHeadCh:
			l.trimLocals()
		case evt := <-l.localTxsCh:
			// I assume that these are mostly transactions originating from this same node
			// TODO: explore edge-cases if they aren't
			l.addLocals(evt.Txs)
			l.maybeBroadcast()
		case <-l.broadcastTrigger.C:
			l.maybeBroadcast()
		case <-l.localTxsSub.Err():
			return
		}
	}
}
