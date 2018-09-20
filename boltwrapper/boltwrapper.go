package boltwrapper

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Bucket we use in the database. No need for multiple buckets.
const bucketName = "ACN"

// Write writes a value to a bbolt database. Note that if the value is a struct,
// only exported fields are written (ie those which start with a capital letter).
func Write(database *bolt.DB, key string, value interface{}) error {
	if database == nil {
		return fmt.Errorf("no database supplied writing %q to bucket %q", key, bucketName)
	}
	if value == nil {
		return fmt.Errorf("no value supplied writing %q to bucket %q in %q", key, bucketName, database.Path())
	}
	marshalled, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal writing %q to bucket %q in %q: %s", key, bucketName, database.Path(), err)
	}

	return database.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return fmt.Errorf("failed creating bucket %q in %s: %s", bucketName, database.Path(), err)
		}
		if err = bucket.Put([]byte(key), marshalled); err != nil {
			return fmt.Errorf("failed writing key %q in bucket %q in %q: %s", key, bucketName, database.Path(), err)
		}
		return nil
	})
}

var (
	ErrNotFound = fmt.Errorf("entry not found in database")
)

// Read reads a value from a bbolt database. We use an out parameter to return the
// value to avoid impossible type-assertions. The caller is expected to know the
// type of value and pass in an appropriately typed variable.
func Read(database *bolt.DB, key string, value interface{}) error {
	if database == nil {
		return fmt.Errorf("no database supplied reading %q from bucket %q", key, bucketName)
	}
	if value == nil {
		return fmt.Errorf("no value supplied reading %q from bucket %q in %q", key, bucketName, database.Path())
	}
	var bytes []byte
	if err := database.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return ErrNotFound
		}
		bytes = bucket.Get([]byte(key))
		if len(bytes) == 0 {
			return ErrNotFound
		}
		return nil
	}); err != nil {
		return err
	}
	if len(bytes) == 0 {
		return ErrNotFound
	}
	if err := json.Unmarshal(bytes, &value); err != nil {
		return fmt.Errorf("failed unmarshalling %q in bucket %q of %q: %s", key, bucketName, database.Path(), err)
	}
	return nil
}

// GetModificationTime returns the UTC time of last modified
func GetModificationTime(file string) (time.Time, error) {
	info, err := os.Stat(file)
	if err != nil {
		log.Printf("os.stat() for file %v failed: %v", file, err)
		return time.Time{}.UTC(), err
	}
	return info.ModTime().UTC(), nil
}
