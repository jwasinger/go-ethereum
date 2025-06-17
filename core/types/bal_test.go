package types

import (
	"bytes"
	"fmt"
	"github.com/ethereum/go-ethereum/rlp"
	"testing"
)

func TestBALEncoding(t *testing.T) {
	b := NewBlockAccessList()
	var buf bytes.Buffer
	err := b.EncodeRLP(&buf)
	if err != nil {
		t.Fatalf("encoding failed: %v\n", err)
	}
	if err := b.DecodeRLP(rlp.NewStream(bytes.NewReader(buf.Bytes()), 10000000)); err != nil {
		t.Fatalf("decoding failed: %v\n", err)
	}
}

func TestBALBlockEncoding(t *testing.T) {
	header := Header{}
	bal := NewBlockAccessList()
	block := NewBlock(&header, &Body{nil, nil, nil, bal}, nil, nil)
	buf, err := rlp.EncodeToBytes(block)
	if err != nil {
		t.Fatalf("encoding failed: %v\n", err)
	}
	fmt.Printf("buf is %x\n", buf)
	if err := block.DecodeRLP(rlp.NewStream(bytes.NewReader(buf), 10000000)); err != nil {
		t.Fatalf("decoding failed: %v\n", err)
	}
}
