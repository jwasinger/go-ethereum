package main

import (
	"fmt"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
	"io"
	"net/http"
)

func server(ctx *cli.Context) error {
	http.HandleFunc("/verify_block", handleVerifyBlockRequest)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Error("server error", "error", err)
	}
	return nil
}

func handleVerifyBlockRequest(w http.ResponseWriter, r *http.Request) {
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

	correct, err := utils.StatelessVerify(nil, witness)
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
