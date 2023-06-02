package clmock

import (
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"
)

func startEthService(t *testing.T) (*node.Node, *eth.Ethereum, *clmock.CLMock) {
	t.Helper()

	n, err := node.New(&node.Config{
		ListenAddr: "127.0.0.1:8545",
		NoDiscovery: true,
		MaxPeers: 0})
	if err != nil {
		t.Fatal("can't create node:", err)
	}

	config := *params.AllDevProtocolChanges
	engine := consensus.Engine(beaconConsensus.NewFaker())

	genesis := core.DeveloperGenesis(period, gasLimit, faucetAccount)
	ethcfg := &ethconfig.Config{Genesis: genesis, SyncMode: downloader.FullSync, TrieTimeout: time.Minute, TrieDirtyCache: 256, TrieCleanCache: 256}
	ethservice, err := eth.New(n, ethcfg)
	if err != nil {
		t.Fatal("can't create eth service:", err)
	}

	clmock := NewCLMock(n, ethservice)

	n.RegisterLifeCycle(clmock)

	if err := n.Start(); err != nil {
		t.Fatal("can't start node:", err)
	}

	ethservice.SetEtherbase(testAddr)
	ethservice.SetSynced()
	return n, ethservice, clmock
}

func TestCLMock(t *testing.T) {
	node, ethService, mock := startEthService(t)

	// test case: add a transaction and poll until a block is created

	// test case: very large gas limit, transaction that takes >50ms to execute.  ensure it is included
}
