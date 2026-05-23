package valkey_test

import (
	"strings"
	"testing"

	"github.com/averak/vfx/internal/infra/valkey"
)

func TestNewClient_RejectsMalformedURL(t *testing.T) {
	_, err := valkey.NewClient("://bad")
	if err == nil {
		t.Fatal("NewClient with a malformed URL succeeded, want a parse error")
	}
	if !strings.Contains(err.Error(), "valkey: parse url") {
		t.Errorf("error = %v, want it wrapped as a parse-url failure", err)
	}
}
