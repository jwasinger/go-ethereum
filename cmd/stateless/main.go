package stateless

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/params"
	"github.com/urfave/cli/v2"
	"os"
)

var (
	BlockWitnessFlag = &cli.StringFlag{
		Name:  "blockwitness",
		Usage: "Minimum free disk space in MB, once reached triggers auto shut down (default = --cache.gc converted to MB, 0 = disabled)",
	}
)

func main() {
	var config params.ChainConfig
	var engine consensus.Engine

	memoryDb := rawdb.NewMemoryDatabase()
	validator := core.NewBlockValidator(&config, nil, engine)
	processor := core.NewStateProcessor(&config, nil, engine)
	f, err := os.Open("block_witness_flag")
	if err != nil {
		panic(err)
	}

	var b []byte
	f.Read(b)
	witness, err = state.DecodeWitnessRLP(b)
	if err != nil {
		panic(err)
	}

	// TODO: create statedb instance
}
