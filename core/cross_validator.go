package core

import (
	"bytes"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/log"
	"io"
	"net/http"
	"net/url"
)

func CrossValidate(endpoint string, wit *state.Witness) (bool, error) {
	enc, err := wit.EncodeRLP()
	if err != nil {
		log.Error("error encoding block witness", "error", err)
	}

	// TODO: post to the endpoint and potentially time-out
	p, err := url.JoinPath(endpoint, "verify_block")
	if err != nil {
		panic(err)
	}
	resp, err := http.Post(p, "application/octet-stream", bytes.NewBuffer(enc))
	if err != nil {
		log.Error("error accessing block verification endpoint", "error", err)
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error("error reading response body", "error", err)
			return false, nil
		}
		log.Error("cross-validator response bad status code", "status", resp.StatusCode, "body", string(body))
		return false, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("error reading response body", "error", err)
		return false, nil
	}

	if bytes.Compare(body, []byte("ok")) == 0 {
		return true, nil
	} else {
		return false, nil
	}
}
