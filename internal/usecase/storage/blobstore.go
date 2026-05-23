package storage

import (
	"context"
	"errors"
	"time"
)

// ErrObjectNotFound is returned by [BlobStore.Stat] when the key holds no object, e.g. an upload URL was issued but the client never completed the PUT.
var ErrObjectNotFound = errors.New("storage: object not found in blob store")

// SignedURL is a time-limited URL the client uses to transfer bytes directly to or from the blob store, bypassing the gateway.
// Headers lists the request headers the client must send unchanged; the URL's signature covers them, so omitting or altering one makes the store reject the request.
type SignedURL struct {
	URL     string
	Method  string
	Headers map[string]string
	Expires time.Time
}

// ObjectAttrs are the authoritative size and content hash the blob store reports for a stored object.
// The gateway reads these at commit time instead of trusting what the client claimed at upload time.
type ObjectAttrs struct {
	Size uint64
	Hash string
}

// BlobStore is the object-storage capability the usecase orchestrates.
//
// For player data the gateway only signs URLs and verifies objects, never touching the bytes.
// Title content is the exception: operators publish small files through the gateway, so Upload writes the bytes server-side (no client round-trip and no per-operator bucket credentials).
type BlobStore interface {
	SignUpload(ctx context.Context, key string, ttl time.Duration) (SignedURL, error)
	SignDownload(ctx context.Context, key string, ttl time.Duration) (SignedURL, error)

	// Upload writes data at key with the given content type, overwriting any existing object.
	Upload(ctx context.Context, key string, data []byte, contentType string) error

	// Stat returns ErrObjectNotFound when key holds no object.
	Stat(ctx context.Context, key string) (ObjectAttrs, error)

	Delete(ctx context.Context, key string) error
}
