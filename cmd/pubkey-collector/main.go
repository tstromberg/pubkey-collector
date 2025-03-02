// The pubkey-collector tool gathers SSH public keys from GitHub users.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v45/github"
	"golang.org/x/oauth2"

	"github.com/tstromberg/pubkey-collector/pkg/collect"
	"github.com/tstromberg/pubkey-collector/pkg/keydb"
)

func main() {
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Fatal("GITHUB_TOKEN environment variable must be set")
	}

	// Define and parse flags
	streamFlag := flag.Bool("stream", false, "Gather active users from GitHub events steam (loops infinitely)")
	orgFlag := flag.String("org", "", "GitHub organization to gather keys from")
	dbPath := flag.String("db", "", "BadgerDB database location")
	flag.Parse()

	// Validate flags - must specify dbPath
	if *dbPath == "" {
		log.Fatal("--db flag must be specified")
	}

	// Initialize database
	db, err := keydb.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// GitHub client setup
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: githubToken})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	if *orgFlag != "" {
		processOrgMembers(ctx, client, *orgFlag, db)
	}

	if *streamFlag {
		processStream(ctx, client, db)
	}
}

// processStream continuously collects user data from the GitHub event stream.
func processStream(ctx context.Context, client *github.Client, db *keydb.KeyDB) {
	for {
		if err := processStreamEvents(ctx, client, db); err != nil {
			if strings.Contains(err.Error(), "rate limit") {
				log.Println("Rate limit hit. Sleeping for 20 minutes.")
				time.Sleep(20 * time.Minute)
			} else {
				log.Printf("Error processing events: %v. Retrying...", err)
				time.Sleep(5 * time.Second)
			}
			continue
		}
		log.Printf("Resting before next events fetch...")
		time.Sleep(1 * time.Second)
	}
}

// processOrgMembers collects and saves public keys for all members of an organization.
func processOrgMembers(ctx context.Context, client *github.Client, org string, db *keydb.KeyDB) {
	log.Printf("Listing members of %s...", org)

	users, err := collect.OrgMembers(ctx, client, org)
	if err != nil {
		log.Fatalf("Failed to list org members: %v", err)
	}

	for _, user := range users {
		storeInDB(user, db)
	}
}

// processStreamEvents collects and saves public keys for users from the GitHub event stream.
func processStreamEvents(ctx context.Context, client *github.Client, db *keydb.KeyDB) error {
	users, err := collect.RecentEvents(ctx, client)
	if err != nil {
		return err
	}

	log.Printf("Processing %d users from events...", len(users))
	for _, user := range users {
		storeInDB(user, db)
	}

	return nil
}

// storeInDB stores a user's public key information in the BadgerDB.
func storeInDB(userInfo *collect.UserInfo, db *keydb.KeyDB) {
	username := userInfo.Username
	if username == "" {
		log.Printf("Cannot determine username, skipping")
		return
	}

	log.Printf("Storing %s from %s (%d keys) to database...", username, userInfo.Repo, len(userInfo.PublicKeys))

	// Store the user info in the database
	if err := db.Store(*userInfo, username, time.Now()); err != nil {
		log.Printf("Failed to store user info for %s: %v", username, err)
	}
}
