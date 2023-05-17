package clmock

import (
	"context"
	"fmt"
	"time"

	"net/http"
	"github.com/ethereum/go-ethereum/rpc"
)

type engineAPI struct {
	client *rpc.Client
}

func (e *engineAPI) Connect(ctx context.Context, httpEndpoint string) error {
	httpEndpoint = "http://127.0.0.1:8545"
	client, err := rpc.DialOptions(ctx, httpEndpoint, rpc.WithHTTPClient(&http.Client{
                Timeout: 10 * time.Second,
        }))

	if err != nil {
		return err
	}

	e.client = client
}

func (e *engineAPI) ForkchoiceUpdatedV1(fcState engine.ForkChoiceStateV1, payloadAttr engine.PayloadAttributes) (*ForkChoiceState, error) {
	var resp engine.ForkChoiceResponse
	if err := client.CallContext(ctx, &resp, "engine_forkchoiceUpdatedV1", fcState, payloadAttr); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (e *engineAPI) GetPayloadV1(id engine.PayloadID) (*engine.ExecutionPayloadBodyV1, error) {
	var res engine.ExecutionPayloadBodyV1
	if err := client.CallContext(ctx, &res, "engine_getPayloadV1", id); err != nil {
		return nil, err
	}

	return res, nil
}

func (e* engineAPI) NewPayloadV1(payload *engine.ExecutionPayloadBodyV1) error {
	if err := client.CallContext(ctx, &res, "engine_newPayloadV1", payload); err != nil {
		return err
	}
	return nil
}
