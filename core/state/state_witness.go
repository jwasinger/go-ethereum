package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
	"math/big"
	"os"
	"path/filepath"
	"sort"
)

type Witness struct {
	Block       *types.Block
	blockHashes map[uint64]common.Hash
	codes       map[common.Hash]Code
	root        common.Hash
	lists       map[common.Hash]map[string][]byte
}

func (w *Witness) GetBlockHash(num uint64) common.Hash {
	return w.blockHashes[num]
}

func (w *Witness) Root() common.Hash {
	return w.root
}

type rlpWitness struct {
	Block       *types.Block
	Root        common.Hash
	Owners      []common.Hash
	AllPaths    [][]string
	AllNodes    [][][]byte
	BlockNums   []uint64
	BlockHashes []common.Hash
	Codes       []Code
	CodeHashes  []common.Hash
}

func (e *rlpWitness) ToWitness() *Witness {
	res := NewWitness(e.Root)
	res.Block = e.Block
	for i := 0; i < len(e.Codes); i++ {
		res.codes[e.CodeHashes[i]] = e.Codes[i]
	}
	for i, owner := range e.Owners {
		pathMap := make(map[string][]byte)
		for j := 0; j < len(e.AllPaths[i]); j++ {
			pathMap[e.AllPaths[i][j]] = e.AllNodes[i][j]
		}
		res.lists[owner] = pathMap
	}
	for i, blockNum := range e.BlockNums {
		res.blockHashes[blockNum] = e.BlockHashes[i]
	}
	return res
}

func DecodeWitnessRLP(b []byte) (*Witness, error) {
	var res rlpWitness
	if err := rlp.DecodeBytes(b, &res); err != nil {
		return nil, err
	}
	return res.ToWitness(), nil
}

func (w *Witness) EncodeRLP() ([]byte, error) {
	var encWit rlpWitness
	encWit.Block = w.Block
	encWit.Root = w.root

	for owner, nodeMap := range w.lists {
		encWit.Owners = append(encWit.Owners, owner)
		var ownerPaths []string
		var ownerNodes [][]byte

		for path, node := range nodeMap {
			ownerPaths = append(ownerPaths, path)
			ownerNodes = append(ownerNodes, node)
		}
		encWit.AllPaths = append(encWit.AllPaths, ownerPaths)
		encWit.AllNodes = append(encWit.AllNodes, ownerNodes)
	}

	for codeHash, code := range w.codes {
		encWit.CodeHashes = append(encWit.CodeHashes, codeHash)
		encWit.Codes = append(encWit.Codes, code)
	}

	for blockNum, blockHash := range w.blockHashes {
		encWit.BlockNums = append(encWit.BlockNums, blockNum)
		encWit.BlockHashes = append(encWit.BlockHashes, blockHash)
	}
	res, err := rlp.EncodeToBytes(&encWit)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (w *Witness) addAccessList(owner common.Hash, list map[string][]byte) {
	if len(list) > 0 {
		w.lists[owner] = list
	}
}

func (w *Witness) AddBlockHash(hash common.Hash, num uint64) {
	w.blockHashes[num] = hash
}

// TODO: don't include the code hash in the witness if not necessary
func (w *Witness) AddCode(hash common.Hash, code Code) {
	if code, ok := w.codes[hash]; ok && len(code) > 0 {
		return
	}
	w.codes[hash] = code
}

func (w *Witness) AddCodeHash(hash common.Hash) {
	if _, ok := w.codes[hash]; ok {
		return
	}
	w.codes[hash] = []byte{}
}

func (w *Witness) LogSizeWithBlock(b *types.Block) {
	enc, _ := w.EncodeRLP()
	fmt.Printf("block %d witness+block size: %d\n", b.Number(), len(enc))
}

func (w *Witness) Summary() string {
	b := new(bytes.Buffer)
	xx, err := rlp.EncodeToBytes(w.Block)
	if err != nil {
		panic(err)
	}
	totBlock := len(xx)

	yy, _ := w.EncodeRLP()

	totWit := len(yy)
	totCode := 0
	for _, c := range w.codes {
		totCode += len(c)
	}
	totNodes := 0
	totPaths := 0
	nodePathCount := 0
	for _, ownerPaths := range w.lists {
		for path, node := range ownerPaths {
			nodePathCount++
			totNodes += len(node)
			totPaths += len(path)
		}
	}

	fmt.Fprintf(b, "%4d hashes: %v\n", len(w.blockHashes), common.StorageSize(len(w.blockHashes)*32))
	fmt.Fprintf(b, "%4d owners: %v\n", len(w.lists), common.StorageSize(len(w.lists)*32))
	fmt.Fprintf(b, "%4d nodes:  %v\n", nodePathCount, common.StorageSize(totNodes))
	fmt.Fprintf(b, "%4d paths:  %v\n", nodePathCount, common.StorageSize(totPaths))
	fmt.Fprintf(b, "%4d codes:  %v\n", len(w.codes), common.StorageSize(totCode))
	fmt.Fprintf(b, "%4d codeHashes: %v\n", len(w.codes), common.StorageSize(len(w.codes)*32))
	fmt.Fprintf(b, "block (%4d txs): %v\n", len(w.Block.Transactions()), common.StorageSize(totBlock))
	fmt.Fprintf(b, "Total size: %v\n ", common.StorageSize(totWit))
	return b.String()
}

func (w *Witness) Copy() *Witness {
	var res Witness
	res.Block = w.Block // we don't actually mutate the block in the witness so don't deep copy

	for blockNr, blockHash := range w.blockHashes {
		res.blockHashes[blockNr] = blockHash
	}
	for codeHash, code := range w.codes {
		cpy := make([]byte, len(code))
		copy(cpy, code)
		res.codes[codeHash] = cpy
	}
	res.root = w.root
	for owner, owned := range w.lists {
		res.lists[owner] = make(map[string][]byte)
		for path, node := range owned {
			cpy := make([]byte, len(node))
			copy(cpy, node)
			res.lists[owner][path] = cpy
		}
	}
	return &res
}

func (w *Witness) sortedWitness() *rlpWitness {
	var sortedCodeHashes []common.Hash
	for key, _ := range w.codes {
		sortedCodeHashes = append(sortedCodeHashes, key)
	}
	sort.Slice(sortedCodeHashes, func(i, j int) bool {
		return bytes.Compare(sortedCodeHashes[i][:], sortedCodeHashes[j][:]) > 0
	})

	// sort the list of owners
	var owners []common.Hash
	for owner, _ := range w.lists {
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(i, j int) bool {
		return bytes.Compare(owners[i][:], owners[j][:]) > 0
	})

	var ownersPaths [][]string
	var ownersNodes [][][]byte

	// sort the noes of each owner by path
	for _, owner := range owners {
		nodes := w.lists[owner]
		// sort each node set
		var ownerPaths []string
		for path, _ := range nodes {
			ownerPaths = append(ownerPaths, path)
		}
		sort.Strings(ownerPaths)

		var ownerNodes [][]byte
		for _, path := range ownerPaths {
			ownerNodes = append(ownerNodes, nodes[path])
		}
		ownersPaths = append(ownersPaths, ownerPaths)
		ownersNodes = append(ownersNodes, ownerNodes)
	}

	var blockNrs []uint64
	var blockHashes []common.Hash
	for blockNr, blockHash := range w.blockHashes {
		blockNrs = append(blockNrs, blockNr)
		blockHashes = append(blockHashes, blockHash)
	}

	var codeHashes []common.Hash
	var codes []Code
	for codeHash, _ := range w.codes {
		codeHashes = append(codeHashes, codeHash)
	}
	sort.Slice(codeHashes, func(i, j int) bool {
		return bytes.Compare(codeHashes[i][:], codeHashes[j][:]) > 0
	})

	for _, codeHash := range codeHashes {
		codes = append(codes, w.codes[codeHash])
	}

	return &rlpWitness{
		Block:       w.Block,
		Root:        common.Hash{},
		Owners:      owners,
		AllPaths:    ownersPaths,
		AllNodes:    ownersNodes,
		BlockNums:   blockNrs,
		BlockHashes: blockHashes,
		Codes:       codes,
		CodeHashes:  codeHashes,
	}
}

func (w *Witness) PrettyPrint() string {
	sorted := w.sortedWitness()
	b := new(bytes.Buffer)
	fmt.Fprintf(b, "block: %+v\n", sorted.Block)
	fmt.Fprintf(b, "root: %x\n", sorted.Root)
	fmt.Fprint(b, "owners:\n")
	for i, owner := range sorted.Owners {
		if owner == (common.Hash{}) {
			fmt.Fprintf(b, "\troot:\n")
		} else {
			fmt.Fprintf(b, "\t%x:\n", owner)
		}
		ownerPaths := sorted.AllPaths[i]
		ownerNodes := sorted.AllNodes[i]
		for j, path := range ownerPaths {
			fmt.Fprintf(b, "\t\t%x:%x\n", []byte(path), ownerNodes[j])
		}
	}
	fmt.Fprintf(b, "block hashes:\n")
	for i, blockNum := range sorted.BlockNums {
		blockHash := sorted.BlockHashes[i]
		fmt.Fprintf(b, "\t%d:%x\n", blockNum, blockHash)
	}
	fmt.Fprintf(b, "codes:\n")
	for i, codeHash := range sorted.CodeHashes {
		code := sorted.Codes[i]
		fmt.Fprintf(b, "\t%x:%x\n", codeHash, code)
	}
	return b.String()
}

func (w *Witness) Hash() common.Hash {
	res, err := rlp.EncodeToBytes(w.sortedWitness())
	if err != nil {
		panic(err)
	}

	return common.Hash(sha256.Sum256(res[:]))
}

func NewWitness(root common.Hash) *Witness {
	return &Witness{
		Block:       nil,
		blockHashes: make(map[uint64]common.Hash),
		codes:       make(map[common.Hash]Code),
		root:        root,
		lists:       make(map[common.Hash]map[string][]byte),
	}
}

func DumpBlockWitnessToFile(w *Witness, path string) error {
	enc, _ := w.EncodeRLP()

	blockHash := w.Block.Hash()
	outputFName := fmt.Sprintf("%d-%x.rlp", w.Block.NumberU64(), blockHash[0:8])
	path = filepath.Join(path, outputFName)
	err := os.WriteFile(path, enc, 0644)
	if err != nil {
		return err
	}
	return nil
}

// This takes ownership over the trie nodes (and what else?) in this Witness object
func (w *Witness) PopulateDB(db ethdb.Database) error {
	batch := db.NewBatch()
	for owner, nodes := range w.lists {
		for path, node := range nodes {
			if owner == (common.Hash{}) {
				rawdb.WriteAccountTrieNode(batch, []byte(path), node)
			} else {
				rawdb.WriteStorageTrieNode(batch, owner, []byte(path), node)
			}
		}
	}

	for blockNum, blockHash := range w.blockHashes {
		fakeHeader := types.Header{}
		fakeHeader.ParentHash = blockHash
		fakeHeader.Number = new(big.Int).SetUint64(blockNum)
		rawdb.WriteHeader(batch, &fakeHeader)
	}

	for codeHash, code := range w.codes {
		rawdb.WriteCode(batch, codeHash, code)
	}

	if err := batch.Write(); err != nil {
		return err
	}
	return nil
}
