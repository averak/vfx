// Package fakeblob is an in-memory [usecasestorage.BlobStore] for tests.
//
// It stands in for the object store without a network backend: Sign* echo the key, and Stat reports only what a test recorded with Put (mimicking a completed upload).
package fakeblob

import (
	"context"
	"net/http"
	"sync"
	"time"

	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

type Store struct {
	mu      sync.Mutex
	objects map[string]usecasestorage.ObjectAttrs

	// DeleteErr, when non-nil, makes Delete fail, to exercise the usecase's best-effort blob cleanup.
	DeleteErr error
}

var _ usecasestorage.BlobStore = (*Store)(nil)

func New() *Store {
	return &Store{objects: map[string]usecasestorage.ObjectAttrs{}}
}

// Put records an object as if a client had uploaded it, so a subsequent CommitPlayerFile's Stat finds it.
func (s *Store) Put(key string, attrs usecasestorage.ObjectAttrs) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = attrs
}

func (s *Store) Has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.objects[key]
	return ok
}

func (s *Store) SignUpload(_ context.Context, key string, ttl time.Duration) (usecasestorage.SignedURL, error) {
	return usecasestorage.SignedURL{URL: "https://blob.test/" + key, Method: http.MethodPut, Expires: time.Now().Add(ttl)}, nil
}

func (s *Store) SignDownload(_ context.Context, key string, ttl time.Duration) (usecasestorage.SignedURL, error) {
	return usecasestorage.SignedURL{URL: "https://blob.test/" + key, Method: http.MethodGet, Expires: time.Now().Add(ttl)}, nil
}

func (s *Store) Stat(_ context.Context, key string) (usecasestorage.ObjectAttrs, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attrs, ok := s.objects[key]
	if !ok {
		return usecasestorage.ObjectAttrs{}, usecasestorage.ErrObjectNotFound
	}
	return attrs, nil
}

func (s *Store) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.DeleteErr != nil {
		return s.DeleteErr
	}
	delete(s.objects, key)
	return nil
}
