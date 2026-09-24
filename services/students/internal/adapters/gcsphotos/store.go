// Package gcsphotos implements the students PhotoStore port over a private
// Google Cloud Storage bucket. Locally, STORAGE_EMULATOR_HOST points the same
// client at fake-gcs-server, so the adapter is exercised as it runs in the
// cloud.
//
// The bucket is never exposed: objects are private, have no public URL, and
// the gateway serves their bytes only after its own permission check.
package gcsphotos

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"cloud.google.com/go/storage"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter satisfies the consumer-defined port.
var _ ports.PhotoStore = (*Store)(nil)

// Store keeps photo objects in one bucket.
type Store struct {
	client *storage.Client
	bucket *storage.BucketHandle
	name   string
}

// New opens a client for bucket. Credentials come from the environment (the
// service account on Cloud Run), or none against the emulator.
func New(ctx context.Context, bucket string) (*Store, error) {
	// JSON reads: the same API as writes, and the one the emulator serves
	// without extra host configuration.
	client, err := storage.NewClient(ctx, storage.WithJSONReads())
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "opening the photo store", err)
	}
	return &Store{client: client, bucket: client.Bucket(bucket), name: bucket}, nil
}

// EnsureBucket creates the bucket when it does not exist. It is meant for the
// local emulator only: in the cloud the bucket is Terraform's, with its access
// policy, and the service account may not create buckets at all.
func (s *Store) EnsureBucket(ctx context.Context, project string) error {
	if _, err := s.bucket.Attrs(ctx); err == nil {
		return nil
	} else if !errors.Is(err, storage.ErrBucketNotExist) {
		return glerrors.Wrap(glerrors.KindInternal, "reading the photo bucket", err)
	}
	if err := s.bucket.Create(ctx, project, nil); err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "creating the photo bucket", err)
	}
	return nil
}

// Close releases the client.
func (s *Store) Close() error { return s.client.Close() }

// uploadChunkBytes is the largest photo rounded up to the next 256 KiB, the
// granularity GCS requires of a chunk.
const uploadChunkBytes = domain.MaxPhotoBytes + 256<<10

// Put writes (or replaces) the object at key.
func (s *Store) Put(ctx context.Context, key, contentType string, data []byte) error {
	w := s.bucket.Object(key).NewWriter(ctx)
	w.ContentType = contentType
	// Private and never cached by an intermediary: this is a child's photo.
	w.CacheControl = "private, no-store"
	// One chunk that holds the whole photo: the default would allocate a
	// 16 MiB buffer per upload, and 0 (no buffer) would also turn off the
	// library's retry of a transient 503/429.
	w.ChunkSize = uploadChunkBytes
	if _, err := w.Write(data); err != nil {
		_ = w.Close() // the write already failed; its error is the one that matters
		return glerrors.Wrap(glerrors.KindInternal, "writing the photo", err)
	}
	if err := w.Close(); err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "writing the photo", err)
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
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the photo", err)
	}
	defer func() { _ = r.Close() }()
	// One byte past the ceiling, so an oversized object is noticed, not truncated.
	data, err := io.ReadAll(io.LimitReader(r, domain.MaxPhotoBytes+1))
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the photo", err)
	}
	return data, nil
}

// Delete removes the object at key. A missing object is not an error. A failed
// delete is logged with its key (tenant and pupil ids, no personal data):
// callers may treat it as best-effort, and an unreferenced object is only
// found again by that line.
func (s *Store) Delete(ctx context.Context, key string) error {
	err := s.bucket.Object(key).Delete(ctx)
	if err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		slog.WarnContext(ctx, "photo object not deleted", "bucket", s.name, "key", key, "error", err)
		return glerrors.Wrap(glerrors.KindInternal, "deleting the photo", err)
	}
	return nil
}
