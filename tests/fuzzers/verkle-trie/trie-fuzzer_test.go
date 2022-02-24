package verkle_trie

import (
	"testing"
)

func TestExecuteFuzzerInput(t *testing.T) {
	crasher := []byte("asdnauthoritiesfkjasf2doiejfasdfa\x00\x04fqlkjfas3lkfjalwk4jfalsdkfjawle")
	Fuzz(crasher)
}
