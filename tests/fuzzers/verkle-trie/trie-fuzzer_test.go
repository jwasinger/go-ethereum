package verkle_trie

import (
	"testing"
)

func TestExecuteFuzzerInput(t *testing.T) {
	crasher := []byte("asasdlkfjalwk4jfalsdkfjawlefkjsadlfkjasldkfjwalefkjasdlfkjM4fe342e2be1a7f9f8ee7eDangling pa")
	Fuzz(crasher)
}
