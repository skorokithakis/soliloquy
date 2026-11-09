package auth

import "testing"

func TestGenerateInviteToken(t *testing.T) {
	const iterations = 100

	for i := 0; i < iterations; i++ {
		token, err := GenerateInviteToken()
		if err != nil {
			t.Fatalf("GenerateInviteToken: %v", err)
		}
		if len(token) != 8 {
			t.Fatalf("GenerateInviteToken() = %q, length %d, want 8", token, len(token))
		}
		for _, r := range token {
			alphanumeric := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			if !alphanumeric {
				t.Fatalf("GenerateInviteToken() = %q contains %q, want [A-Za-z0-9]", token, r)
			}
		}
	}
}
