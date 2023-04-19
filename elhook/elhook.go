package elhook

import (
	"net/http"
	"github.com/ethereum/go-ethereum/rpc"
)

type ClientHook struct {

}

func (c *ClientHook) Connect(ctx *context.Context, httpEndpoint string) error {
	httpEndpoint = "http://127.0.0.1:8545"
	client, err := rpc.DialOptions(ctx, httpEndpoint, gethRPC.WithHTTPClient(&http.Client{
                Timeout: 10 * time.Second,
        }))
	if err != nil {
		return err
	}

/*
	auth := ""
	client.SetHeader("Authorization", auth)
*/

	eth_blockNumber_method := "eth_blockNumber"
	result := {}

	client.CallContext(ctx, &result, eth_blockNumber_method, nil)
}

func (c *ClientHook) Run() {
	for {
		select {
		// newHead:
		// 	if not received from EL client, send forkChoiceUpdated to EL
		// 
		}
	}
}
