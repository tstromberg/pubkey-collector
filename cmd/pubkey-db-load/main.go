// load pubkey JSON data into BadgerDB
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/tstromberg/pubkey-collector/pkg/collect"
	"github.com/tstromberg/pubkey-collector/pkg/keydb"
)

func main() {
	// Define command-line flags
	dirPath := flag.String("dir", "", "Directory to search for JSON files")
	dbPath := flag.String("db", "", "BadgerDB database location")
	flag.Parse()

	// Validate flags
	if *dirPath == "" || *dbPath == "" {
		fmt.Println("Both -dir and -db flags are required")
		flag.Usage()
		os.Exit(1)
	}

	// Open KeyDB
	db, err := keydb.New(*dbPath)
	if err != nil {
		fmt.Printf("Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Process JSON files
	err = filepath.Walk(*dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-JSON files
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
			return nil
		}

		log.Printf("Processing %s", path)
		// Read and parse JSON file
		data, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", path, err)
			return nil // Continue with next file
		}

		var userInfo collect.UserInfo
		if err := json.Unmarshal(data, &userInfo); err != nil {
			fmt.Printf("Error parsing JSON in file %s: %v\n", path, err)
			return nil // Continue with next file
		}

		// Extract the base filename without extension (user)
		baseName := filepath.Base(path)
		baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))

		// Get file modification time
		fileInfo, err := os.Stat(path)
		if err != nil {
			fmt.Printf("Error getting file info for %s: %v\n", path, err)
			return nil
		}
		modTime := fileInfo.ModTime()

		// Store user info in database
		if err := db.Store(userInfo, baseName, modTime); err != nil {
			log.Printf("Error storing data from file %s: %v\n", path, err)
		}

		return nil
	})
	if err != nil {
		log.Printf("Error walking directory: %v\n", err)
		os.Exit(1)
	}

	// Count the total number of keys in the database
	keyCount, err := db.Count()
	if err != nil {
		log.Printf("Error counting keys: %v\n", err)
	}

	log.Printf("Processing completed successfully. Total keys in database: %d", keyCount)
}
