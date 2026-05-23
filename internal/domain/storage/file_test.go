package storage_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/averak/vfx/internal/domain/storage"
)

// Equivalence partitions for the filename invariant.
// Boundary cases at exactly the length limit live in TestValidateFilename_LengthBoundary.
func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"ordinary name", "save.dat", false},
		{"dotted segments without separator", "save.v2.dat", false},
		{"multibyte name", "セーブデータ.bin", false},
		{"empty rejected", "", true},
		{"current dir rejected", ".", true},
		{"parent dir rejected", "..", true},
		{"forward slash rejected", "a/b", true},
		{"back slash rejected", "a\\b", true},
		{"leading space rejected", " save.dat", true},
		{"trailing space rejected", "save.dat ", true},
		{"control char rejected", "save\x00.dat", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := storage.ValidateFilename(tt.input)
			if tt.wantErr {
				if !errors.Is(err, storage.ErrInvalidFilename) {
					t.Fatalf("err = %v, want ErrInvalidFilename", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// The length limit counts runes, so a multibyte name of the same rune count must also pass (proving it is not a byte count).
func TestValidateFilename_LengthBoundary(t *testing.T) {
	atLimit := strings.Repeat("a", storage.MaxFilenameLength)
	if err := storage.ValidateFilename(atLimit); err != nil {
		t.Errorf("name of exactly MaxFilenameLength runes rejected: %v", err)
	}

	overLimit := strings.Repeat("a", storage.MaxFilenameLength+1)
	if !errors.Is(storage.ValidateFilename(overLimit), storage.ErrInvalidFilename) {
		t.Errorf("name one rune over the limit accepted")
	}

	multibyteAtLimit := strings.Repeat("あ", storage.MaxFilenameLength)
	if err := storage.ValidateFilename(multibyteAtLimit); err != nil {
		t.Errorf("multibyte name of MaxFilenameLength runes rejected (byte-counting?): %v", err)
	}
}

// Size is valid up to and including MaxFileSize; one byte over is rejected.
func TestValidateSize_Boundary(t *testing.T) {
	if err := storage.ValidateSize(storage.MaxFileSize); err != nil {
		t.Errorf("size of exactly MaxFileSize rejected: %v", err)
	}
	if !errors.Is(storage.ValidateSize(storage.MaxFileSize+1), storage.ErrFileTooLarge) {
		t.Errorf("size one byte over the limit accepted")
	}
	// A zero-byte file is legitimate (an intentionally empty save slot).
	if err := storage.ValidateSize(0); err != nil {
		t.Errorf("zero size rejected: %v", err)
	}
}

func TestNewFile_RevalidatesName(t *testing.T) {
	if _, err := storage.NewFile("a/b", 10, "deadbeef", time.Now()); !errors.Is(err, storage.ErrInvalidFilename) {
		t.Errorf("NewFile accepted an invalid filename")
	}
	if _, err := storage.NewFile("ok.dat", storage.MaxFileSize+1, "deadbeef", time.Now()); !errors.Is(err, storage.ErrFileTooLarge) {
		t.Errorf("NewFile accepted an over-size file")
	}
}
