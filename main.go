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
		listOrgMembers(ctx, client, *orgFlag, *outDir)
	}

	if *streamFlag {
		streamEvents(ctx, client, *outDir)
	}
}

func listOrgMembers(ctx context.Context, client *github.Client, org string, outDir string) {
	opts := &github.ListMembersOptions{}
	for {
		members, resp, err := client.Organizations.ListMembers(ctx, org, opts)
		if err != nil {
			log.Fatalf("Failed to list org members: %v", err)
		}

		for _, member := range members {
			processUser(ctx, client, member.GetLogin(), org, outDir)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
}

func streamEvents(ctx context.Context, client *github.Client, outDir string) {
	opts := &github.ListOptions{PerPage: 100}
	for {
		// effectively a 100-row cache
		seen := map[string]bool{}
		events, resp, err := client.Activity.ListEvents(ctx, opts)
		if err != nil {
			if _, ok := err.(*github.RateLimitError); ok {
				log.Println("Rate limit hit. Sleeping for an hour.")
				time.Sleep(time.Hour)
				continue
			}
			log.Fatalf("Failed to list events: %v", err)
		}

		for _, event := range events {
			if event.GetActor() == nil {
				continue
			}

			login := event.GetActor().GetLogin()
			if seen[login] {
				continue
			}
			processUser(ctx, client, event.Actor.GetLogin(), event.GetRepo().GetName(), outDir)
			seen[login] = true
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage

		// Optional: Sleep between requests to avoid rate limiting
		time.Sleep(5 * time.Second)
	}
}

func processUser(ctx context.Context, client *github.Client, username string, repo string, outDir string) {
	// Check if user file already exists
	filePath := filepath.Join(outDir, username+".json")
	if _, err := os.Stat(filePath); err == nil {
		return // File exists, skip processing
	}

	// Fetch public keys
	publicKeys, err := fetchPublicKeys(username)
	if err != nil {
		//		log.Printf("Failed to fetch public keys for %s: %v", username, err)
		publicKeys = []string{}
	}

	// Fetch u details
	u, _, err := client.Users.Get(ctx, username)
	if err != nil {
		log.Printf("Failed to get user details for %s: %v", username, err)
		time.Sleep(15 * time.Second)
		return
	}

	if u.GetType() == "Bot" {
		return
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

	if err := os.WriteFile(filePath, jsonData, 0o644); err != nil {
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
	keys = strings.Split(string(body), "\n")
	return keys, nil
}
