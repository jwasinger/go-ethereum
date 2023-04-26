package elhook

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rpc"
)

type EngineAPI struct {
	client *rpc.Client
}

const (
	exchangeCapabilitiesMethod = "engine_exchangeCapabilities"
	forkchoiceUpdatedV2Method = "engine_forkchoiceUpdatedV2"
	getPayloadV2Method = "engine_getPayloadV2"
	getPayloadBodiesByHashV1Method = "engine_getPayloadBodiesByHashV1"
	getPayloadBodiesByRangeV1Method = "engine_getPayloadBodiesByRangeV1"
	newPayloadV1Method = "engine_newPayloadV1"
	forkchoiceUpdatedV1Method = "engine_forkchoiceUpdatedV1"
	getPayloadV1Method = "engine_getPayloadV1"
	exchangeTransitionConfigurationV1 = "engine_exchangeTransitionConfigurationV1"
)

func requiredEngineAPIMethods() map[string]struct{} {
	return map[string]struct{} {
		forkchoiceUpdatedV2Method: struct{}{},
		getPayloadV2Method: struct{}{},
		getPayloadBodiesByHashV1Method: struct{}{},
		getPayloadBodiesByRangeV1Method: struct{}{},
		newPayloadV1Method: struct{}{},
		forkchoiceUpdatedV1Method: struct{}{},
		getPayloadV1Method: struct{}{},
		exchangeTransitionConfigurationV1: struct{}{},
	}
}

func NewWithConnection(ctx context.Context) (*EngineAPI, error) {
	var testSecret = [32]byte{94, 111, 36, 109, 245, 74, 43, 72, 202, 33, 205, 86, 199, 174, 186, 77, 165, 99, 13, 225, 149, 121, 125, 249, 128, 109, 219, 163, 224, 176, 46, 233}
	var testEndpoint = "http://127.0.0.1:8551"

	auth := node.NewJWTAuth(testSecret)
	client, err := rpc.DialOptions(ctx, testEndpoint, rpc.WithHTTPAuth(auth))
	if err != nil {
		fmt.Println("err1")
		fmt.Println(err)
		return nil, err
	}

	return &EngineAPI{client}, nil
}

func (e *EngineAPI) ExchangeCapabilities(ctx context.Context, capsRequested []string) ([]string, error) {
	var result []string

	err := e.client.CallContext(ctx, &result, exchangeCapabilitiesMethod, capsRequested)
	if err != nil {
		return nil, err
	}

	return result, nil
}
