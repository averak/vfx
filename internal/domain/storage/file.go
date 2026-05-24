// Package storage is the file-storage aggregate.
//
// A File is just metadata; the bytes live in object storage under a key derived from the same name.
// Player data is owner-scoped (a player reads and writes only their own files); title storage is operator-written and read-only for clients.
package storage

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxFilenameLength = 256

	// MaxFileSize is the absolute per-file ceiling.
	// Per-owner totals and file counts are an application rule enforced in the usecase, not here.
	MaxFileSize = 25 << 20 // 25 MiB
)

var (
	ErrInvalidFilename = errors.New("storage: invalid filename")
	ErrFileTooLarge    = errors.New("storage: file exceeds max size")
	ErrFileNotFound    = errors.New("storage: file not found")
)

// File is one stored object's metadata.
// Hash is whatever the object store reports for the content (md5 hex today); it is an opaque content identifier the client compares for diff-sync, not a security check.
type File struct {
	Filename   string
	Size       uint64
	Hash       string
	ModifiedAt time.Time
}

func NewFile(filename string, size uint64, hash string, modifiedAt time.Time) (*File, error) {
	if err := ValidateFilename(filename); err != nil {
		return nil, err
	}
	if err := ValidateSize(size); err != nil {
		return nil, err
	}
	return &File{
		Filename:   filename,
		Size:       size,
		Hash:       hash,
		ModifiedAt: modifiedAt,
	}, nil
}

// ValidateFilename admits only names that are safe and portable as an object key.
// "/" and "\" are rejected so a name can never escape its owner-scoped key prefix (owner_id + "/" + filename); the namespace is deliberately flat.
func ValidateFilename(name string) error {
	if name == "" || utf8.RuneCountInString(name) > MaxFilenameLength {
		return ErrInvalidFilename
	}
	if !utf8.ValidString(name) {
		return ErrInvalidFilename
	}
	// "." and ".." are path traversal even after the slash ban, since a store could interpret them.
	if name == "." || name == ".." {
		return ErrInvalidFilename
	}
	// Leading/trailing whitespace is almost always an accident and produces near-duplicate names that are hard to tell apart.
	if strings.TrimSpace(name) != name {
		return ErrInvalidFilename
	}
	for _, r := range name {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return ErrInvalidFilename
		}
	}
	return nil
}

func ValidateSize(size uint64) error {
	if size > MaxFileSize {
		return ErrFileTooLarge
	}
	return nil
}
