// Package fraud validates and normalises the identifiers an officer records
// from a cyber-fraud complaint. Normalisation decides whether two complaints
// name the same entity, so it must be deterministic and conservative: it
// removes formatting, never guesses.
package fraud

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
)

var ErrInvalidEntity = errors.New("invalid entity")

func invalid(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", ErrInvalidEntity, fmt.Sprintf(format, args...))
}

var (
	// Indian mobile numbers: ten digits beginning 6–9.
	mobilePattern = regexp.MustCompile(`^[6-9][0-9]{9}$`)
	// NPCI virtual payment address: handle@psp.
	upiPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,255}@[a-z][a-z0-9]{1,63}$`)
	// RBI IFSC: four letters, a zero, six alphanumerics.
	ifscPattern    = regexp.MustCompile(`^[A-Z]{4}0[A-Z0-9]{6}$`)
	accountPattern = regexp.MustCompile(`^[0-9]{9,18}$`)
	walletPattern  = regexp.MustCompile(`^[A-Za-z0-9:_.-]{6,128}$`)
	separators     = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "")
)

// Normalized is the canonical form of one entity.
type Normalized struct {
	Value   string // matching key, unique per type
	Display string // what the officer sees
	IFSC    *string
}

// Normalize validates value for the given entity type. ifsc is required for
// bank accounts and rejected for everything else.
func Normalize(entityType, value string, ifsc *string) (*Normalized, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil, invalid("a value is required")
	}
	if entityType != "BANK_ACCOUNT" && ifsc != nil && strings.TrimSpace(*ifsc) != "" {
		return nil, invalid("an IFSC applies only to bank accounts")
	}

	switch entityType {
	case "PHONE":
		digits := separators.Replace(v)
		digits = strings.TrimPrefix(digits, "+")
		switch {
		case len(digits) == 12 && strings.HasPrefix(digits, "91"):
			digits = digits[2:]
		case len(digits) == 11 && strings.HasPrefix(digits, "0"):
			digits = digits[1:]
		}
		if !mobilePattern.MatchString(digits) {
			return nil, invalid("%q is not a ten-digit Indian mobile number", value)
		}
		return &Normalized{Value: digits, Display: "+91 " + digits[:5] + " " + digits[5:]}, nil

	case "UPI":
		lower := strings.ToLower(v)
		if !upiPattern.MatchString(lower) {
			return nil, invalid("%q is not a UPI ID (expected name@bank)", value)
		}
		return &Normalized{Value: lower, Display: lower}, nil

	case "BANK_ACCOUNT":
		if ifsc == nil || strings.TrimSpace(*ifsc) == "" {
			return nil, invalid("a bank account needs its IFSC")
		}
		code := strings.ToUpper(strings.TrimSpace(*ifsc))
		if !ifscPattern.MatchString(code) {
			return nil, invalid("%q is not a valid IFSC", *ifsc)
		}
		account := separators.Replace(v)
		if !accountPattern.MatchString(account) {
			return nil, invalid("%q is not a bank account number (9–18 digits)", value)
		}
		return &Normalized{Value: code + ":" + account, Display: account + " · " + code, IFSC: &code}, nil

	case "WALLET":
		if !walletPattern.MatchString(v) {
			return nil, invalid("%q is not a wallet identifier (6–128 letters, digits, : _ . -)", value)
		}
		return &Normalized{Value: strings.ToLower(v), Display: v}, nil

	case "URL":
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, invalid("%q is not an http or https URL", value)
		}
		u.Scheme = strings.ToLower(u.Scheme)
		u.Host = strings.ToLower(u.Host)
		u.Fragment = ""
		normalized := strings.TrimSuffix(u.String(), "/")
		return &Normalized{Value: normalized, Display: normalized}, nil

	case "EMAIL":
		addr, err := mail.ParseAddress(v)
		if err != nil || addr.Name != "" || !strings.Contains(addr.Address, ".") {
			return nil, invalid("%q is not an email address", value)
		}
		lower := strings.ToLower(addr.Address)
		return &Normalized{Value: lower, Display: lower}, nil
	}
	return nil, invalid("unknown entity type %q", entityType)
}
