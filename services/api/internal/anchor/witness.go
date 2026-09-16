package anchor

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/digitorus/timestamp"
)

// A Witness records, outside this platform, that a Merkle root existed.
//
// Two are implemented, deliberately different in what they require you to
// trust:
//
//	RFC3161        A timestamping authority signs "this hash was presented to
//	               me at this time". It answers in about a second and the token
//	               verifies offline against the authority's certificate, but it
//	               is the authority's word: if the authority is dishonest or its
//	               key is taken, the proof is worth nothing.
//	OPENTIMESTAMPS The hash is committed into the Bitcoin blockchain through
//	               free public calendar servers. No one's word is required —
//	               the proof is checked against the chain itself — but the
//	               Bitcoin block that settles it takes hours to arrive.
//
// Neither costs anything, and neither sees anything but a 32-byte hash.
type Witness interface {
	Name() string
	// Configured reports whether this deployment has somewhere to send a root.
	Configured() bool
	Authority() string
	// Submit witnesses a root and returns the proof exactly as the witness
	// gave it, for storing and for handing to anyone who wants to check it.
	Submit(ctx context.Context, root []byte) (Receipt, error)
}

// Receipt is what a witness gave back.
type Receipt struct {
	Witness     string
	Status      string // WITNESSED where the proof is final, PENDING where it settles later
	Authority   string
	Proof       []byte
	ProofSHA256 string
	At          *time.Time
	Detail      string
}

const (
	StatusPending   = "PENDING"
	StatusWitnessed = "WITNESSED"
	StatusConfirmed = "CONFIRMED"
	StatusFailed    = "FAILED"
)

/* ------------------------------------------------------------- RFC 3161 --- */

// TSAWitness talks to an RFC 3161 timestamping authority.
//
// The authority is deliberately not hardcoded. For a demonstration a free
// public authority is enough; for real use, West Bengal should name a
// CCA-licensed Indian authority, so that the token carries weight under the
// Information Technology Act rather than being a foreign hobbyist service.
type TSAWitness struct {
	url    string
	client *http.Client
}

func NewTSAWitness() *TSAWitness {
	return &TSAWitness{
		url:    strings.TrimSpace(os.Getenv("ANCHOR_TSA_URL")),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *TSAWitness) Name() string      { return "RFC3161" }
func (w *TSAWitness) Configured() bool  { return w.url != "" }
func (w *TSAWitness) Authority() string { return w.url }

func (w *TSAWitness) Submit(ctx context.Context, root []byte) (Receipt, error) {
	if !w.Configured() {
		return Receipt{}, ErrNotConfigured
	}

	request, err := timestamp.CreateRequest(bytes.NewReader(root), &timestamp.RequestOptions{
		Hash: crypto.SHA256,
		// Ask for the authority's certificate in the token, so the proof can be
		// checked by someone who does not already hold it.
		Certificates: true,
	})
	if err != nil {
		return Receipt{}, fmt.Errorf("could not build the timestamp request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(request))
	if err != nil {
		return Receipt{}, err
	}
	req.Header.Set("Content-Type", "application/timestamp-query")

	resp, err := w.client.Do(req)
	if err != nil {
		return Receipt{}, fmt.Errorf("the timestamping authority could not be reached: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Receipt{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Receipt{}, fmt.Errorf("the timestamping authority answered %d", resp.StatusCode)
	}

	// Parse it before storing it: a token that cannot be read is not a proof,
	// and finding that out now is better than finding out in court.
	parsed, err := timestamp.ParseResponse(body)
	if err != nil {
		return Receipt{}, fmt.Errorf("the authority's answer could not be read as a timestamp: %w", err)
	}
	if !bytes.Equal(parsed.HashedMessage, root) {
		return Receipt{}, errors.New("the authority timestamped a different hash from the one sent")
	}

	at := parsed.Time
	sum := sha256.Sum256(body)
	return Receipt{
		Witness:     w.Name(),
		Status:      StatusWitnessed,
		Authority:   w.url,
		Proof:       body,
		ProofSHA256: hex.EncodeToString(sum[:]),
		At:          &at,
		Detail: fmt.Sprintf("Signed by the timestamping authority at %s. The token verifies offline against the authority's certificate.",
			at.UTC().Format(time.RFC3339)),
	}, nil
}

/* ------------------------------------------------------ OpenTimestamps --- */

// ErrNotConfigured: this deployment has nowhere to send a root. The screens
// say so rather than pretending a batch was anchored.
var ErrNotConfigured = errors.New("anchor_witness_not_configured")

// OpenTimestampsWitness submits a root to the free public OpenTimestamps
// calendars, which aggregate submissions and commit them into Bitcoin.
//
// The proof written here is a complete .ots file: the standard magic header,
// the root, and the calendar's own timestamp for it. It is deliberately the
// ordinary format rather than something of ours, so that `ots verify` — a tool
// that knows nothing about this platform — can check it.
type OpenTimestampsWitness struct {
	calendars []string
	client    *http.Client
}

// The public calendars, free and requiring no account. They are run by
// different people, which is the point: a root submitted to several does not
// depend on any one of them staying up.
var defaultCalendars = []string{
	"https://a.pool.opentimestamps.org",
	"https://b.pool.opentimestamps.org",
	"https://a.pool.eternitywall.com",
	"https://ots.btc.catallaxy.com",
}

func NewOpenTimestampsWitness() *OpenTimestampsWitness {
	calendars := defaultCalendars
	if configured := strings.TrimSpace(os.Getenv("ANCHOR_OTS_CALENDARS")); configured != "" {
		calendars = strings.Split(configured, ",")
		for i := range calendars {
			calendars[i] = strings.TrimSpace(calendars[i])
		}
	}
	// An air-gapped edge server has no route to a calendar. Setting this to
	// "none" says so plainly rather than leaving batches to time out.
	if len(calendars) == 1 && strings.EqualFold(calendars[0], "none") {
		calendars = nil
	}
	return &OpenTimestampsWitness{calendars: calendars, client: &http.Client{Timeout: 30 * time.Second}}
}

func (w *OpenTimestampsWitness) Name() string      { return "OPENTIMESTAMPS" }
func (w *OpenTimestampsWitness) Configured() bool  { return len(w.calendars) > 0 }
func (w *OpenTimestampsWitness) Authority() string { return strings.Join(w.calendars, ", ") }

// otsMagic is the file header every .ots proof begins with.
var otsMagic = []byte{
	0x00, 0x4f, 0x70, 0x65, 0x6e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61,
	0x6d, 0x70, 0x73, 0x00, 0x00, 0x50, 0x72, 0x6f, 0x6f, 0x66, 0x00, 0xbf,
	0x89, 0xe2, 0xe8, 0x84, 0xe8, 0x92, 0x94,
}

const (
	otsVersion   = 0x01
	otsSHA256Tag = 0x08
	otsAttestTag = 0x00
	otsForkTag   = 0xff
)

func (w *OpenTimestampsWitness) Submit(ctx context.Context, root []byte) (Receipt, error) {
	if !w.Configured() {
		return Receipt{}, ErrNotConfigured
	}

	type answer struct {
		calendar string
		body     []byte
		err      error
	}

	results := make(chan answer, len(w.calendars))
	for _, calendar := range w.calendars {
		go func(calendar string) {
			body, err := w.submitOne(ctx, calendar, root)
			results <- answer{calendar: calendar, body: body, err: err}
		}(calendar)
	}

	var accepted []answer
	var refused []string
	for range w.calendars {
		r := <-results
		if r.err != nil {
			refused = append(refused, fmt.Sprintf("%s (%v)", r.calendar, r.err))
			continue
		}
		accepted = append(accepted, r)
	}

	if len(accepted) == 0 {
		return Receipt{}, fmt.Errorf("no calendar accepted the root: %s", strings.Join(refused, "; "))
	}

	extra := make([][]byte, 0, len(accepted)-1)
	for _, a := range accepted[1:] {
		extra = append(extra, a.body)
	}
	proof := buildOTSProof(root, accepted[0].body, extra)
	sum := sha256.Sum256(proof)
	now := time.Now().UTC()

	calendars := make([]string, 0, len(accepted))
	for _, a := range accepted {
		calendars = append(calendars, a.calendar)
	}

	detail := fmt.Sprintf("Submitted to %d calendar(s): %s. The proof is complete but the Bitcoin block that settles it takes a few hours; until then it is a pending attestation.",
		len(accepted), strings.Join(calendars, ", "))
	if len(refused) > 0 {
		detail += " Not accepted by: " + strings.Join(refused, "; ") + "."
	}

	return Receipt{
		Witness:     w.Name(),
		Status:      StatusPending, // Bitcoin has not settled it yet, and saying otherwise would be a lie
		Authority:   strings.Join(calendars, ", "),
		Proof:       proof,
		ProofSHA256: hex.EncodeToString(sum[:]),
		At:          &now,
		Detail:      detail,
	}, nil
}

func (w *OpenTimestampsWitness) submitOne(ctx context.Context, calendar string, root []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(calendar, "/")+"/digest", bytes.NewReader(root))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/vnd.opentimestamps.v1")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("answered %d", resp.StatusCode)
	}
	if len(body) == 0 {
		return nil, errors.New("answered with an empty timestamp")
	}
	return body, nil
}

// buildOTSProof assembles the standard .ots file: header, the hash operation
// and the digest, then the calendars' timestamps. Several calendars are joined
// with the format's fork marker, so one proof carries every witness.
func buildOTSProof(root, first []byte, rest [][]byte) []byte {
	var out bytes.Buffer
	out.Write(otsMagic)
	out.WriteByte(otsVersion)
	out.WriteByte(otsSHA256Tag)
	out.Write(root)

	for _, body := range rest {
		out.WriteByte(otsForkTag)
		out.Write(body)
	}
	out.Write(first)

	return out.Bytes()
}
