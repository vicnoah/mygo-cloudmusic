// Command plugin-directory generates the website's catalog of public GitHub plugins.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"waygu-music/internal/platformcatalog"
	"waygu-music/internal/pluginregistry"
)

type target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type websitePlugin struct {
	Repo        string   `json:"repo"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Avatar      string   `json:"avatar,omitempty"`
	Stars       int      `json:"stars"`
	Official    bool     `json:"official"`
	Bundled     bool     `json:"bundled,omitempty"`
	Version     string   `json:"version,omitempty"`
	Targets     []target `json:"targets,omitempty"`
}

type repository struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Stars       int    `json:"stargazers_count"`
	Owner       struct {
		Avatar string `json:"avatar_url"`
	} `json:"owner"`
}

type github struct {
	client *http.Client
	token  string
}

func (g github) get(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com"+endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	response, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub %s returned HTTP %d", endpoint, response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(dst)
}

func (g github) search(ctx context.Context) ([]repository, error) {
	var repos []repository
	for page := 1; page <= 10; page++ {
		query := url.Values{
			"q":        {"topic:" + platformcatalog.DiscoveryTopic + " archived:false is:public fork:true"},
			"sort":     {"stars"},
			"per_page": {"100"},
			"page":     {fmt.Sprint(page)},
		}
		var result struct {
			Incomplete bool         `json:"incomplete_results"`
			Items      []repository `json:"items"`
		}
		if err := g.get(ctx, "/search/repositories?"+query.Encode(), &result); err != nil {
			return nil, err
		}
		if result.Incomplete {
			return nil, errors.New("GitHub returned an incomplete plugin search")
		}
		repos = append(repos, result.Items...)
		if len(result.Items) < 100 {
			break
		}
	}
	return repos, nil
}

func (g github) verifyAssets(ctx context.Context, repo string, release pluginregistry.Release) ([]target, error) {
	_, tag, _ := pluginregistry.GitHubReleaseAsset(release.Artifacts[0].URL)
	var published struct {
		Draft      bool
		Prerelease bool
		Assets     []struct {
			Name, State, Digest string
			Size                int64
		}
	}
	if err := g.get(ctx, "/repos/"+repo+"/releases/tags/"+url.PathEscape(tag), &published); err != nil {
		return nil, err
	}
	if published.Draft || published.Prerelease {
		return nil, errors.New("plugin release is not public and stable")
	}
	var targets []target
	for _, artifact := range release.Artifacts {
		_, _, name := pluginregistry.GitHubReleaseAsset(artifact.URL)
		present := slices.ContainsFunc(published.Assets, func(asset struct {
			Name, State, Digest string
			Size                int64
		}) bool {
			return asset.Name == name && asset.State == "uploaded" && asset.Size == artifact.Size &&
				(asset.Digest == "" || strings.EqualFold(asset.Digest, "sha256:"+artifact.SHA256))
		})
		if !present {
			return nil, fmt.Errorf("published archive is missing or differs from the signed manifest: %s", name)
		}
		targets = append(targets, target{OS: artifact.OS, Arch: artifact.Arch})
	}
	return targets, nil
}

func run(ctx context.Context, g github) ([]websitePlugin, error) {
	registry, err := pluginregistry.New(os.TempDir(), g.client)
	if err != nil {
		return nil, err
	}
	official, err := registry.Catalog(ctx, platformcatalog.OfficialCatalog, false)
	if err != nil {
		return nil, err
	}
	expected := map[string]bool{platformcatalog.BundledRepository: true}
	for _, plugin := range official.Packages {
		if repo, _, _ := pluginregistry.GitHubReleaseAsset(plugin.Manifest); repo != "" {
			expected[repo] = true
		}
	}
	repos, err := g.search(ctx)
	if err != nil {
		return nil, err
	}
	plugins := make([]websitePlugin, 0, len(repos))
	seen := make(map[string]bool)
	for _, found := range repos {
		repo, err := pluginregistry.GitHubRepository(found.FullName)
		if err != nil || seen[repo] {
			continue
		}
		plugin := websitePlugin{Repo: repo, Description: found.Description, Avatar: found.Owner.Avatar, Stars: found.Stars}
		if repo == platformcatalog.BundledRepository {
			plugin.Name, plugin.Official, plugin.Bundled = "飞牛音乐", true, true
		} else {
			review, err := registry.ReviewGitHub(ctx, repo, false)
			if err == nil {
				plugin.Targets, err = g.verifyAssets(ctx, repo, review.Release)
			}
			if err != nil {
				if expected[repo] {
					return nil, fmt.Errorf("checking official plugin %s: %w", repo, err)
				}
				log.Printf("skip %s: %v", repo, err)
				continue
			}
			plugin.Name, plugin.Version = review.Release.Name, review.Release.Version
			plugin.Official = review.Source.Catalog == platformcatalog.OfficialCatalog
			if plugin.Name == "" {
				plugin.Name = review.Source.Package
			}
		}
		seen[repo] = true
		plugins = append(plugins, plugin)
	}
	for repo := range expected {
		if !seen[repo] {
			return nil, fmt.Errorf("GitHub search is missing the official plugin %s", repo)
		}
	}
	return plugins, nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	// Never send the publishing token through redirects or to plugin assets.
	api := &http.Client{
		Timeout: 12 * time.Second,
		CheckRedirect: func(req *http.Request, previous []*http.Request) error {
			if len(previous) >= 8 {
				return errors.New("too many GitHub redirects")
			}
			req.Header.Del("Authorization")
			return nil
		},
	}
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	plugins, err := run(ctx, github{client: api, token: token})
	if err != nil {
		log.Fatal(err)
	}
	output := json.NewEncoder(os.Stdout)
	output.SetIndent("", "  ")
	if err := output.Encode(plugins); err != nil {
		log.Fatal(err)
	}
}
