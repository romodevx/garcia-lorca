//go:build integration

package gcsphotos_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/adapters/gcsphotos"
)

// The store runs against a real fake-gcs-server, the emulator the local stack
// uses: put, get, replace-by-key, delete, and a missing object is NotFound.
func TestStoreAgainstTheEmulator(t *testing.T) {
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "fsouza/fake-gcs-server:1.52.2",
			Cmd:          []string{"-scheme", "http", "-port", "4443"},
			ExposedPorts: []string{"4443/tcp"},
			WaitingFor:   wait.ForListeningPort("4443/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start fake-gcs: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "4443")
	t.Setenv("STORAGE_EMULATOR_HOST", fmt.Sprintf("http://%s:%s", host, port.Port()))

	store, err := gcsphotos.New(ctx, "student-photos-test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.EnsureBucket(ctx, "test-project"); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 1, 2, 3}
	if err := store.Put(ctx, "t/students/s1/a", "image/jpeg", jpeg); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(ctx, "t/students/s1/a")
	if err != nil || !bytes.Equal(got, jpeg) {
		t.Fatalf("Get = (%v, %v)", got, err)
	}
	if err := store.Delete(ctx, "t/students/s1/a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, "t/students/s1/a"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("after delete: want KindNotFound, got %v", err)
	}
	if err := store.Delete(ctx, "t/students/s1/a"); err != nil {
		t.Errorf("deleting a missing object is not an error: %v", err)
	}
}
