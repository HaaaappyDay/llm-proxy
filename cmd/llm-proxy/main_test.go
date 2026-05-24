package main

import (
	"strings"
	"testing"
)

func TestVersionStringIncludesBuildMetadata(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	version, commit, date = "v1.2.3", "abc123", "2026-05-23T00:00:00Z"
	defer func() {
		version, commit, date = oldVersion, oldCommit, oldDate
	}()

	got := versionString()
	for _, want := range []string{"v1.2.3", "abc123", "2026-05-23T00:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Fatalf("versionString() = %q, missing %q", got, want)
		}
	}
}

func TestPublicListenWarning(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "127.0.0.1", want: false},
		{host: "localhost", want: false},
		{host: "::1", want: false},
		{host: "0.0.0.0", want: true},
		{host: "192.168.1.10", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := publicListenWarning(tt.host)
			if (got != "") != tt.want {
				t.Fatalf("publicListenWarning(%q) = %q, want warning=%v", tt.host, got, tt.want)
			}
		})
	}
}
