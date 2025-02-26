package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v45/github"
	"golang.org/x/oauth2"
)

type UserInfo struct {
	PublicKeys []string `json:"public_keys"`
	Repo       string
	GitHub     *github.User
}

func main() {
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Fatal("GITHUB_TOKEN environment variable must be set")
	}

	// Flags
	streamFlag := flag.Bool("stream", false, "Stream GitHub events")
	userInfoFlag := flag.Bool("userinfo", false, "Fetch GitHub user information (requires more API calls)")
	orgFlag := flag.String("org", "", "GitHub organization to list members")
	outDir := flag.String("out", ".", "Output directory for JSON files")

	flag.Parse()

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// GitHub client setup
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: githubToken})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	if *orgFlag != "" {
		listOrgMembers(ctx, client, *orgFlag, *outDir, *userInfoFlag)
	}

	if *streamFlag {
		for {
			streamEvents(ctx, client, *outDir, *userInfoFlag)
			log.Printf("resting ...")
			time.Sleep(1 * time.Second)
		}
	}
}

func listOrgMembers(ctx context.Context, client *github.Client, org string, outDir string, fetchUser bool) {
	opts := &github.ListMembersOptions{}
	for {
		members, resp, err := client.Organizations.ListMembers(ctx, org, opts)
		if err != nil {
			log.Fatalf("Failed to list org members: %v", err)
		}

		for _, member := range members {
			processUser(ctx, client, member.GetLogin(), org, outDir, fetchUser)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
}

func streamEvents(ctx context.Context, client *github.Client, outDir string, fetchUser bool) {
	opts := &github.ListOptions{PerPage: 100}
	seen := map[string]bool{}
	for {
		events, resp, err := client.Activity.ListEvents(ctx, opts)
		if err != nil {
			if _, ok := err.(*github.RateLimitError); ok {
				log.Println("Rate limit hit. Sleeping for 20 minutes.")
				time.Sleep(20 * time.Minute)
				continue

			}
			log.Fatalf("Failed to list events: %v", err)
		}

		log.Printf("processing %d events ...", len(events))
		for _, event := range events {
			if event.GetActor() == nil {
				continue
			}

			// log.Printf("kind: %s", event.GetType())
			login := event.GetActor().GetLogin()
			if seen[login] {
				continue
			}
			time.Sleep(50 * time.Millisecond)
			processUser(ctx, client, event.Actor.GetLogin(), event.GetRepo().GetName(), outDir, fetchUser)
			seen[login] = true
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage

		// Optional: Sleep between requests to avoid rate limiting
		log.Printf("sleep before page %d", opts.Page)
		time.Sleep(250 * time.Millisecond)
	}
}

func processUser(ctx context.Context, client *github.Client, username string, repo string, outDir string, fetchUser bool) {
	if username == "" {
		return
	}
	subdir := username
	if len(username) > 2 {
		subdir = strings.ToLower(username[0:2])
	}

	// Check if user file already exists
	path := filepath.Join(outDir, subdir, username+".json")
	if _, err := os.Stat(path); err == nil {
		//		log.Printf("exists: %s", username)
		return // File exists, skip processing
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		log.Printf("mkdir failed: %v", err)
	}

	// Fetch public keys
	// log.Printf("Fetching keys for %s ...", username)
	publicKeys, err := fetchPublicKeys(username)
	if err != nil {
		//		log.Printf("Failed to fetch public keys for %s: %v", username, err)
		publicKeys = []string{}
	}

	// Fetch u details
	// log.Printf("Fetching info for %s ...", username)
	var u *github.User
	if fetchUser {
		u, _, err := client.Users.Get(ctx, username)
		if err != nil {
			log.Printf("Failed to get user details for %s: %v", username, err)
			time.Sleep(15 * time.Second)
			return
		}

		if u.GetType() == "Bot" {
			return
		}
	} else {
		if strings.HasSuffix(username, "bot") || strings.HasSuffix(username, "bot]") {
			return
		}
	}

	userInfo := UserInfo{
		PublicKeys: publicKeys,
		Repo:       repo,
		GitHub:     u,
	}

	log.Printf("saving %s from %s (%d keys) ...", username, repo, len(publicKeys))

	// Write user info to JSON file
	jsonData, err := json.MarshalIndent(userInfo, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal user info for %s: %v", username, err)
		return
	}

	if err := os.WriteFile(path, jsonData, 0o644); err != nil {
		log.Printf("Failed to write user file for %s: %v", username, err)
	}
}

func fetchPublicKeys(username string) ([]string, error) {
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
		return nil, nil
	}
	keys = strings.Split(strings.TrimSpace(string(body)), "\n")
	return keys, nil
}
