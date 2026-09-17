package index

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// StatsDir is where the mirrored tables live in the repository. The nightly
// workflow passes the same path to `od3 index --data`.
const StatsDir = "data/statistics"

// History is what the repository recorded about one dataset's table
type History struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Changes []Change `json:"changes"`
}

// Change is one recorded change of a mirrored table
type Change struct {
	Date    string `json:"date"`
	Commit  string `json:"commit"`
	Message string `json:"message"`
	// Diff holds the change's rows in unified format, only when asked for. It
	// carries a note instead when the change was too large for GitHub to
	// return a patch.
	Diff string `json:"diff,omitempty"`
}

// History reports how a dataset's mirrored table changed, newest first. Only
// statistics datasets are mirrored; for anything else it says so rather than
// reporting an empty history. Asking for diffs costs one request per change on
// top of the listing.
func (c *Catalog) History(ctx context.Context, id string, limit int, withDiff bool) (*History, error) {
	dataset, err := c.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	name := statsFile(dataset)
	if name == "" {
		return nil, fmt.Errorf("dataset %s is not mirrored, only statistics tables are", id)
	}
	path := StatsDir + "/" + name

	query := url.Values{"path": {path}, "per_page": {fmt.Sprint(limit)}}
	var commits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message   string `json:"message"`
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := c.repoJSON(ctx, "/commits?"+query.Encode(), &commits); err != nil {
		return nil, err
	}

	history := &History{ID: dataset.ID, Title: dataset.Title, Changes: make([]Change, len(commits))}
	for i, commit := range commits {
		date, _, _ := strings.Cut(commit.Commit.Committer.Date, "T")
		message, _, _ := strings.Cut(commit.Commit.Message, "\n")
		history.Changes[i] = Change{Date: date, Commit: commit.SHA, Message: message}
		if !withDiff {
			continue
		}
		if history.Changes[i].Diff, err = c.patch(ctx, commit.SHA, path); err != nil {
			return nil, err
		}
	}
	return history, nil
}

// patch returns the part of a commit that touched one file
func (c *Catalog) patch(ctx context.Context, sha, path string) (string, error) {
	var commit struct {
		Files []struct {
			Filename string `json:"filename"`
			Patch    string `json:"patch"`
		} `json:"files"`
	}
	if err := c.repoJSON(ctx, "/commits/"+sha, &commit); err != nil {
		return "", err
	}
	for _, f := range commit.Files {
		if f.Filename != path {
			continue
		}
		if f.Patch == "" {
			return "this change was too large for GitHub to return a diff", nil
		}
		return f.Patch, nil
	}
	return "", nil
}

func (c *Catalog) repoJSON(ctx context.Context, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.RepoAPI+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.cfg.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.GitHubToken)
	}
	resp, err := (&http.Client{Timeout: c.cfg.RequestTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// The unauthenticated limit of 60 requests per hour is the one users hit
		if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return fmt.Errorf("GitHub's rate limit is exhausted, set GITHUB_TOKEN to raise it")
		}
		return fmt.Errorf("reading history: %s returned %s", c.cfg.RepoAPI+path, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, into)
}
