package cmd

import (
	"errors"
	"testing"
)

func TestRemoveBeforeLogoutPreservesAuthenticationWhenDataRemovalFails(t *testing.T) {
	want := errors.New("data is still in use")
	authenticationRemoved := false

	err := removeBeforeLogout(
		func() error { return want },
		func() error {
			authenticationRemoved = true
			return nil
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("removeBeforeLogout() error = %v, want %v", err, want)
	}
	if authenticationRemoved {
		t.Fatal("removeBeforeLogout() removed authentication after data removal failed")
	}
}

func TestRemoveBeforeLogoutRemovesAuthenticationAfterData(t *testing.T) {
	var order []string
	err := removeBeforeLogout(
		func() error {
			order = append(order, "data")
			return nil
		},
		func() error {
			order = append(order, "authentication")
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "data" || order[1] != "authentication" {
		t.Fatalf("removal order = %v", order)
	}
}
