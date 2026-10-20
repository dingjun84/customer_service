package admin

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct-horse-1")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !VerifyPassword(hash, "correct-horse-1") {
		t.Fatal("correct password did not verify")
	}
	if VerifyPassword(hash, "correct-horse-2") {
		t.Fatal("wrong password verified")
	}
	// Two hashes of the same password must differ, otherwise salts are not random.
	other, err := HashPassword("correct-horse-1")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == other {
		t.Fatal("two hashes of the same password are identical; salt is not random")
	}
}

func TestPasswordHashRejectsShortAndMalformed(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected error for a password under 8 characters")
	}
	for _, encoded := range []string{
		"", "plain", "pbkdf2_sha256$120000$onlythree",
		"bcrypt$120000$AAAA$AAAA", "pbkdf2_sha256$abc$AAAA$AAAA",
	} {
		if VerifyPassword(encoded, "whatever") {
			t.Fatalf("malformed hash %q verified", encoded)
		}
	}
}

func TestSessionTokenIsRandomAndHashed(t *testing.T) {
	t1, d1, err := newSessionToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	t2, d2, err := newSessionToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if t1 == t2 || d1 == d2 {
		t.Fatal("tokens are not random")
	}
	if d1 == t1 {
		t.Fatal("stored digest equals the raw token")
	}
	if len(d1) != 64 {
		t.Fatalf("digest length = %d, want 64 hex chars", len(d1))
	}
	if hashToken(t1) != d1 {
		t.Fatal("hashToken is not stable")
	}
}

func TestTicketTransitionTable(t *testing.T) {
	allowed := [][2]string{
		{"open", "assigned"}, {"open", "closed"}, {"assigned", "resolved"},
		{"pending_customer", "assigned"}, {"resolved", "closed"}, {"closed", "open"},
	}
	for _, pair := range allowed {
		if !ticketTransitionAllowed(pair[0], pair[1]) {
			t.Fatalf("expected %s -> %s to be allowed", pair[0], pair[1])
		}
	}
	denied := [][2]string{
		{"closed", "resolved"}, {"closed", "assigned"}, {"resolved", "assigned"},
		{"open", "open"}, {"unknown", "closed"},
	}
	for _, pair := range denied {
		if ticketTransitionAllowed(pair[0], pair[1]) {
			t.Fatalf("expected %s -> %s to be rejected", pair[0], pair[1])
		}
	}
}

func TestMoneyFormatting(t *testing.T) {
	cases := map[int64]string{
		0:       "¥0.00",
		1:       "¥0.01",
		99:      "¥0.99",
		100:     "¥1.00",
		123456:  "¥1,234.56",
		1000000: "¥10,000.00",
		-2500:   "-¥25.00",
	}
	for minor, want := range cases {
		if got := Money(minor); got != want {
			t.Fatalf("Money(%d) = %q, want %q", minor, got, want)
		}
	}
}

func TestPageNormalisation(t *testing.T) {
	p := Page{Page: 0, PageSize: 0}.Normalise()
	if p.Page != 1 || p.PageSize != 20 {
		t.Fatalf("defaults = %+v", p)
	}
	p = Page{Page: 3, PageSize: 500}.Normalise()
	if p.PageSize != 100 {
		t.Fatalf("page size = %d, want 100 cap", p.PageSize)
	}
	if p.Offset() != 200 {
		t.Fatalf("offset = %d, want 200", p.Offset())
	}
}
