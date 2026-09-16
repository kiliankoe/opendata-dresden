package config

import (
	"testing"
	"time"
)

func TestLoadConfigRequestTimeout(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want time.Duration
	}{
		{"default", "", 30 * time.Second},
		{"override", "1500", 1500 * time.Millisecond},
		{"invalid falls back", "soon", 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("REQUEST_TIMEOUT_MS", tt.env)
			if got := LoadConfig().RequestTimeout; got != tt.want {
				t.Errorf("RequestTimeout = %v, want %v", got, tt.want)
			}
		})
	}
}
