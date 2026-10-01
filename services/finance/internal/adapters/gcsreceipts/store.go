// Package gcsreceipts implements the finance ReceiptStore port over a private
// Google Cloud Storage bucket — the same mechanism as the pupils' photos
// (students/gcsphotos): objects are private, have no public URL, and the
// gateway serves their bytes only after its own permission check
// (expenses:read). Locally, STORAGE_EMULATOR_HOST points the same client at
// fake-gcs-server.
package gcsreceipts

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"cloud.google.com/go/storage"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Compile-time proof the adapter satisfies the consumer-defined port.
var _ ports.ReceiptStore = (*Store)(nil)

// Store keeps receipt objects in one bucket.
type Store struct {
	client *storage.Client
	bucket *storage.BucketHandle
	name   string
}

// New opens a client for bucket. Credentials come from the environment (the
// service account on Cloud Run), or none against the emulator.
func New(ctx context.Context, bucket string) (*Store, error) {
	client, err := storage.NewClient(ctx, storage.WithJSONReads())
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "opening the receipt store", err)
	}
	return &Store{client: client, bucket: client.Bucket(bucket), name: bucket}, nil
}

// EnsureBucket creates the bucket when it does not exist — for the local
// emulator only: in the cloud the bucket is Terraform's.
func (s *Store) EnsureBucket(ctx context.Context, project string) error {
	if _, err := s.bucket.Attrs(ctx); err == nil {
		return nil
	} else if !errors.Is(err, storage.ErrBucketNotExist) {
		return glerrors.Wrap(glerrors.KindInternal, "reading the receipt bucket", err)
	}
	if err := s.bucket.Create(ctx, project, nil); err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "creating the receipt bucket", err)
	}
	return nil
}

// Close releases the client.
func (s *Store) Close() error { return s.client.Close() }

// uploadChunkBytes is the largest receipt rounded up to the next 256 KiB, the
// granularity GCS requires of a chunk: one chunk holds the whole file and the
// library keeps retrying a transient 503/429.
const uploadChunkBytes = domain.MaxReceiptBytes + 256<<10

// Put writes (or replaces) the object at key.
func (s *Store) Put(ctx context.Context, key, contentType string, data []byte) error {
	w := s.bucket.Object(key).NewWriter(ctx)
	w.ContentType = contentType
	// Private and never cached by an intermediary: a supplier's invoice.
	w.CacheControl = "private, no-store"
	w.ChunkSize = uploadChunkBytes
	if _, err := w.Write(data); err != nil {
		_ = w.Close() // the write already failed; its error is the one that matters
		return glerrors.Wrap(glerrors.KindInternal, "writing the receipt", err)
	}
	if err := w.Close(); err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "writing the receipt", err)
	}
	return nil
}

// Get reads the object at key. A missing object is glerrors.ErrNotFound.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	r, err := s.bucket.Object(key).NewReader(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, glerrors.ErrNotFound
	}
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the receipt", err)
	}
	defer func() { _ = r.Close() }()
	// One byte past the ceiling, so an oversized object is noticed, not truncated.
	data, err := io.ReadAll(io.LimitReader(r, domain.MaxReceiptBytes+1))
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the receipt", err)
	}
	return data, nil
}

// Delete removes the object at key; a missing object is not an error. A
// failed delete is logged with its key (ids only, no personal data): callers
// treat it as best-effort, and an unreferenced object is found by that line.
func (s *Store) Delete(ctx context.Context, key string) error {
	err := s.bucket.Object(key).Delete(ctx)
	if err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		slog.WarnContext(ctx, "receipt object not deleted", "bucket", s.name, "key", key, "error", err)
		return glerrors.Wrap(glerrors.KindInternal, "deleting the receipt", err)
	}
	return nil
}
