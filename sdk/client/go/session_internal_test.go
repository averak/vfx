package vfxclient

import (
	"encoding/base64"
	"strings"
	"testing"
)

// jwtLike builds a token of the shape matchIDFromToken expects: three
// dot-separated segments whose middle segment is the base64url-encoded
// (unpadded, as JWT mandates) JSON payload.
func jwtLike(payloadJSON string) string {
	seg := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return "header." + seg + ".signature"
}

func TestMatchIDFromToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		want    string
		wantErr bool
	}{
		{"extracts mid", jwtLike(`{"mid":"match-123","sub":"player-1"}`), "match-123", false},
		{"not three segments", "only.two", "", true},
		{"payload not base64", "header.!!!notbase64!!!.sig", "", true},
		{"payload not json", jwtLike("not json"), "", true},
		{"missing mid claim", jwtLike(`{"sub":"player-1"}`), "", true},
		{"empty mid claim", jwtLike(`{"mid":""}`), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchIDFromToken(tt.token)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("matchIDFromToken(%q) = %q, want error", tt.token, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("matchIDFromToken = %q, want %q", got, tt.want)
			}
		})
	}
}

// decodeJWTSegment must accept segments regardless of how many padding
// characters a real JWT omitted, so it re-pads to a multiple of four.
func TestDecodeJWTSegment_RepadsAllRemainders(t *testing.T) {
	for _, raw := range []string{"a", "ab", "abc", "abcd", "hello world"} {
		seg := base64.RawURLEncoding.EncodeToString([]byte(raw))
		got, err := decodeJWTSegment(seg)
		if err != nil {
			t.Fatalf("decodeJWTSegment(%q) (len%%4=%d): %v", seg, len(seg)%4, err)
		}
		if string(got) != raw {
			t.Errorf("decodeJWTSegment round trip = %q, want %q", got, raw)
		}
	}
}

func TestDecodeJWTSegment_Rejects(t *testing.T) {
	if _, err := decodeJWTSegment(strings.Repeat("!", 3)); err == nil {
		t.Error("decodeJWTSegment accepted non-base64 input")
	}
}
