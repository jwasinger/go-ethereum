package stateless

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
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
	var vmConfig vm.Config

	validator := core.NewBlockValidator(&config, nil, engine)
	processor := core.NewStateProcessor(&config, nil, engine)
	f, err := os.Open("block_witness_flag")
	if err != nil {
		panic(err)
	}

	var b []byte
	f.Read(b)
	block, witness, err := state.DecodeWitnessRLP(b)
	if err != nil {
		panic(err)
	}

	memoryDb := witness.PopulateMemoryDB()
	db, err := state.New(witness.Root(), state.NewDatabase(memoryDb), nil)
	if err != nil {
		panic(err)
	}

	receipts, logs, usedGas, err := processor.ProcessStateless(witness, block, db, vmConfig)
	if err != nil {
		panic(err)
	}

	_ = logs

	if err := validator.ValidateState(block, db, receipts, usedGas); err != nil {
		panic(err)
	}
}
