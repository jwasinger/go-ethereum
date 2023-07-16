package eth

import (
	"time"
	"math"
	"sync"
	
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

const (
	// TODO: weird to have two values here...
	// reason is that resetting broadcastElapseTimer
	// will trigger a wait, and I need that wait to
	// last long enough to make sentRecently return false
	// for sure
	broadcastWaitTime = 500 * time.Millisecond
	minSendTime = 600 * time.Millisecond
)

type localsTxState struct {
	// map of sender account -> nonce-ordered list of transactions
	accounts map[common.Address][]*types.Transaction
	// map of locals address to the timestamp that it was last broadcasted
	accountsBcast map[string]map[common.Address]time.Time
	handler *handler
	chainHeadCh <-chan core.ChainHeadEvent
	chainHeadSub event.Subscription
	localTxsCh <-chan core.NewTxsEvent
	localTxsSub event.Subscription
	broadcastElapseTimer *time.Timer
	signer types.Signer
}

func newLocalsTxState(handler *handler) *localsTxState {
	chainHeadCh := make(chan core.ChainHeadEvent, 10)
	chainHeadSub := handler.chain.SubscribeChainHeadEvent(chainHeadCh)
	localTxsCh := make(chan core.NewTxsEvent, 10)
	localTxsSub := handler.txpool.SubscribeNewLocalTxsEvent(localTxsCh)
	l := localsTxState{
		make(map[common.Address][]*types.Transaction),
		make(map[string]map[common.Address]time.Time),
		handler,
		chainHeadCh,
		chainHeadSub,
		localTxsCh,
		localTxsSub,
		nil,
		// TODO: this can never be pre-eip155 right?
		types.LatestSigner(handler.chain.Config()),
	}

	return &l
}

// get the nonce of an account at the head block
func (l *localsTxState) GetNonce(addr common.Address) uint64 {
	curState, err := l.handler.chain.State()
	if err != nil {
		// TODO figure out what this could be
		panic(err)
	}
	return curState.GetNonce(addr)
}

// returns whether or not we sent a given peer a local transaction from sender
func (l *localsTxState) sentRecently(peerID string, sender common.Address) bool {
	if time.Since(l.accountsBcast[peerID][sender]) >= minSendTime {
		return true
	}
	return false
} 

func (l *localsTxState) maybeBroadcast() {
	allPeers := l.handler.peers.allEthPeers()
	directBroadcastPeers := allPeers[:int(math.Sqrt(float64(len(allPeers))))]
	for _, peer := range directBroadcastPeers {
		var txsToBroadcast []common.Hash
		// TODO: cache accounts modified by newLocalTxs so that we
		// don't iterate the entire account set here
		for addr, txs := range l.accounts {
			if l.sentRecently(peer.ID(), addr) {
				continue
			}

			for i, tx := range txs {
				if peer.KnownTransaction(tx.Hash()) {
					continue
				}
				
				if i != len(txs) - 1 {
					// there is a higher nonce transaction on the queue
					// after this one.  reset the timer to ensure it will be sent
					l.broadcastElapseTimer.Reset(broadcastWaitTime)
				}

				txsToBroadcast = append(txsToBroadcast, tx.Hash())
				// TODO: use LRU cache so we don't need sig recovery
				from, _ := types.Sender(l.signer, tx)
				l.accountsBcast[peer.ID()][from] = time.Now()
				break
			}
		}
		if len(txsToBroadcast) > 0 {
			peer.AsyncSendTransactions(txsToBroadcast)
		}
	}
}

func (l *localsTxState) deleteAccount(addr common.Address) {
	delete(l.accounts, addr)
	for peer, _ := range l.accountsBcast {
		if _, ok := l.accountsBcast[peer][addr]; ok {
			delete(l.accountsBcast[peer], addr)
		}
	}
}

func (l *localsTxState) trimLocals() {
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
			l.deleteAccount(addr)
		}
	}
}

func (l *localsTxState) insertLocals(sender common.Address, txs []*types.Transaction) {
	var (
		curTxs []*types.Transaction
	)

	curTxs, ok := l.accounts[sender]
	if !ok {
		l.accounts[sender] = txs
		return
	}

	// insert txs into the sender's queue, keeping the resulting array nonce-ordered and
	// replacing pre-existing txs if there is a tx with same nonce

	// this code is super gross.  I'm not sure how to improve it atm
	res := []*types.Transaction{}
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

// TODO combine this function with insertLocals for clarity
func (l *localsTxState) addLocals(txs []*types.Transaction) {
	// we assume that these are a flattened list of account-ordered, then nonce-ordered txs

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
		l.insertLocals(acct, txs)
	}
}

func (l *localsTxState) loop(wg *sync.WaitGroup) {
	defer l.chainHeadSub.Unsubscribe()
	defer l.localTxsSub.Unsubscribe()
	defer wg.Done()

	for {
		select {
		case <-l.chainHeadCh:
			l.trimLocals()
		case evt := <-l.localTxsCh:
			// I assume that these are mostly transactions originating from this same node
			// TODO: explore edge-cases if they aren't
			l.addLocals(evt.Txs)
			l.maybeBroadcast()
		case <-l.broadcastElapseTimer.C:
			l.maybeBroadcast()
		case <-l.localTxsSub.Err():
			return
		}
	}
}
