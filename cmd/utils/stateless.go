package utils

import (
	"fmt"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/eth/tracers/logger"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"io"
	"net"
	"net/http"
	"os"
)

func StatelessVerify(logOutput io.Writer, chainCfg *params.ChainConfig, witness *state.Witness) (success bool, err error) {
	var vmConfig vm.Config
	logconfig := &logger.Config{
		EnableMemory:     false,
		DisableStack:     false,
		DisableStorage:   false,
		EnableReturnData: true,
		Debug:            true,
	}
	tracer := logger.NewJSONLogger(logconfig, os.Stdout)
	_ = tracer
	//vmConfig.Tracer = tracer

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
	//fmt.Printf("stateless used gas is %d\n", usedGas)
	if err := validator.ValidateState(witness.Block, db, receipts, usedGas); err != nil {
		return false, err
	}
	// TODO: differentiate between state-root mismatch (possible consensus failure) and other errors stemming from
	// invalid/malformed witness
	return true, nil
}

func RunLocalServer(port int) (closeChan chan<- struct{}, actualPort int, err error) {
	mux := http.NewServeMux()
	mux.Handle("/verify_block", &verifyHandler{})
	srv := http.Server{Handler: mux}
	listener, err := net.Listen("tcp", ":"+fmt.Sprintf("%d", port))
	if err != nil {
		panic(err)
	}
	actualPort = listener.Addr().(*net.TCPAddr).Port

	go func() {
		if err := srv.Serve(listener); err != nil {
			panic(err)
		}
	}()

	closeCh := make(chan struct{})
	go func() {
		select {
		case <-closeCh:
			if err := srv.Close(); err != nil {
				panic(err)
			}
		}
	}()
	return closeCh, actualPort, nil
}

type verifyHandler struct{}

func (v *verifyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	respError := func(descr string, err error) {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(fmt.Sprintf("%s: %s", descr, err))); err != nil {
			log.Error("write failed", "error", err)
		}

		log.Error("responded with error", "descr", descr, "error", err)
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respError("error reading body", err)
		return
	}
	if len(body) == 0 {
		respError("error", fmt.Errorf("empty body"))
		return
	}
	witness, err := state.DecodeWitnessRLP(body)
	if err != nil {
		respError("error decoding body witness rlp", err)
		return
	}
	defer func() {
		if err := recover(); err != nil {
			errr, _ := err.(error)
			respError("execution error", errr)
			return
		}
	}()

	correct, err := StatelessVerify(nil, params.MainnetChainConfig, witness)
	if err != nil {
		respError("error verifying stateless proof", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	if correct {
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Error("error writing response", "error", err)
		}
	} else {
		if _, err := w.Write([]byte("bad")); err != nil {
			log.Error("error writing response", "error", err)
		}
	}
}
