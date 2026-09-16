package anchor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func leaves(n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		sum := sha256.Sum256([]byte(fmt.Sprintf("audit entry %d", i)))
		out[i] = HashLeaf(sum[:])
	}
	return out
}

func TestEveryEntryInABatchCanBeProved(t *testing.T) {
	// Odd sizes matter: they are where a tree implementation usually goes wrong.
	for _, size := range []int{1, 2, 3, 4, 5, 7, 8, 9, 100, 101} {
		batch := leaves(size)
		root, err := Root(batch)
		if err != nil {
			t.Fatalf("batch of %d: %v", size, err)
		}
		rootHex := hex.EncodeToString(root)

		for index := range batch {
			steps, err := Proof(batch, index)
			if err != nil {
				t.Fatalf("batch of %d, entry %d: %v", size, index, err)
			}
			ok, err := VerifyProof(batch[index], steps, rootHex)
			if err != nil {
				t.Fatalf("batch of %d, entry %d: %v", size, index, err)
			}
			if !ok {
				t.Errorf("batch of %d: entry %d could not be proved against the root", size, index)
			}
		}
	}
}

// The whole point of an anchor: an entry that was altered after the batch was
// made no longer proves against the anchored root.
func TestAnAlteredEntryNoLongerProves(t *testing.T) {
	batch := leaves(9)
	root, err := Root(batch)
	if err != nil {
		t.Fatal(err)
	}

	steps, err := Proof(batch, 4)
	if err != nil {
		t.Fatal(err)
	}

	altered := sha256.Sum256([]byte("audit entry 4, quietly changed"))
	ok, err := VerifyProof(HashLeaf(altered[:]), steps, hex.EncodeToString(root))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("an altered entry still proved against the anchored root")
	}
}

// Two different batches must never share a root. The naive tree that
// duplicates a lone odd node fails this; RFC 6962 with a carried node does not.
func TestDifferentBatchesHaveDifferentRoots(t *testing.T) {
	seen := map[string]int{}
	for size := 1; size <= 12; size++ {
		root, err := Root(leaves(size))
		if err != nil {
			t.Fatal(err)
		}
		hexRoot := hex.EncodeToString(root)
		if previous, clash := seen[hexRoot]; clash {
			t.Errorf("a batch of %d entries has the same root as a batch of %d", size, previous)
		}
		seen[hexRoot] = size
	}
}

// A leaf and an interior node must not be confusable, or a batch could be
// presented as having a structure it did not have.
func TestALeafIsNotANode(t *testing.T) {
	a := sha256.Sum256([]byte("left"))
	b := sha256.Sum256([]byte("right"))

	node := hashNode(a[:], b[:])
	leaf := HashLeaf(append(append([]byte{}, a[:]...), b[:]...))

	if hex.EncodeToString(node) == hex.EncodeToString(leaf) {
		t.Error("a leaf hashed the same as an interior node")
	}
}

func TestAnEmptyBatchIsRefused(t *testing.T) {
	if _, err := Root(nil); err == nil {
		t.Error("an empty batch produced a root")
	}
	if _, err := Proof(leaves(3), 5); err == nil {
		t.Error("a proof was produced for an entry outside the batch")
	}
}
