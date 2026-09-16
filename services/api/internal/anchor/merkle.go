// Package anchor publishes a fingerprint of the audit trail outside the
// platform, so that a court or an auditor can satisfy themselves that a given
// state of the record existed by a given time without having to trust the
// police at all.
//
// What it proves, and what it does not. An anchor proves that one 32-byte
// value — the Merkle root of a batch of audit entries — existed no later than
// the time the witness recorded. It says nothing about whether the entries are
// true, and nothing about what happened before the batch was made: an entry
// altered and then anchored is anchored in its altered form. It is evidence of
// time, not of honesty.
//
// Only the root leaves the platform. It commits to every entry in the batch
// while revealing none of them, and cannot be reversed into them.
package anchor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// Merkle trees here follow RFC 6962, the structure used by Certificate
// Transparency, because it is the one auditors and courts have already seen
// and because it is not vulnerable to the second-preimage confusion that a
// naive tree has: leaves are hashed with a 0x00 prefix and interior nodes with
// 0x01, so no leaf can ever be mistaken for a node.
const (
	leafPrefix = 0x00
	nodePrefix = 0x01
)

// HashLeaf hashes one record's own hash into a tree leaf.
func HashLeaf(recordHash []byte) []byte {
	h := sha256.New()
	h.Write([]byte{leafPrefix})
	h.Write(recordHash)
	return h.Sum(nil)
}

// hashNode joins two subtrees.
func hashNode(left, right []byte) []byte {
	h := sha256.New()
	h.Write([]byte{nodePrefix})
	h.Write(left)
	h.Write(right)
	return h.Sum(nil)
}

// Root builds the Merkle root over leaves already hashed by HashLeaf.
//
// An odd node at any level is carried up unchanged rather than duplicated.
// Duplicating it — the Bitcoin approach — lets two different sets of leaves
// produce the same root, which is precisely the property an anchor must not
// have.
func Root(leaves [][]byte) ([]byte, error) {
	if len(leaves) == 0 {
		return nil, errors.New("a batch must commit to at least one entry")
	}

	level := make([][]byte, len(leaves))
	copy(level, leaves)

	for len(level) > 1 {
		next := make([][]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, level[i]) // carried, not duplicated
				continue
			}
			next = append(next, hashNode(level[i], level[i+1]))
		}
		level = next
	}

	return level[0], nil
}

// ProofStep is one sibling on the path from a leaf to the root.
type ProofStep struct {
	Hash string `json:"hash"`
	// Left says which side the sibling is on, which the verifier needs to
	// combine the pair in the same order the tree did.
	Left bool `json:"left"`
}

// Proof returns the path proving that the leaf at index is in the tree, as a
// verifier would walk it.
func Proof(leaves [][]byte, index int) ([]ProofStep, error) {
	if index < 0 || index >= len(leaves) {
		return nil, fmt.Errorf("entry %d is not in a batch of %d", index, len(leaves))
	}

	level := make([][]byte, len(leaves))
	copy(level, leaves)

	var steps []ProofStep
	for len(level) > 1 {
		next := make([][]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, level[i])
				if i == index {
					index = len(next) - 1
				}
				continue
			}
			if i == index {
				steps = append(steps, ProofStep{Hash: hex.EncodeToString(level[i+1]), Left: false})
				index = len(next)
			} else if i+1 == index {
				steps = append(steps, ProofStep{Hash: hex.EncodeToString(level[i]), Left: true})
				index = len(next)
			}
			next = append(next, hashNode(level[i], level[i+1]))
		}
		level = next
	}

	return steps, nil
}

// VerifyProof walks a proof from a leaf to a root. It is deliberately written
// so that it can be read, checked and reimplemented by someone who does not
// trust this codebase — which is the only way an anchor is worth anything.
func VerifyProof(leaf []byte, steps []ProofStep, root string) (bool, error) {
	current := leaf
	for _, step := range steps {
		sibling, err := hex.DecodeString(step.Hash)
		if err != nil {
			return false, fmt.Errorf("a step in the proof is not a hash: %w", err)
		}
		if step.Left {
			current = hashNode(sibling, current)
		} else {
			current = hashNode(current, sibling)
		}
	}
	return hex.EncodeToString(current) == root, nil
}
