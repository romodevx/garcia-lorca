package usecase_test

import (
	"context"
	"testing"

	"github.com/garcia-lorca/services/iam/internal/usecase"
)

func TestHealthServicePing(t *testing.T) {
	svc := usecase.NewHealthService("iam")

	got, err := svc.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Service != "iam" {
		t.Errorf("Service = %q, want iam", got.Service)
	}
	if got.Status != "SERVING" {
		t.Errorf("Status = %q, want SERVING", got.Status)
	}
}
