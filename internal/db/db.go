// Package db is the only package that touches go.etcd.io/bbolt. It opens the
// single database file, runs migrations, and exposes repository types that
// return plain domain models — callers never see a *bolt.Tx, *bolt.Bucket, or a
// transaction-scoped byte slice.
package db

import (
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Bucket names — defined once here, never inlined at call sites.
var (
	configBucket   = []byte("config")
	installsBucket = []byte("installs")
)

// buckets lists every top-level bucket created at startup.
func buckets() [][]byte { return [][]byte{configBucket, installsBucket} }

// Store owns the process's single *bolt.DB and hands out repositories. cmd opens
// one Store at startup and injects its repositories into whichever mode runs, so
// no other package needs to import bbolt.
type Store struct {
	db *bolt.DB
}

// Open opens (or creates) the bbolt file at path — always with a Timeout so a
// stale exclusive lock fails fast instead of blocking forever — then ensures all
// buckets exist.
func Open(path string) (*Store, error) {
	bdb, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bbolt at %q: %w", path, err)
	}
	s := &Store{db: bdb}
	if err := s.migrate(); err != nil {
		_ = bdb.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// migrate creates every required bucket idempotently in a single write txn.
func (s *Store) migrate() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, name := range buckets() {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("create bucket %q: %w", name, err)
			}
		}
		return nil
	})
}

// Config returns the singleton-config repository.
func (s *Store) Config() *ConfigRepo { return &ConfigRepo{db: s.db} }

// Installs returns the tracked-installs repository.
func (s *Store) Installs() *InstallRepo { return &InstallRepo{db: s.db} }
