package main

import (
	"flag"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	testCases := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"Zero bytes", 0, "0 B"},
		{"Bytes", 500, "500 B"},
		{"Kilobytes", 1536, "1.5 KB"},
		{"Megabytes", 1572864, "1.5 MB"},
		{"Gigabytes", 1610612736, "1.5 GB"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatBytes(tc.bytes); got != tc.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tc.bytes, got, tc.expected)
			}
		})
	}
}

func TestNormalizeData(t *testing.T) {
	input := []ReleaseNode{
		{
			ID:          "1",
			Name:        "Release",
			TagName:     "v4.4.0",
			PublishedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			ReleaseAssets: ReleaseAssets{
				Nodes: []AssetNode{
					{ID: "asset1", Name: "asset1.zip", Size: 1024},
				},
			},
		},
		{
			ID:      "2",
			TagName: "v4.4.1",
			Name:    "",
		},
	}

	expected := []NormalizedRelease{
		{
			ID:          "1",
			Name:        "Release",
			Version:     "v4.4.0",
			PublishedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Assets: []NormalizedAsset{
				{ID: "asset1", Name: "asset1.zip", Size: 1024, SizeFormatted: "1.0 KB"},
			},
		},
		{
			ID:      "2",
			Name:    "v4.4.1",
			Version: "v4.4.1",
			Assets:  []NormalizedAsset{},
		},
	}

	result := normalizeData(input)

	if len(result) != len(expected) {
		t.Fatalf("Expected %d releases, but got %d", len(expected), len(result))
	}

	for i := range result {
		if result[i].ID != expected[i].ID || result[i].Name != expected[i].Name {
			t.Errorf("Mismatch in release %d. Got %+v, expected %+v", i, result[i], expected[i])
		}
		if len(result[i].Assets) != len(expected[i].Assets) {
			t.Errorf("Mismatch in asset count for release %d.", i)
		}
	}
}

func TestParseArgs(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	oldFlagSet := flag.CommandLine
	defer func() { flag.CommandLine = oldFlagSet }()

	testCases := []struct {
		name     string
		args     []string
		expected *Config
	}{
		{
			name: "Defaults",
			args: []string{"cmd"},
			expected: &Config{
				Owner:  "Typeflu",
				Repo:   "gale",
				Count:  10,
				Output: "releases.json",
				Token:  os.Getenv("GITHUB_TOKEN"),
			},
		},
		{
			name: "Owner and Repo",
			args: []string{"cmd", "microsoft", "vscode"},
			expected: &Config{
				Owner:  "microsoft",
				Repo:   "vscode",
				Count:  10,
				Output: "releases.json",
				Token:  os.Getenv("GITHUB_TOKEN"),
			},
		},
		{
			name: "All flags",
			args: []string{"cmd", "--count", "20", "-o", "out.json", "-q", "owner", "repo"},
			expected: &Config{
				Owner:  "owner",
				Repo:   "repo",
				Count:  20,
				Output: "out.json",
				Quiet:  true,
				Token:  os.Getenv("GITHUB_TOKEN"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			flag.CommandLine = flag.NewFlagSet(tc.name, flag.ExitOnError)
			os.Args = tc.args
			cfg := parseArgs()

			if !reflect.DeepEqual(cfg, tc.expected) {
				t.Errorf("parseArgs() = %+v, want %+v", cfg, tc.expected)
			}
		})
	}
}
