package database

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/seal-io/walrus/utils/gopool"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/multierr"
)

// Bolt holds the BoltDB instance.
type Bolt struct {
	m  sync.Mutex
	db *bolt.DB
}

// Run starts the BoltDB instance.
func (b *Bolt) Run(ctx context.Context, dir string, lockMemory bool) (err error) {
	b.m.Lock()

	opts := getBoltOpts()
	opts.Mlock = lockMemory

	b.db, err = bolt.Open(filepath.Join(dir, "metadata.db"), 0o600, opts)
	if err != nil {
		b.m.Unlock()
		return err
	}
	b.m.Unlock()

	var (
		done = ctx.Done()
		down = make(chan error)
	)

	gopool.Go(func() {
		<-done
		down <- multierr.Combine(
			b.db.Sync(),
			b.db.Close(),
		)
	})

	return <-down
}

// GetDriver returns the BoltDB driver.
func (b *Bolt) GetDriver() BoltDriver {
	b.m.Lock()
	defer b.m.Unlock()

	const wait = 100 * time.Millisecond

	// Spinning until db is ready.
	for b.db == nil {
		b.m.Unlock()

		runtime.Gosched()
		time.Sleep(wait)

		b.m.Lock()
	}

	return b.db
}
