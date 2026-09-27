package main

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/comms/internal/adapters/expo"
	"github.com/garcia-lorca/services/comms/internal/adapters/fcm"
)

// PUSH_PROVIDER picks the sender: log by default (no credentials needed), expo
// for real delivery, and a typo fails startup instead of silently not pushing.
func TestSelectSender(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	tests := []struct {
		provider string
		wantExpo bool
		wantErr  bool
	}{
		{provider: "", wantExpo: false},
		{provider: "log", wantExpo: false},
		{provider: "expo", wantExpo: true},
		{provider: "EXPO", wantExpo: true},
		{provider: "fcm", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			sender, err := selectSender(tt.provider, "", log)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantExpo {
				assert.IsType(t, &expo.Sender{}, sender)
			} else {
				assert.IsType(t, &fcm.LogSender{}, sender)
			}
		})
	}
}
