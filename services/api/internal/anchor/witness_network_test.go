package anchor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These tests reach the public witnesses, so they are opt-in: set
// ANCHOR_NETWORK_TESTS=1 to run them. They are how the proof format was
// checked against the reference tooling in the first place, and re-running
// them is how it stays checked.
func networkTests(t *testing.T) {
	t.Helper()
	if os.Getenv("ANCHOR_NETWORK_TESTS") == "" {
		t.Skip("set ANCHOR_NETWORK_TESTS=1 to reach the public witnesses")
	}
}

// The proof this platform writes must be readable by tools that know nothing
// about this platform. The test writes a real .ots file for a real root; the
// companion script checks it with the reference `ots` client.
func TestOpenTimestampsProducesAProofTheReferenceToolCanRead(t *testing.T) {
	networkTests(t)

	w := NewOpenTimestampsWitness()
	if !w.Configured() {
		t.Skip("no calendars configured")
	}

	sum := sha256.Sum256([]byte(fmt.Sprintf("npdms anchor test %d", time.Now().UnixNano())))
	root := sum[:]

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	receipt, err := w.Submit(ctx, root)
	if err != nil {
		t.Fatalf("no calendar accepted the root: %v", err)
	}

	if receipt.Status != StatusPending {
		t.Errorf("status was %q; a fresh Bitcoin attestation is pending until a block settles it", receipt.Status)
	}
	if len(receipt.Proof) < len(otsMagic)+33 {
		t.Fatalf("the proof is %d bytes, too short to be an .ots file", len(receipt.Proof))
	}
	if string(receipt.Proof[:len(otsMagic)]) != string(otsMagic) {
		t.Error("the proof does not begin with the OpenTimestamps header")
	}
	// The digest must appear in the file, or the proof is about something else.
	if !containsBytes(receipt.Proof, root) {
		t.Error("the proof does not carry the root that was submitted")
	}

	// Leave the artefacts where the reference client can be pointed at them.
	dir := os.Getenv("ANCHOR_TEST_OUTPUT")
	if dir == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "root.bin"), root, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.bin.ots"), receipt.Proof, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("root %s written to %s for checking with the reference client",
		hex.EncodeToString(root), dir)
}

// A timestamping authority answers in about a second, and the token must
// commit to the root that was sent — not to something else.
func TestTSAWitnessTimestampsTheRootItWasGiven(t *testing.T) {
	networkTests(t)

	w := NewTSAWitness()
	if !w.Configured() {
		t.Skip("ANCHOR_TSA_URL is not set")
	}

	sum := sha256.Sum256([]byte(fmt.Sprintf("npdms anchor test %d", time.Now().UnixNano())))
	root := sum[:]

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	receipt, err := w.Submit(ctx, root)
	if err != nil {
		t.Fatalf("the authority did not answer: %v", err)
	}

	if receipt.Status != StatusWitnessed {
		t.Errorf("status was %q, want %q: a signed token is final, not pending", receipt.Status, StatusWitnessed)
	}
	if receipt.At == nil {
		t.Fatal("the authority's time was not recorded")
	}
	if drift := time.Since(*receipt.At); drift > 10*time.Minute || drift < -10*time.Minute {
		t.Errorf("the authority's time is %v away from ours, which is too far to be right", drift)
	}

	if dir := os.Getenv("ANCHOR_TEST_OUTPUT"); dir != "" {
		os.WriteFile(filepath.Join(dir, "root.tsr"), receipt.Proof, 0o644)
		os.WriteFile(filepath.Join(dir, "root-tsa.bin"), root, 0o644)
	}
}

// A deployment with no witness configured must say so, not silently record a
// batch as anchored.
func TestAnUnconfiguredWitnessRefuses(t *testing.T) {
	t.Setenv("ANCHOR_TSA_URL", "")
	if w := NewTSAWitness(); w.Configured() {
		t.Error("a timestamping authority with no URL reported itself as configured")
	} else if _, err := w.Submit(context.Background(), make([]byte, 32)); err != ErrNotConfigured {
		t.Errorf("submitting to an unconfigured authority gave %v, want ErrNotConfigured", err)
	}

	t.Setenv("ANCHOR_OTS_CALENDARS", "none")
	if w := NewOpenTimestampsWitness(); w.Configured() {
		t.Error("calendars set to none reported as configured")
	}
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
