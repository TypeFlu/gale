// 🚀 Gale: GitHub Artifact & Lifecycle Explorer
// A beautiful, modern, and high-performance CLI tool to fetch GitHub releases.
//
// Author: Saksham Singla (@Typeflu)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

var (
	icons = map[string]string{
		"check":    "✔",
		"error":    "✖",
		"warning":  "!",
		"info":     "i",
		"sparkles": "*",
		"gear":     "»",
		"folder":   "→",
	}

	successLog = color.New(color.FgGreen).PrintfFunc()
	errorLog   = color.New(color.FgRed).PrintfFunc()
	warningLog = color.New(color.FgYellow).PrintfFunc()
	infoLog    = color.New(color.FgCyan).PrintfFunc()
	dimLog     = color.New(color.Faint).PrintlnFunc()
	bright     = color.New(color.Bold).SprintFunc()
	cyan       = color.New(color.FgCyan).SprintFunc()
	magenta    = color.New(color.FgMagenta).SprintFunc()
)

const version = "4.5.0"

func showBanner() {
	logoColor := color.New(color.FgCyan)
	dimColor := color.New(color.Faint)

	banner := `
  %s__
  %s/ /  %s GALE %s
 %s/ /___%s A modern CLI to fetch GitHub releases
%s`
	fmt.Printf(banner,
		logoColor.Sprint(" "),
		logoColor.Sprint(""),
		bright(fmt.Sprintf("v%s", version)),
		dimColor.Sprint(""),
		logoColor.Sprint(""),
		dimColor.Sprint(""),
		color.New(color.Reset).Sprint("\n"),
	)
}

func showHelp() {
	fmt.Printf(`
%s:
  gale [owner] [repo] [options]

%s:
  %s                       # Fetch releases for the default repo
  %s microsoft vscode         # Fetch VS Code releases
  %s cli gh --count 20        # Fetch 20 GitHub CLI releases
  %s --help                   # Show this help

%s:
  %s, -c   Number of releases to fetch (default: 10)
  %s, -o   Output file name (default: releases.json)
  %s, -t   GitHub token (or use GITHUB_TOKEN env var)
  %s, -q   Quiet mode (minimal output)
  %s, -h   Show this help
  %s, -v   Show version

%s:
  %s   Your GitHub personal access token
`,
		bright("USAGE"),
		bright("EXAMPLES"),
		cyan("gale"),
		cyan("gale"),
		cyan("gale"),
		cyan("gale"),
		bright("OPTIONS"),
		color.GreenString("--count"),
		color.GreenString("--output"),
		color.GreenString("--token"),
		color.GreenString("--quiet"),
		color.GreenString("--help"),
		color.GreenString("--version"),
		bright("ENVIRONMENT"),
		color.YellowString("GITHUB_TOKEN"),
	)
}

const githubGraphQLQuery = `
query ($owner: String!, $repo: String!, $first: Int!) {
  repository(owner: $owner, name: $repo) {
    releases(first: $first, orderBy: { field: CREATED_AT, direction: DESC }) {
      totalCount
      nodes {
        id
        name
        tagName
        publishedAt
        isPrerelease
        isDraft
        url
        description
        releaseAssets(first: 50) {
          totalCount
          nodes {
            id
            name
            size
            downloadUrl
            contentType
          }
        }
      }
    }
  }
}`

type GraphQLResponse struct {
	Data   *GraphQLData   `json:"data"`
	Errors []GraphQLError `json:"errors"`
}

type GraphQLError struct {
	Message string `json:"message"`
}

type GraphQLData struct {
	Repository *Repository `json:"repository"`
}

type Repository struct {
	Releases Releases `json:"releases"`
}

type Releases struct {
	TotalCount int           `json:"totalCount"`
	Nodes      []ReleaseNode `json:"nodes"`
}

type ReleaseNode struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	TagName       string        `json:"tagName"`
	PublishedAt   time.Time     `json:"publishedAt"`
	IsPrerelease  bool          `json:"isPrerelease"`
	IsDraft       bool          `json:"isDraft"`
	URL           string        `json:"url"`
	Description   string        `json:"description"`
	ReleaseAssets ReleaseAssets `json:"releaseAssets"`
}

type ReleaseAssets struct {
	TotalCount int         `json:"totalCount"`
	Nodes      []AssetNode `json:"nodes"`
}

type AssetNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"downloadUrl"`
	ContentType string `json:"contentType"`
}

type OutputFile struct {
	Metadata   Metadata            `json:"metadata"`
	Repository RepoInfo            `json:"repository"`
	Releases   []NormalizedRelease `json:"releases"`
}

type Metadata struct {
	FetchedAt string `json:"fetchedAt"`
	FetchedBy string `json:"fetchedBy"`
	Author    string `json:"author"`
	URL       string `json:"url"`
}

type RepoInfo struct {
	Owner           string `json:"owner"`
	Repo            string `json:"repo"`
	URL             string `json:"url"`
	TotalReleases   int    `json:"totalReleases"`
	FetchedReleases int    `json:"fetchedReleases"`
}

type NormalizedRelease struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	PublishedAt   time.Time         `json:"publishedAt"`
	IsPrerelease  bool              `json:"isPrerelease"`
	IsDraft       bool              `json:"isDraft"`
	URL           string            `json:"url"`
	Description   string            `json:"description"`
	DownloadCount int               `json:"downloadCount"`
	Assets        []NormalizedAsset `json:"assets"`
}

type NormalizedAsset struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	SizeFormatted string `json:"sizeFormatted"`
	ContentType   string `json:"contentType"`
	DownloadURL   string `json:"downloadUrl"`
}

type Config struct {
	Owner   string
	Repo    string
	Count   int
	Output  string
	Token   string
	Quiet   bool
	Help    bool
	Version bool
}

var httpClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
		MaxIdleConnsPerHost: 10,
	},
	Timeout: 30 * time.Second,
}

func parseArgs() *Config {
	cfg := &Config{}

	flag.IntVar(&cfg.Count, "count", 10, "Number of releases to fetch")
	flag.IntVar(&cfg.Count, "c", 10, "Number of releases to fetch (shorthand)")
	flag.StringVar(&cfg.Output, "output", "releases.json", "Output file name")
	flag.StringVar(&cfg.Output, "o", "releases.json", "Output file name (shorthand)")
	flag.StringVar(&cfg.Token, "token", os.Getenv("GITHUB_TOKEN"), "GitHub token")
	flag.StringVar(&cfg.Token, "t", os.Getenv("GITHUB_TOKEN"), "GitHub token (shorthand)")
	flag.BoolVar(&cfg.Quiet, "quiet", false, "Quiet mode (minimal output)")
	flag.BoolVar(&cfg.Quiet, "q", false, "Quiet mode (shorthand)")
	flag.BoolVar(&cfg.Help, "help", false, "Show help")
	flag.BoolVar(&cfg.Help, "h", false, "Show help (shorthand)")
	flag.BoolVar(&cfg.Version, "version", false, "Show version")
	flag.BoolVar(&cfg.Version, "v", false, "Show version (shorthand)")

	flag.Usage = showHelp // Use our custom help function
	flag.Parse()

	args := flag.Args()
	cfg.Owner = "Typeflu"
	cfg.Repo = "gale"
	if len(args) > 0 {
		cfg.Owner = args[0]
	}
	if len(args) > 1 {
		cfg.Repo = args[1]
	}

	return cfg
}

func fetchGraphQL(ctx context.Context, variables map[string]interface{}, token string) (*GraphQLResponse, error) {
	payload := map[string]interface{}{
		"query":     githubGraphQLQuery,
		"variables": variables,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/graphql", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("gale/%s (+https://github.com/Typeflu)", version))
	if token != "" {
		req.Header.Set("Authorization", "bearer "+token)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to GitHub API: %w", err)
	}
	defer func() {
		if closeErr := res.Body.Close(); closeErr != nil {
			// We can't return this error, but we should log it.
			// Since this is a CLI tool, printing to stderr is acceptable.
			fmt.Fprintf(os.Stderr, "warning: failed to close response body: %v\n", closeErr)
		}
	}()

	if res.StatusCode >= 400 {
		resBody, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("GitHub API responded with status %d: %s", res.StatusCode, string(resBody))
	}

	var result GraphQLResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub API response: %w", err)
	}

	return &result, nil
}

func formatBytes(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func normalizeData(nodes []ReleaseNode) []NormalizedRelease {
	releases := make([]NormalizedRelease, len(nodes))
	for i, node := range nodes {
		name := node.Name
		if name == "" {
			name = node.TagName
		}
		if name == "" {
			name = "Unnamed Release"
		}

		assets := make([]NormalizedAsset, len(node.ReleaseAssets.Nodes))
		for j, asset := range node.ReleaseAssets.Nodes {
			assets[j] = NormalizedAsset{
				ID:            asset.ID,
				Name:          asset.Name,
				Size:          asset.Size,
				SizeFormatted: formatBytes(asset.Size),
				ContentType:   asset.ContentType,
				DownloadURL:   asset.DownloadURL,
			}
		}

		releases[i] = NormalizedRelease{
			ID:            node.ID,
			Name:          name,
			Version:       node.TagName,
			PublishedAt:   node.PublishedAt,
			IsPrerelease:  node.IsPrerelease,
			IsDraft:       node.IsDraft,
			URL:           node.URL,
			Description:   node.Description,
			DownloadCount: node.ReleaseAssets.TotalCount,
			Assets:        assets,
		}
	}
	return releases
}

func run() error {
	cfg := parseArgs()

	if cfg.Help {
		showBanner()
		showHelp()
		return nil
	}

	if cfg.Version {
		fmt.Printf("%s v%s\n", bright("gale"), version)
		return nil
	}

	if !cfg.Quiet {
		showBanner()
	}

	if cfg.Token == "" {
		warningLog("%s No GitHub token provided. Rate limits may be lower.\n", icons["warning"])
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithSuffix(fmt.Sprintf(" Fetching %d releases for %s...", cfg.Count, bright(fmt.Sprintf("%s/%s", cfg.Owner, cfg.Repo)))))
	if !cfg.Quiet {
		s.Start()
	}

	type fetchResult struct {
		response *GraphQLResponse
		err      error
	}
	resultChan := make(chan fetchResult, 1)

	go func() {
		variables := map[string]interface{}{
			"owner": cfg.Owner,
			"repo":  cfg.Repo,
			"first": cfg.Count,
		}
		res, err := fetchGraphQL(context.Background(), variables, cfg.Token)
		resultChan <- fetchResult{response: res, err: err}
	}()

	resultData := <-resultChan
	s.Stop() // Stop the spinner

	if resultData.err != nil {
		return resultData.err
	}
	result := resultData.response

	if len(result.Errors) > 0 {
		var errorMessages string
		for _, e := range result.Errors {
			errorMessages += "- " + e.Message + "\n"
		}
		return fmt.Errorf("GraphQL returned errors:\n%s", errorMessages)
	}

	if result.Data == nil || result.Data.Repository == nil {
		return fmt.Errorf("repository not found or access denied")
	}

	repoData := result.Data.Repository
	releases := normalizeData(repoData.Releases.Nodes)

	if !cfg.Quiet {
		infoLog("%s Found %s releases (%s total)\n", icons["info"], bright(len(releases)), bright(repoData.Releases.TotalCount))
		if len(releases) > 0 {
			latest := releases[0]
			infoLog("%s Latest is %s published on %s\n", icons["sparkles"], magenta(latest.Version), latest.PublishedAt.Format("Jan 02, 2006"))
		}
	}

	output := OutputFile{
		Metadata: Metadata{
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
			FetchedBy: fmt.Sprintf("gale v%s", version),
			Author:    "Saksham Singla (@Typeflu)",
			URL:       "https://github.com/Typeflu",
		},
		Repository: RepoInfo{
			Owner:           cfg.Owner,
			Repo:            cfg.Repo,
			URL:             fmt.Sprintf("https://github.com/%s/%s", cfg.Owner, cfg.Repo),
			TotalReleases:   repoData.Releases.TotalCount,
			FetchedReleases: len(releases),
		},
		Releases: releases,
	}

	outPath, err := filepath.Abs(cfg.Output)
	if err != nil {
		return fmt.Errorf("could not resolve path %q: %w", cfg.Output, err)
	}

	file, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal output JSON: %w", err)
	}

	err = os.WriteFile(outPath, file, 0644)
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %w", cfg.Output, err)
	}

	successLog("\n%s Success! Saved %s releases to %s\n", icons["check"], bright(len(releases)), cyan(cfg.Output))

	if !cfg.Quiet {
		dimLog(fmt.Sprintf("%s %s", icons["folder"], outPath))
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		errorLog("\n%s Error: %v\n", icons["error"], err)
		os.Exit(1)
	}
}
