package config_test

import (
	"reflect"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/config"
)

func TestParseAllowedOrigins(t *testing.T) {
	tests := []struct {
		name string
		csv  string
		want []string
	}{
		{"empty falls back to localhost dev default", "", []string{"http://localhost:3000"}},
		{"blank falls back to default", "   ", []string{"http://localhost:3000"}},
		{"single origin", "https://app.example.com", []string{"https://app.example.com"}},
		{
			"comma list is split and trimmed",
			"http://localhost:3000, http://localhost:3002 ,https://app.example.com",
			[]string{"http://localhost:3000", "http://localhost:3002", "https://app.example.com"},
		},
		{"empty entries are dropped", "http://localhost:3002,,", []string{"http://localhost:3002"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.ParseAllowedOrigins(tt.csv)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseAllowedOrigins(%q) = %v, want %v", tt.csv, got, tt.want)
			}
		})
	}
}
