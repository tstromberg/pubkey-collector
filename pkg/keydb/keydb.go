// pkg/keydb/keydb.go
package keydb

import (
	"encoding/json"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/tstromberg/pubkey-collector/pkg/collect"
)

// Metadata stores information about a public key
type Metadata struct {
	User      string    `json:"user"`
	Repo      string    `json:"repo"`
	Timestamp time.Time `json:"timestamp"`
}

// KeyDB represents a BadgerDB instance for storing SSH public keys
type KeyDB struct {
	db *badger.DB
}

// New creates a new KeyDB instance
func New(path string) (*KeyDB, error) {
	opts := badger.DefaultOptions(path)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	return &KeyDB{db: db}, nil
}

// Close closes the underlying BadgerDB
func (k *KeyDB) Close() error {
	return k.db.Close()
}

// Store adds all public keys from a UserInfo object to the database
func (k *KeyDB) Store(userInfo collect.UserInfo, user string, timestamp time.Time) error {
	metadata := Metadata{
		User:      user,
		Repo:      userInfo.Repo,
		Timestamp: timestamp,
	}

	// Convert metadata to JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	// Store each public key in BadgerDB
	return k.db.Update(func(txn *badger.Txn) error {
		for _, pubKey := range userInfo.PublicKeys {
			if err := txn.Set([]byte(pubKey), metadataJSON); err != nil {
				return err
			}
		}
		return nil
	})
}

// Lookup retrieves metadata for a given public key
func (k *KeyDB) Lookup(pubKey string) (*Metadata, error) {
	var metadata Metadata
	err := k.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(pubKey))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &metadata)
		})
	})
	if err != nil {
		return nil, err
	}
	return &metadata, nil
}

// Count returns the total number of keys in the database
func (k *KeyDB) Count() (int, error) {
	keyCount := 0
	err := k.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Keys only
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			keyCount++
		}
		return nil
	})
	return keyCount, err
}
