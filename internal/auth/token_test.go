package auth

import "testing"

func TestGenerateShortID(t *testing.T) {
	const iterations = 100

	for i := 0; i < iterations; i++ {
		token, err := GenerateShortID()
		if err != nil {
			t.Fatalf("GenerateShortID: %v", err)
		}
		if len(token) != ShortIDLength {
			t.Fatalf("GenerateShortID() = %q, length %d, want %d", token, len(token), ShortIDLength)
		}
		for _, r := range token {
			alphanumeric := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			if !alphanumeric {
				t.Fatalf("GenerateShortID() = %q contains %q, want [A-Za-z0-9]", token, r)
			}
		}
	}
}
