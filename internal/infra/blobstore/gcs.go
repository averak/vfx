// Package blobstore implements [usecasestorage.BlobStore] on Google Cloud Storage.
//
// File bytes never pass through the gateway: the adapter issues V4 signed URLs the client uses to PUT and GET objects directly, and reads object attributes only to verify a completed upload.
package blobstore

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/storage"

	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

type GCS struct {
	client *storage.Client
	bucket string
	signer signer
}

var _ usecasestorage.BlobStore = (*GCS)(nil)

// signer holds the material BucketHandle.SignedURL needs.
// Against the emulator the signature is never verified, so an ephemeral key and a placeholder access id suffice.
// In production both fields are empty and the storage client auto-detects the service account and signs through IAM SignBlob, so no private key is ever placed on disk.
type signer struct {
	accessID   string
	privateKey []byte
}

type Config struct {
	Bucket string

	// Emulated is true when STORAGE_EMULATOR_HOST is set; it selects the ephemeral-key signing path.
	// The storage client itself honors STORAGE_EMULATOR_HOST automatically, so the URLs already point at the emulator.
	Emulated bool
}

// NewGCS's returned cleanup closes the storage client and must be called before the process exits.
func NewGCS(ctx context.Context, cfg Config) (*GCS, func(), error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("blobstore: new client: %w", err)
	}

	g := &GCS{client: client, bucket: cfg.Bucket}
	if cfg.Emulated {
		key, genErr := ephemeralSigningKey()
		if genErr != nil {
			//nolint:errcheck // Failing construction; the client we are about to discard cannot report anything actionable.
			_ = client.Close()
			return nil, nil, genErr
		}
		g.signer = signer{accessID: "vfx-local-signer@example.com", privateKey: key}
	}

	cleanup := func() {
		//nolint:errcheck // Close errors at shutdown are not actionable.
		_ = client.Close()
	}
	return g, cleanup, nil
}

func (g *GCS) SignUpload(_ context.Context, key string, ttl time.Duration) (usecasestorage.SignedURL, error) {
	return g.sign(key, http.MethodPut, ttl)
}

func (g *GCS) SignDownload(_ context.Context, key string, ttl time.Duration) (usecasestorage.SignedURL, error) {
	return g.sign(key, http.MethodGet, ttl)
}

func (g *GCS) sign(key, method string, ttl time.Duration) (usecasestorage.SignedURL, error) {
	expires := time.Now().Add(ttl)
	opts := &storage.SignedURLOptions{
		Method:  method,
		Expires: expires,
		Scheme:  storage.SigningSchemeV4,
	}
	// Empty signer => production: leave the credentials unset so SignedURL auto-detects the service account and signs via IAM SignBlob.
	if g.signer.privateKey != nil {
		opts.GoogleAccessID = g.signer.accessID
		opts.PrivateKey = g.signer.privateKey
		// The emulator serves plain HTTP, so the signed URL must too; production keeps the HTTPS default.
		opts.Insecure = true
	}
	u, err := g.client.Bucket(g.bucket).SignedURL(key, opts)
	if err != nil {
		return usecasestorage.SignedURL{}, fmt.Errorf("blobstore: sign %s: %w", method, err)
	}
	return usecasestorage.SignedURL{URL: u, Method: method, Expires: expires}, nil
}

func (g *GCS) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	w := g.client.Bucket(g.bucket).Object(key).NewWriter(ctx)
	w.ContentType = contentType
	if _, err := w.Write(data); err != nil {
		//nolint:errcheck // The write already failed; the close error adds nothing.
		_ = w.Close()
		return fmt.Errorf("blobstore: upload write: %w", err)
	}
	// The object is only durably written once Close succeeds, so its error is the authoritative one.
	if err := w.Close(); err != nil {
		return fmt.Errorf("blobstore: upload close: %w", err)
	}
	return nil
}

func (g *GCS) Stat(ctx context.Context, key string) (usecasestorage.ObjectAttrs, error) {
	attrs, err := g.client.Bucket(g.bucket).Object(key).Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return usecasestorage.ObjectAttrs{}, usecasestorage.ErrObjectNotFound
		}
		return usecasestorage.ObjectAttrs{}, fmt.Errorf("blobstore: stat: %w", err)
	}
	return usecasestorage.ObjectAttrs{
		//nolint:gosec // An object size reported by the store is non-negative and bounded; it does not overflow uint64.
		Size: uint64(attrs.Size),
		Hash: hex.EncodeToString(attrs.MD5),
	}, nil
}

func (g *GCS) Delete(ctx context.Context, key string) error {
	err := g.client.Bucket(g.bucket).Object(key).Delete(ctx)
	// An object that is already gone satisfies the caller's intent (absent), so it is not an error here.
	if err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return fmt.Errorf("blobstore: delete: %w", err)
	}
	return nil
}

func ephemeralSigningKey() ([]byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("blobstore: generate signing key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}), nil
}
