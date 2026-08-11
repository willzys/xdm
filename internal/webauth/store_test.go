package webauth

import "testing"

func TestSelectSessionUsesActiveAccountByDefault(t *testing.T) {
	vault := Vault{
		Active: "2",
		Sessions: map[string]Session{
			"1": {Account: Account{ID: "1", Username: "first"}},
			"2": {Account: Account{ID: "2", Username: "second"}},
		},
	}

	session, err := selectSession(vault, "")
	if err != nil {
		t.Fatal(err)
	}
	if session.Key() != "2" {
		t.Fatalf("selected account = %q, want %q", session.Key(), "2")
	}
}

func TestSelectSessionMatchesKeyOrUsername(t *testing.T) {
	vault := Vault{Sessions: map[string]Session{
		"123": {Account: Account{ID: "123", Username: "Example"}},
	}}

	for _, account := range []string{"123", "example", "@EXAMPLE"} {
		session, err := selectSession(vault, account)
		if err != nil {
			t.Fatalf("selectSession(%q): %v", account, err)
		}
		if session.Key() != "123" {
			t.Fatalf("selectSession(%q) selected %q", account, session.Key())
		}
	}
}

func TestSelectSessionRejectsUnknownAccount(t *testing.T) {
	vault := Vault{Sessions: map[string]Session{
		"123": {Account: Account{ID: "123", Username: "example"}},
	}}
	if _, err := selectSession(vault, "missing"); err == nil {
		t.Fatal("selectSession() accepted an unknown account")
	}
}
