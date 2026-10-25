package pebble

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	crdb "github.com/cockroachdb/pebble/v2"

	"github.com/safing/portmaster/base/database/iterator"
	"github.com/safing/portmaster/base/database/query"
	"github.com/safing/portmaster/base/database/record"
	"github.com/safing/portmaster/base/database/storage"
	"github.com/safing/portmaster/base/log"
)

// Memory limits. Pebble reads through a fixed-size block cache instead of
// mapping the database file, so these bound the memory used by the database
// regardless of its size on disk.
const (
	blockCacheSize = 8 << 20
	memTableSize   = 2 << 20
	maxOpenFiles   = 64

	// batchCommitSize is the number of staged changes after which a batch is
	// committed, in order to bound the memory used by the batch.
	batchCommitSize = 1000
)

// UnsyncedDatabases names databases whose writes skip the fsync. Set it
// before the database opens. Every other database syncs each write, like
// bbolt does, so a power cut or kernel crash cannot drop a saved setting.
// Skipping the sync only suits data that is rebuilt or fetched again: there
// the fsync per write costs battery for nothing.
var UnsyncedDatabases = map[string]bool{}

var errClosed = errors.New("pebble: database is closed")

// Pebble is a storage backed by a pebble database.
type Pebble struct {
	name  string
	db    *crdb.DB
	write *crdb.WriteOptions

	// pebble panics when a closed DB is used, so every use of db holds
	// closeLock for reading and Shutdown holds it for writing.
	closeLock sync.RWMutex
	closed    bool
}

func init() {
	_ = storage.Register("pebble", NewPebble)
}

// NewPebble opens/creates a pebble database.
func NewPebble(name, location string) (storage.Interface, error) {
	write := crdb.Sync
	if UnsyncedDatabases[name] {
		write = crdb.NoSync
	}
	return open(name, location, write)
}

func open(name, location string, write *crdb.WriteOptions) (*Pebble, error) {
	db, err := crdb.Open(filepath.Join(location, "db"), &crdb.Options{
		CacheSize:    blockCacheSize,
		MemTableSize: memTableSize,
		MaxOpenFiles: maxOpenFiles,
		Logger:       &logger{name: name},
	})
	if err != nil {
		return nil, err
	}

	return &Pebble{
		name:  name,
		db:    db,
		write: write,
	}, nil
}

// rLockOpen read-locks closeLock and fails if the database is closed.
// On success, the caller must call closeLock.RUnlock.
func (p *Pebble) rLockOpen() error {
	p.closeLock.RLock()
	if p.closed {
		p.closeLock.RUnlock()
		return errClosed
	}
	return nil
}

// Get returns a database record.
func (p *Pebble) Get(key string) (record.Record, error) {
	if err := p.rLockOpen(); err != nil {
		return nil, err
	}
	defer p.closeLock.RUnlock()

	value, closer, err := p.db.Get([]byte(key))
	if err != nil {
		if errors.Is(err, crdb.ErrNotFound) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	duplicate := make([]byte, len(value))
	copy(duplicate, value)
	_ = closer.Close()

	return record.NewRawWrapper(p.name, key, duplicate)
}

// GetMeta returns the metadata of a database record.
func (p *Pebble) GetMeta(key string) (*record.Meta, error) {
	r, err := p.Get(key)
	if err != nil {
		return nil, err
	}

	return r.Meta(), nil
}

// Put stores a record in the database.
func (p *Pebble) Put(r record.Record) (record.Record, error) {
	data, err := r.MarshalRecord(r)
	if err != nil {
		return nil, err
	}

	if err := p.rLockOpen(); err != nil {
		return nil, err
	}
	defer p.closeLock.RUnlock()

	err = p.db.Set([]byte(r.DatabaseKey()), data, p.write)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// PutMany stores many records in the database.
// Records are committed in chunks, not as one atomic transaction.
func (p *Pebble) PutMany(shadowDelete bool) (chan<- record.Record, <-chan error) {
	batch := make(chan record.Record, 100)
	errs := make(chan error, 1)

	go func() {
		// The batch is only staged in memory, so the lock is held per
		// commit instead of while waiting on the caller to send records.
		b := p.db.NewBatch()
		defer func() {
			_ = b.Close()
		}()

		for r := range batch {
			err := p.batchPutOrDelete(b, shadowDelete, r)
			if err == nil {
				_, err = p.lockedCommitIfFull(b)
			}
			if err != nil {
				errs <- err
				return
			}
		}
		errs <- p.lockedCommit(b)
	}()

	return batch, errs
}

func (p *Pebble) batchPutOrDelete(b *crdb.Batch, shadowDelete bool, r record.Record) (err error) {
	r.Lock()
	defer r.Unlock()

	if !shadowDelete && r.Meta().IsDeleted() {
		// Immediate delete.
		err = b.Delete([]byte(r.DatabaseKey()), nil)
	} else {
		// Put or shadow delete.
		var data []byte
		data, err = r.MarshalRecord(r)
		if err == nil {
			err = b.Set([]byte(r.DatabaseKey()), data, nil)
		}
	}

	return err
}

// Delete deletes a record from the database.
func (p *Pebble) Delete(key string) error {
	if err := p.rLockOpen(); err != nil {
		return err
	}
	defer p.closeLock.RUnlock()

	return p.db.Delete([]byte(key), p.write)
}

// Query returns an iterator for the supplied query.
func (p *Pebble) Query(q *query.Query, local, internal bool) (*iterator.Iterator, error) {
	_, err := q.Check()
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	// queryExecutor releases the lock. The pebble iterator must be closed
	// before the database, so the query holds off Shutdown until it ends.
	if err := p.rLockOpen(); err != nil {
		return nil, err
	}
	queryIter := iterator.New()

	go p.queryExecutor(queryIter, q, local, internal)
	return queryIter, nil
}

func (p *Pebble) queryExecutor(queryIter *iterator.Iterator, q *query.Query, local, internal bool) {
	defer p.closeLock.RUnlock()

	prefix := []byte(q.DatabaseKeyPrefix())
	err := func() error {
		it, err := p.newPrefixIter(prefix)
		if err != nil {
			return err
		}
		defer func() {
			_ = it.Close()
		}()

		for it.First(); it.Valid(); it.Next() {
			value, err := it.ValueAndErr()
			if err != nil {
				return err
			}

			iterWrapper, err := record.NewRawWrapper(p.name, string(it.Key()), value)
			if err != nil {
				return err
			}

			if !iterWrapper.Meta().CheckValidity() {
				continue
			}
			if !iterWrapper.Meta().CheckPermission(local, internal) {
				continue
			}

			if q.MatchesRecord(iterWrapper) {
				duplicate := make([]byte, len(value))
				copy(duplicate, value)

				newWrapper, err := record.NewRawWrapper(p.name, iterWrapper.DatabaseKey(), duplicate)
				if err != nil {
					return err
				}
				select {
				case <-queryIter.Done:
					return nil
				case queryIter.Next <- newWrapper:
				default:
					select {
					case <-queryIter.Done:
						return nil
					case queryIter.Next <- newWrapper:
					case <-time.After(1 * time.Second):
						return errors.New("query timeout")
					}
				}
			}
		}
		return it.Error()
	}()
	queryIter.Finish(err)
}

// ReadOnly returns whether the database is read only.
func (p *Pebble) ReadOnly() bool {
	return false
}

// Injected returns whether the database is injected.
func (p *Pebble) Injected() bool {
	return false
}

// MaintainRecordStates maintains records states in the database.
func (p *Pebble) MaintainRecordStates(ctx context.Context, purgeDeletedBefore time.Time, shadowDelete bool) error {
	if err := p.rLockOpen(); err != nil {
		return err
	}
	defer p.closeLock.RUnlock()

	now := time.Now().Unix()
	purgeThreshold := purgeDeletedBefore.Unix()

	it, err := p.newPrefixIter(nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = it.Close()
	}()

	b := p.db.NewBatch()
	defer func() {
		_ = b.Close()
	}()

	for it.First(); it.Valid(); it.Next() {
		select {
		case <-ctx.Done():
			return b.Commit(p.write)
		default:
		}

		value, err := it.ValueAndErr()
		if err != nil {
			return err
		}

		wrapper, err := record.NewRawWrapper(p.name, string(it.Key()), value)
		if err != nil {
			return err
		}

		meta := wrapper.Meta()
		switch {
		case meta.Deleted == 0 && meta.Expires > 0 && meta.Expires < now:
			if shadowDelete {
				meta.Deleted = meta.Expires
				deleted, err := wrapper.MarshalRecord(wrapper)
				if err != nil {
					return err
				}
				err = b.Set(it.Key(), deleted, nil)
				if err != nil {
					return err
				}
				break
			}

			// Immediately delete expired entries if shadowDelete is disabled.
			fallthrough
		case meta.Deleted > 0 && (!shadowDelete || meta.Deleted < purgeThreshold):
			err = b.Delete(it.Key(), nil)
			if err != nil {
				return err
			}
		}

		_, err = p.commitIfFull(b)
		if err != nil {
			return err
		}
	}
	if err := it.Error(); err != nil {
		return err
	}

	return b.Commit(p.write)
}

// Purge deletes all records that match the given query. It returns the number of successful deletes and an error.
func (p *Pebble) Purge(ctx context.Context, q *query.Query, local, internal, shadowDelete bool) (int, error) {
	if err := p.rLockOpen(); err != nil {
		return 0, err
	}
	defer p.closeLock.RUnlock()

	prefix := []byte(q.DatabaseKeyPrefix())

	it, err := p.newPrefixIter(prefix)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = it.Close()
	}()

	b := p.db.NewBatch()
	defer func() {
		_ = b.Close()
	}()

	var cnt, persistedCnt int
	for it.First(); it.Valid(); it.Next() {
		select {
		case <-ctx.Done():
			err = b.Commit(p.write)
			if err != nil {
				return persistedCnt, err
			}
			return cnt, nil
		default:
		}

		value, err := it.ValueAndErr()
		if err != nil {
			return persistedCnt, err
		}

		wrapper, err := record.NewRawWrapper(p.name, string(it.Key()), value)
		if err != nil {
			return persistedCnt, err
		}

		if !wrapper.Meta().CheckPermission(local, internal) {
			continue
		}
		if wrapper.Meta().IsDeleted() {
			continue
		}
		if !q.MatchesRecord(wrapper) {
			continue
		}

		if shadowDelete {
			wrapper.Meta().Delete()
			deleted, err := wrapper.MarshalRecord(wrapper)
			if err != nil {
				return persistedCnt, err
			}
			err = b.Set(it.Key(), deleted, nil)
			if err != nil {
				return persistedCnt, err
			}
		} else {
			err = b.Delete(it.Key(), nil)
			if err != nil {
				return persistedCnt, err
			}
		}
		cnt++

		committed, err := p.commitIfFull(b)
		if err != nil {
			return persistedCnt, err
		}
		if committed {
			persistedCnt = cnt
		}
	}

	if err := it.Error(); err != nil {
		return persistedCnt, err
	}

	err = b.Commit(p.write)
	if err != nil {
		return persistedCnt, err
	}
	return cnt, nil
}

// Shutdown shuts down the database. It waits for running operations to
// finish; later operations fail with errClosed.
func (p *Pebble) Shutdown() error {
	p.closeLock.Lock()
	defer p.closeLock.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true
	return p.db.Close()
}

// commitIfFull commits and resets the batch if it reached the commit size,
// in order to bound the memory used by the batch. The caller must hold
// closeLock.
func (p *Pebble) commitIfFull(b *crdb.Batch) (committed bool, err error) {
	if b.Count() < batchCommitSize {
		return false, nil
	}

	err = b.Commit(p.write)
	b.Reset()
	return true, err
}

func (p *Pebble) lockedCommitIfFull(b *crdb.Batch) (committed bool, err error) {
	if b.Count() < batchCommitSize {
		return false, nil
	}
	if err := p.rLockOpen(); err != nil {
		return false, err
	}
	defer p.closeLock.RUnlock()

	return p.commitIfFull(b)
}

func (p *Pebble) lockedCommit(b *crdb.Batch) error {
	if err := p.rLockOpen(); err != nil {
		return err
	}
	defer p.closeLock.RUnlock()

	return b.Commit(p.write)
}

// newPrefixIter returns an iterator over all keys with the given prefix.
// The iterator reads from a snapshot, changes made while iterating are not
// visible to it.
func (p *Pebble) newPrefixIter(prefix []byte) (*crdb.Iterator, error) {
	if len(prefix) == 0 {
		return p.db.NewIter(nil)
	}

	return p.db.NewIter(&crdb.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpperBound(prefix),
	})
}

// prefixUpperBound returns the smallest key that is greater than all keys
// with the given prefix, or nil if there is no such key.
func prefixUpperBound(prefix []byte) []byte {
	for i := len(prefix) - 1; i >= 0; i-- {
		if prefix[i] < 0xFF {
			upper := make([]byte, i+1)
			copy(upper, prefix)
			upper[i]++
			return upper
		}
	}
	return nil
}

// logger forwards pebble log messages to the Portmaster log.
type logger struct {
	name string
}

func (l *logger) Infof(format string, args ...any) {
	log.Debugf("database: pebble %s: %s", l.name, fmt.Sprintf(format, args...))
}

func (l *logger) Errorf(format string, args ...any) {
	log.Errorf("database: pebble %s: %s", l.name, fmt.Sprintf(format, args...))
}

// Fatalf is called by pebble when it cannot continue safely.
func (l *logger) Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Criticalf("database: pebble %s: %s", l.name, msg)
	panic("pebble " + l.name + ": " + msg)
}
