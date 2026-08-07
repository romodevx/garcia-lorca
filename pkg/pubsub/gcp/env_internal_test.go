package gcp

import (
	"context"
	"errors"
	"testing"

	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/pkg/pubsub"
)

// The selection is tested against a stubbed client builder rather than a live
// broker: what matters here is the DECISION (dev bus, real client, or refuse to
// start), and only a stub can fail construction on demand — dialling is lazy, so
// a real client is built successfully even with nothing listening.
func TestSelectPublisherDecision(t *testing.T) {
	errBuild := errors.New("no credentials")

	tests := []struct {
		name         string
		emulatorHost string
		projectID    string
		buildErr     error
		wantProject  string // the project the builder must be asked for; "" = not called
		wantLogBus   bool
		wantErr      bool
	}{
		{
			name:       "no broker configured falls back to the dev bus",
			wantLogBus: true,
		},
		{
			name:         "the emulator alone configures the transport",
			emulatorHost: "127.0.0.1:8085",
			wantProject:  defaultProjectID,
		},
		{
			name:        "an explicit project overrides the local default",
			projectID:   "garcia-lorca-prod",
			wantProject: "garcia-lorca-prod",
		},
		{
			name:         "a configured but unbuildable broker fails startup",
			emulatorHost: "127.0.0.1:8085",
			buildErr:     errBuild,
			wantProject:  defaultProjectID,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PUBSUB_EMULATOR_HOST", tt.emulatorHost)
			t.Setenv("PUBSUB_PROJECT_ID", tt.projectID)

			askedFor := ""
			build := func(_ context.Context, projectID string) (*Client, error) {
				askedFor = projectID
				if tt.buildErr != nil {
					return nil, tt.buildErr
				}
				return &Client{}, nil
			}

			publisher, err := selectPublisher(context.Background(), logger.Default(), build)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("selectPublisher() = %T, nil; want an error rather than a silent dev-bus fallback", publisher)
				}
				if !errors.Is(err, tt.buildErr) {
					t.Errorf("error = %v, want the build failure in the chain", err)
				}
				if publisher != nil {
					t.Errorf("selectPublisher() returned %T alongside its error, want nil", publisher)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectPublisher() error = %v", err)
			}
			if askedFor != tt.wantProject {
				t.Errorf("builder asked for project %q, want %q", askedFor, tt.wantProject)
			}

			_, isLogBus := publisher.(*pubsub.LogPublisher)
			if isLogBus != tt.wantLogBus {
				t.Errorf("selectPublisher() = %T, wantLogBus = %v", publisher, tt.wantLogBus)
			}
		})
	}
}
