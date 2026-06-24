package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var (
	Version = "0.1.0-dev"
	Commit  = "unknown"
	Date    = "unknown"
)

type Info struct {
	Version           string     `json:"version"`
	Commit            string     `json:"commit"`
	Date              string     `json:"date"`
	Repository        string     `json:"repository"`
	LatestVersion     string     `json:"latestVersion,omitempty"`
	LatestURL         string     `json:"latestUrl,omitempty"`
	UpdateAvailable   bool       `json:"updateAvailable"`
	SelfUpdateEnabled bool       `json:"selfUpdateEnabled"`
	CheckedAt         *time.Time `json:"checkedAt,omitempty"`
	Error             string     `json:"error,omitempty"`
}

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Name    string `json:"name"`
}

func Current(repository string, selfUpdateEnabled bool) Info {
	return Info{
		Version:           normalize(Version),
		Commit:            Commit,
		Date:              Date,
		Repository:        repository,
		SelfUpdateEnabled: selfUpdateEnabled,
	}
}

func CheckLatest(ctx context.Context, client *http.Client, repository string, selfUpdateEnabled bool) Info {
	info := Current(repository, selfUpdateEnabled)
	if repository == "" || strings.Contains(repository, "OWNER/") {
		info.Error = "GITHUB_REPO is not configured"
		now := time.Now().UTC()
		info.CheckedAt = &now
		return info
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", strings.Trim(repository, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "api-monitor")
	res, err := client.Do(req)
	now := time.Now().UTC()
	info.CheckedAt = &now
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		info.Error = fmt.Sprintf("GitHub status %d", res.StatusCode)
		return info
	}
	var release GitHubRelease
	if err := json.NewDecoder(res.Body).Decode(&release); err != nil {
		info.Error = err.Error()
		return info
	}
	info.LatestVersion = normalize(release.TagName)
	info.LatestURL = release.HTMLURL
	info.UpdateAvailable = compareSimple(info.LatestVersion, info.Version) > 0
	return info
}

func normalize(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "v")
}

func compareSimple(a, b string) int {
	if a == "" || b == "" {
		return 0
	}
	as := splitVersion(a)
	bs := splitVersion(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		av, bv := 0, 0
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return 0
}

func splitVersion(value string) []int {
	value = strings.Split(value, "-")[0]
	parts := strings.Split(value, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n := 0
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				break
			}
			n = n*10 + int(ch-'0')
		}
		out = append(out, n)
	}
	return out
}
