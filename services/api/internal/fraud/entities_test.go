package fraud

import (
	"errors"
	"testing"
)

func strp(s string) *string { return &s }

func TestNormalize(t *testing.T) {
	cases := []struct {
		name, typ, value string
		ifsc             *string
		want             string
		wantErr          bool
	}{
		{"mobile plain", "PHONE", "9831012345", nil, "9831012345", false},
		{"mobile with +91 and spaces", "PHONE", "+91 98310 12345", nil, "9831012345", false},
		{"mobile with trunk zero", "PHONE", "09831012345", nil, "9831012345", false},
		{"landline-like rejected", "PHONE", "0332212345", nil, "", true},
		{"mobile starting 5 rejected", "PHONE", "5831012345", nil, "", true},

		{"upi lower-cased", "UPI", "Fraud.Pay@OKAXIS", nil, "fraud.pay@okaxis", false},
		{"upi without psp rejected", "UPI", "fraudpay", nil, "", true},
		{"upi with space rejected", "UPI", "fraud pay@okaxis", nil, "", true},

		{"account with ifsc", "BANK_ACCOUNT", "1234 5678 9012", strp("sbin0001234"), "SBIN0001234:123456789012", false},
		{"account without ifsc rejected", "BANK_ACCOUNT", "123456789012", nil, "", true},
		{"account bad ifsc rejected", "BANK_ACCOUNT", "123456789012", strp("SBIN1001234"), "", true},
		{"account too short rejected", "BANK_ACCOUNT", "12345", strp("SBIN0001234"), "", true},
		{"ifsc on a phone rejected", "PHONE", "9831012345", strp("SBIN0001234"), "", true},

		{"wallet", "WALLET", "TQ9x7a1b2c3d", nil, "tq9x7a1b2c3d", false},
		{"wallet too short rejected", "WALLET", "abc", nil, "", true},

		{"url normalised", "URL", "HTTPS://Secure-Verify.IN/login/", nil, "https://secure-verify.in/login", false},
		{"url without scheme rejected", "URL", "secure-verify.in", nil, "", true},
		{"javascript url rejected", "URL", "javascript:alert(1)", nil, "", true},

		{"email lower-cased", "EMAIL", "Scam.Desk@Example.COM", nil, "scam.desk@example.com", false},
		{"email with display name rejected", "EMAIL", "Desk <desk@example.com>", nil, "", true},
		{"email without domain dot rejected", "EMAIL", "desk@localhost", nil, "", true},

		{"empty rejected", "UPI", "  ", nil, "", true},
		{"unknown type rejected", "DEVICE", "abc123", nil, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.typ, tc.value, tc.ifsc)
			if tc.wantErr {
				if err == nil || !errors.Is(err, ErrInvalidEntity) {
					t.Fatalf("want ErrInvalidEntity, got %v (value %+v)", err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Value != tc.want {
				t.Fatalf("value = %q, want %q", got.Value, tc.want)
			}
		})
	}
}
