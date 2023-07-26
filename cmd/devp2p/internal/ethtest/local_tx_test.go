package ethtest

import (
	"bufio"
	"context"
	"math/big"
	"crypto/ecdsa"
	"errors"
	"time"
	"os"


	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/utesting"
	"github.com/ethereum/go-ethereum/params"
)

// local_tx_test creates a geth node and establishes a lot of peer connections.
// transactions are created, signed and inserted into the node via RPC.
// tests ensure that local transactions are broadcasted/announced with delays

// b/c this is more of a black-box test, it's probably going to be put into own suite.
// it's easier to use the existing utility code in the eth suite for now

// automatically assume that the engine API is available, assume fixed jwt secret

// load pre-generated test private keys from a file under testdata
func loadAccountKeypairs() []*ecdsa.PrivateKey {
    file, err := os.Open("testdata/pk_list.txt")
    if err != nil {
	panic(err)
    }
    defer file.Close()

    keys := []*ecdsa.PrivateKey{}

    scanner := bufio.NewScanner(file)
    // optionally, resize scanner's capacity for lines over 64K, see next example
    for scanner.Scan() {
        privateKey, err := crypto.HexToECDSA(scanner.Text())
	if err != nil {
		panic(err)
	}
	keys = append(keys, privateKey)
    }
    return keys
}

// check that the tx hashes announce/broadcast were sent from different sender accounts
func (s *Suite) checkUniqueSenders(txHashes []common.Hash, allTxs map[common.Hash]*types.Transaction) bool {
	chainConfig := s.backend.ChainConfig()
	signer := types.LatestSigner(chainConfig)

	foundSenders := make(map[common.Address]struct{})
	for _, txHash := range txHashes {
		sender, _ := types.Sender(signer, allTxs[txHash])
		if _, ok := foundSenders[sender]; ok {
			return false
		}
		foundSenders[sender] = struct{}{}
	}
	return true
}

type peerTxReport struct {
	PeerIdx int
	Hashes []common.Hash
}

// wait for test transactions to be announced or propagated
func (s *Suite) waitForTxs(txSendCh, txHashAnnounceCh chan peerTxReport, allTxs map[common.Hash]*types.Transaction) error {
	timeout := time.NewTimer(10 * time.Second)
	for {
		select {
		case report := <-txSendCh:
			txHashes := report.Hashes
			if !s.checkUniqueSenders(txHashes, allTxs) {
				return errors.New("list of senders for transactions in announce/broadcast should be unique")
			}
			// validate the txs (nonce is the next one we want for the given account, no multiple txs from same acct, there was proper delay)

			// check that delay from last announcement was sufficient
			// add it to tx hashes result map
			// if both result maps are full, return
		case report := <-txHashAnnounceCh:
			txHashes := report.Hashes
			if !s.checkUniqueSenders(txHashes, allTxs) {
				return errors.New("list of senders for transactions in announce/broadcast should be unique")
			}
			// validate the tx hashes (nonce is the next one we want for the given account, no multiple txs from same acct, there was proper delay)
			// check that delay from last announcement was sufficient
			// add it to tx hashes result map
			// if both result maps are full, return
		case <-timeout.C:
			return errors.New("timeout without all txs being announced/broadcasted")
		}
	}
}

func (s *Suite) peerLoop(peerIdx int, txsCh, txHashesCh chan peerTxReport) {
	peerConn, err := s.dial()
	if err != nil {
		panic(err)
	}

	// TODO: what do:
	// defer peerConn.Close()

	if err := peerConn.handshake(); err != nil {
		panic(err)
	}

	if _, err = peerConn.statusExchange(s.chain, nil); err != nil {
		panic(err)
	}

	for {
		switch msg := peerConn.Read().(type) {
		case *Ping:
			// pong (TODO: see how often this should happen)
			panic("no pong!")
		case *NewPooledTransactionHashes:
			hashes := msg.Hashes
			txHashesCh <- peerTxReport{peerIdx, hashes}
		case *Transactions:
			txs := msg
			var hashes []common.Hash
			for _, tx := range *txs {
				hashes = append(hashes, tx.Hash())
			}
			txsCh <- peerTxReport{peerIdx, hashes}
		default:
			return
		}
	}
}

func (s *Suite) generateTestTxs(keys []*ecdsa.PrivateKey) []*types.Transaction {
	var txs []*types.Transaction

	testAddress := common.Address{}
	//chainID := big.NewInt(19763)
	chainConfig := s.backend.ChainConfig()
	signer := types.LatestSigner(chainConfig)

/*
	block, err := s.backend.BlockByNumber(context.Background(), rpc.LatestBlockNumber)
	if err != nil {
		panic(err)
	}

	baseFee := block.BaseFee()
*/

	for _, sk := range keys {
		var nonce uint64

		for nonce = 0; nonce < 50; nonce++ {
/*
			tx := types.MustSignNewTx(sk, signer, &types.DynamicFeeTx{
				ChainID:  chainID,
				Nonce:    nonce,
				To:       &testAddress,
				Value:    big.NewInt(1000),
				Gas:      params.TxGas,
				GasFeeCap: baseFee,
				GasTipCap: baseFee,
			})
*/
			tx := types.MustSignNewTx(sk, signer, &types.LegacyTx{
				Nonce:    nonce,
				To:       &testAddress,
				Value:    big.NewInt(1000),
				Gas:      params.TxGas,
				GasPrice:   big.NewInt(params.InitialBaseFee),
			})
			txs = append(txs, tx)
		}
	}

	return txs
}

func (s *Suite) TestLocalTxBasic(t *utesting.T) {
	txsCh := make(chan peerTxReport)
	txHashesCh := make(chan peerTxReport)

	keys := loadAccountKeypairs()

	numPeers := 3
	for i := 0; i < numPeers; i++ {
		go s.peerLoop(i, txsCh, txHashesCh)
	}

	// insert txs from many local accounts, many txs per account
	testTxs := s.generateTestTxs(keys)

	for _, tx := range testTxs {
		s.backend.SendTx(context.Background(), tx)
	}

	// optional:  make a peer broadcast transactions to us and test remote tx propagation

	expectedTxs := make(map[common.Hash]*types.Transaction)
	for _, tx := range testTxs {
		expectedTxs[tx.Hash()] = tx
	}

	if err := s.waitForTxs(txsCh, txHashesCh, expectedTxs); err != nil {
		panic(err)
	}

	// check that hashes were broadcasted to square root subset of peers

	// ideas (?):
	// check that all accounts that were direct-delivered have the max nonce
	// check that all accounts that were announced have the txHash for the max nonce
}

/*
func (s *Suite) TestLocalTxReplacement(t *utesting.T) {

}
*/
