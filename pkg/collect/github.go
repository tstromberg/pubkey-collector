// Package collect provides functionality to collect SSH public keys from GitHub users.
package collect

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v45/github"
)

// UserInfo represents a GitHub user and their SSH public keys.
type UserInfo struct {
	// PublicKeys contains the user's public SSH keys.
	PublicKeys []string `json:"public_keys"`
	// Repo is the repository the user was active in (for event-based collection).
	Repo string `json:"repo,omitempty"`
	// Username is the GitHub username.
	Username string `json:"username"`
}

// OrgMembers retrieves all members of a GitHub organization and their public keys.
func OrgMembers(ctx context.Context, client *github.Client, org string) ([]*UserInfo, error) {
	var allUsers []*UserInfo
	opts := &github.ListMembersOptions{}

	for {
		members, resp, err := client.Organizations.ListMembers(ctx, org, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list org members: %w", err)
		}

		for _, member := range members {
			username := member.GetLogin()
			if username == "" {
				continue
			}

			user, err := processUser(username, org)
			if err == nil && user != nil {
				allUsers = append(allUsers, user)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allUsers, nil
}

// RecentEvents retrieves active users from the GitHub events stream.
func RecentEvents(ctx context.Context, client *github.Client) ([]*UserInfo, error) {
	opts := &github.ListOptions{PerPage: 100}
	seen := map[string]bool{}
	var allUsers []*UserInfo

	events, _, err := client.Activity.ListEvents(ctx, opts)
	if err != nil {
		if _, ok := err.(*github.RateLimitError); ok {
			return nil, fmt.Errorf("rate limit hit: %w", err)
		}
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	for _, event := range events {
		if event.GetActor() == nil {
			continue
		}

		login := event.GetActor().GetLogin()
		if login == "" || seen[login] {
			continue
		}

		// Skip likely bots
		if strings.HasSuffix(login, "bot") || strings.HasSuffix(login, "bot]") {
			continue
		}

		// Small delay to avoid hammering the API
		time.Sleep(50 * time.Millisecond)

		repoName := ""
		if event.GetRepo() != nil {
			repoName = event.GetRepo().GetName()
		}

		user, err := processUser(login, repoName)
		if err == nil && user != nil {
			allUsers = append(allUsers, user)
		}
		seen[login] = true
	}

	return allUsers, nil
}

// processUser fetches public keys for a GitHub user.
func processUser(username, repo string) (*UserInfo, error) {
	if username == "" {
		return nil, fmt.Errorf("empty username")
	}

	// Fetch public keys
	publicKeys, err := fetchPublicKeys(username)
	if err != nil {
		// Return empty keys array rather than failing
		publicKeys = []string{}
	}

	return &UserInfo{
		PublicKeys: publicKeys,
		Repo:       repo,
		Username:   username,
	}, nil
}

// fetchPublicKeys retrieves the public SSH keys for a GitHub user.
func fetchPublicKeys(username string) ([]string, error) {
	log.Printf("fetching public keys: %q", username)
	resp, err := http.Get(fmt.Sprintf("https://github.com/%s.keys", username))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch keys, status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var keys []string
	if len(body) == 0 {
		return keys, nil
	}

	keys = strings.Split(strings.TrimSpace(string(body)), "\n")
	return keys, nil
}
