package services

import "testing"

func TestRupees(t *testing.T) {
	for paise, want := range map[int64]string{
		0: "₹0", 5000: "₹50", 100000: "₹1,000", 18400000: "₹1,84,000", 18400050: "₹1,84,000.50",
		1234567890: "₹1,23,45,678.90",
	} {
		if got := rupees(paise); got != want {
			t.Errorf("rupees(%d) = %q, want %q", paise, got, want)
		}
	}
}
