package core

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ethereum/go-ethereum/core/state"
)

// CrossValidate posts the provided witness to the URL at {endpoint}/verify_block and returns whether the remote
// verification was successful or not.  TODO: differentiate between errors from witness verification (maybe consensus
// failure) and anything else.
func crossValidate(endpoint string, wit *state.Witness) (err error) {
	enc, _ := wit.EncodeRLP()

	// TODO: implement retry if endpoint can't be reached
	p, err := url.JoinPath(endpoint, "verify_block")
	if err != nil {
		return fmt.Errorf("url.JoinPath failed: %v", err)
	}
	resp, err := http.Post(p, "application/octet-stream", bytes.NewBuffer(enc))
	if err != nil {
		return fmt.Errorf("error accessing block verification endpoint: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %v", err)
		}
		return fmt.Errorf("cross-validator bad response code (%d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}
	if bytes.Compare(body, []byte("ok")) != 0 {
		return fmt.Errorf("success response with unexpected body: %s", string(body))
	}
	return nil
}
