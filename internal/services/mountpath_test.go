package services

import (
	"testing"
)

func TestMountPathRegex(t *testing.T) {
	tests := []struct {
		path   string
		valid  bool
		reason string
	}{
		// Unix paths
		{"/config", true, "simple Unix path"},
		{"/var/lib/server", true, "nested Unix path"},
		{"/opt/server/data", true, "Unix path with dash"},
		{"/home/user/server", true, "Unix path with underscore"},
		{"/tmp/server_backup", true, "Unix path with underscore"},
		{"/server_dir", true, "Unix path with underscore"},
		{"/path/with.dots", true, "Unix path with dots"},

		// Windows paths
		{`C:\Users\server`, true, "Windows path with backslashes"},
		{`C:\server\data`, true, "Windows path with colon"},
		{`\\network\share`, true, "UNC path"},
		{`C:\server\config`, true, "Windows path with backslashes"},

		// Docker volume syntax
		{"/host/container", true, "Docker volume syntax"},

		// Invalid paths
		{"", false, "empty path"},
		{"/path\nwith", false, "path with newline"},
		{"/path\twith", false, "path with tab"},
		{"/path$with", false, "path with special char"},
		{"/path;cmd", false, "path with command injection attempt"},
		{"/path|cmd", false, "path with pipe"},
		{"$(whoami)", false, "command injection"},
		{"`whoami`", false, "backtick injection"},
		{"/path\"quote", false, "quoted path"},
		{"/path'quote", false, "single quote"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			matches := mountPathRegex.MatchString(tt.path)
			if matches != tt.valid {
				t.Errorf("Path %q: expected valid=%v, got matches=%v", tt.path, tt.valid, matches)
			}
		})
	}
}
