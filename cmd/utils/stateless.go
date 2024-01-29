package utils

import (
	"io"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
)

func StatelessVerify(logOutput io.Writer, chainCfg *params.ChainConfig, witness *state.Witness) (success bool, err error) {
	var vmConfig vm.Config

	rawDb := rawdb.NewMemoryDatabase()
	if err := witness.PopulateDB(rawDb); err != nil {
		return false, err
	}
	db, err := state.New(witness.Root(), state.NewDatabaseWithConfig(rawDb, trie.PathDefaults), nil)
	if err != nil {
		return false, err
	}
	engine, err := ethconfig.CreateConsensusEngine(chainCfg, rawDb)
	if err != nil {
		return false, err
	}
	validator := core.NewStatelessBlockValidator(chainCfg, engine)
	chainCtx := core.NewStatelessChainContext(rawDb, engine)
	processor := core.NewStatelessStateProcessor(chainCfg, chainCtx, engine)

	receipts, _, usedGas, err := processor.ProcessStateless(witness, witness.Block, db, vmConfig)
	if err != nil {
		return false, err
	}
	if err := validator.ValidateState(witness.Block, db, receipts, usedGas); err != nil {
		return false, err
	}
	// TODO: differentiate between state-root mismatch (possible consensus failure) and other errors stemming from
	// invalid/malformed witness
	return true, nil
}
