package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// IPIntelService resolves an IP address to network and geolocation context.
//
// This runs on the server, not in the officer's browser. Three reasons:
//
//   - Egress. One controlled outbound path from the edge server, rather than
//     every workstation reaching the public internet.
//   - Evidence. The lookup and its result are recorded in the audit trail, so
//     what an officer saw at a given moment can be reproduced later.
//   - Air gap. On a disconnected deployment the lookup is disabled by
//     configuration and the API says so plainly, instead of each browser
//     failing in its own way.
type IPIntelService struct {
	redis     *redis.Client
	auditRepo *repository.AuditRepository
	client    *http.Client

	// provider is the upstream lookup service. Empty disables external lookup
	// entirely, which is the correct setting for an air-gapped edge server.
	provider string
	enabled  bool

	// A cache inside the process, because the deployed API has no Redis: the
	// rate limiter there already falls back to process memory. Without this a
	// sign-in would call the provider every single time, and the provider is
	// rate limited by source address — which is how the deployed API came to
	// be answered with 429 for every lookup it made.
	local   map[string]localIntel
	localMu sync.RWMutex
}

type localIntel struct {
	intel *IPIntel
	until time.Time
}

// localIntelTTL is short beside the Redis cache's day: this one is lost on
// every cold start anyway, and a shorter life keeps a rate-limited refusal
// from being remembered as though it were an answer.
const localIntelTTL = 30 * time.Minute

func NewIPIntelService(rdb *redis.Client, auditRepo *repository.AuditRepository) *IPIntelService {
	provider := os.Getenv("IP_INTEL_PROVIDER")
	if provider == "" {
		// HTTPS by default. The browser implementation this replaces used
		// plaintext HTTP, which exposed both the queried address and the answer.
		provider = "https://ipapi.co/%s/json/"
	}

	enabled := strings.ToLower(os.Getenv("IP_INTEL_ENABLED")) != "false"

	return &IPIntelService{
		redis:     rdb,
		auditRepo: auditRepo,
		local:     make(map[string]localIntel),
		client:    &http.Client{Timeout: 8 * time.Second},
		provider:  provider,
		enabled:   enabled,
	}
}

// IPIntel is the structured answer. Fields left empty mean the provider did not
// supply them; the caller is never handed a guess.
type IPIntel struct {
	IP           string   `json:"ip"`
	Version      string   `json:"version,omitempty"`
	City         string   `json:"city,omitempty"`
	Region       string   `json:"region,omitempty"`
	Country      string   `json:"country,omitempty"`
	CountryCode  string   `json:"countryCode,omitempty"`
	Postal       string   `json:"postal,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Timezone     string   `json:"timezone,omitempty"`
	ASN          string   `json:"asn,omitempty"`
	Organisation string   `json:"organisation,omitempty"`

	// Classification the platform derives itself, not taken from the provider.
	IsPrivate  bool `json:"isPrivate"`
	IsLoopback bool `json:"isLoopback"`

	// Provenance, so a result can be judged and reproduced.
	Source      string    `json:"source"`
	RetrievedAt time.Time `json:"retrievedAt"`
	Cached      bool      `json:"cached"`
	Available   bool      `json:"available"`
	Note        string    `json:"note,omitempty"`
}

const ipIntelCacheTTL = 24 * time.Hour

// Lookup resolves an address. It never returns an error for an unreachable
// provider: an investigator needs to know the lookup could not be made, which
// is different from the address not existing.
// Lookup resolves an address on an officer's behalf and records the egress
// against them with the purpose they stated.
func (s *IPIntelService) Lookup(ctx context.Context, raw string, actor *uuid.UUID, purpose string) (*IPIntel, error) {
	s.record(ctx, actor, strings.TrimSpace(raw), purpose)
	return s.locate(ctx, raw)
}

// Locate resolves an address for the platform's own records — the place a
// sign-in came from, written onto the sign-in's own audit entry.
//
// It does not write an audit entry of its own. The egress it may cause is
// already accounted for: the address and what was learned about it are on the
// entry this result is attached to, and a lookup that recorded itself as well
// would double every sign-in in the trail for no added account of anything.
func (s *IPIntelService) Locate(ctx context.Context, raw string) (*IPIntel, error) {
	return s.locate(ctx, raw)
}

func (s *IPIntelService) locate(ctx context.Context, raw string) (*IPIntel, error) {
	if cached, ok := s.fromLocal(strings.TrimSpace(raw)); ok {
		return cached, nil
	}
	result, err := s.resolve(ctx, raw)
	if err == nil && result != nil {
		s.toLocal(result)
	}
	return result, err
}

func (s *IPIntelService) fromLocal(ip string) (*IPIntel, bool) {
	s.localMu.RLock()
	hit, ok := s.local[ip]
	s.localMu.RUnlock()
	if !ok || time.Now().After(hit.until) {
		return nil, false
	}
	copied := *hit.intel
	copied.Cached = true
	return &copied, true
}

func (s *IPIntelService) toLocal(intel *IPIntel) {
	s.localMu.Lock()
	// Bounded. A police deployment sees a few hundred addresses; anything far
	// past that is a scan, and the map must not grow without limit.
	if len(s.local) > 2000 {
		s.local = make(map[string]localIntel)
	}
	s.local[intel.IP] = localIntel{intel: intel, until: time.Now().Add(localIntelTTL)}
	s.localMu.Unlock()
}

func (s *IPIntelService) resolve(ctx context.Context, raw string) (*IPIntel, error) {
	addr := net.ParseIP(strings.TrimSpace(raw))
	if addr == nil {
		return nil, fmt.Errorf("%q is not a valid IP address", raw)
	}

	result := &IPIntel{
		IP:          addr.String(),
		IsPrivate:   addr.IsPrivate() || addr.IsLinkLocalUnicast(),
		IsLoopback:  addr.IsLoopback(),
		RetrievedAt: time.Now(),
		Source:      "local",
		Available:   true,
	}
	if addr.To4() != nil {
		result.Version = "IPv4"
	} else {
		result.Version = "IPv6"
	}

	// A private or loopback address has no public registration to look up, and
	// asking an external provider about it would leak internal topology.
	if result.IsPrivate || result.IsLoopback {
		result.Note = "Private or loopback address — no public registration exists"
		return result, nil
	}

	if !s.enabled {
		result.Available = false
		result.Note = "External lookup is disabled on this deployment"
		return result, nil
	}

	if cached := s.fromCache(ctx, addr.String()); cached != nil {
		cached.Cached = true
		return cached, nil
	}

	fetched, err := s.fetch(ctx, addr.String())
	if err != nil {
		result.Available = false
		result.Note = "Lookup could not be completed: " + err.Error()
		return result, nil
	}

	fetched.IsPrivate = result.IsPrivate
	fetched.IsLoopback = result.IsLoopback
	fetched.Version = result.Version
	s.toCache(ctx, fetched)
	return fetched, nil
}

func (s *IPIntelService) fetch(ctx context.Context, ip string) (*IPIntel, error) {
	url := fmt.Sprintf(s.provider, ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider returned status %d", resp.StatusCode)
	}

	var payload struct {
		City      string   `json:"city"`
		Region    string   `json:"region"`
		Country   string   `json:"country_name"`
		Code      string   `json:"country_code"`
		Postal    string   `json:"postal"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
		Timezone  string   `json:"timezone"`
		ASN       string   `json:"asn"`
		Org       string   `json:"org"`
		Error     bool     `json:"error"`
		Reason    string   `json:"reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Error {
		return nil, fmt.Errorf("provider declined: %s", payload.Reason)
	}

	return &IPIntel{
		IP:           ip,
		City:         payload.City,
		Region:       payload.Region,
		Country:      payload.Country,
		CountryCode:  payload.Code,
		Postal:       payload.Postal,
		Latitude:     payload.Latitude,
		Longitude:    payload.Longitude,
		Timezone:     payload.Timezone,
		ASN:          payload.ASN,
		Organisation: payload.Org,
		Source:       s.providerHost(),
		RetrievedAt:  time.Now(),
		Available:    true,
	}, nil
}

func (s *IPIntelService) providerHost() string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(s.provider, "https://"), "http://")
	if i := strings.Index(trimmed, "/"); i > 0 {
		return trimmed[:i]
	}
	return trimmed
}

func (s *IPIntelService) fromCache(ctx context.Context, ip string) *IPIntel {
	if s.redis == nil {
		return nil
	}
	raw, err := s.redis.Get(ctx, "ipintel:"+ip).Result()
	if err != nil {
		return nil
	}
	var out IPIntel
	if json.Unmarshal([]byte(raw), &out) != nil {
		return nil
	}
	return &out
}

func (s *IPIntelService) toCache(ctx context.Context, intel *IPIntel) {
	if s.redis == nil {
		return
	}
	if encoded, err := json.Marshal(intel); err == nil {
		s.redis.Set(ctx, "ipintel:"+intel.IP, encoded, ipIntelCacheTTL)
	}
}

// record writes the lookup to the audit trail. Who asked about which address,
// and why, is itself information worth keeping.
func (s *IPIntelService) record(ctx context.Context, actor *uuid.UUID, ip, purpose string) {
	if s.auditRepo == nil {
		return
	}
	description := "IP intelligence lookup for " + ip
	if purpose != "" {
		description += " — " + purpose
	}
	if err := s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       actor,
		Action:       "ip_intel_lookup",
		ResourceType: "ip_address",
		Description:  &description,
		Success:      true,
	}); err != nil {
		log.Printf("audit write failed for ip_intel_lookup: %v", err)
	}
}
