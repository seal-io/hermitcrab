package database

import bolt "go.etcd.io/bbolt"

type BoltDriver interface {
	// Begin starts a new transaction.
	// Multiple read-only transactions can be used concurrently but only one
	// write transaction can be used at a time. Starting multiple write transactions
	// will cause the calls to block and be serialized until the current write
	// transaction finishes.
	//
	// Transactions should not be dependent on one another. Opening a read
	// transaction and a write transaction in the same goroutine can cause the
	// writer to deadlock because the database periodically needs to re-mmap itself
	// as it grows and it cannot do that while a read transaction is open.
	//
	// If a long running read transaction (for example, a snapshot transaction) is
	// needed, you might want to set DB.InitialMmapSize to a large enough value
	// to avoid potential blocking of write transaction.
	//
	// IMPORTANT: You must close read-only transactions after you are finished or
	// else the database will not reclaim old pages.
	Begin(writable bool) (*bolt.Tx, error)

	// Update executes a function within the context of a read-write managed transaction.
	// If no error is returned from the function then the transaction is committed.
	// If an error is returned then the entire transaction is rolled back.
	// Any error that is returned from the function or returned from the commit is
	// returned from the Update() method.
	//
	// Attempting to manually commit or rollback within the function will cause a panic.
	Update(fn func(*bolt.Tx) error) error

	// View executes a function within the context of a managed read-only transaction.
	// Any error that is returned from the function is returned from the View() method.
	//
	// Attempting to manually rollback within the function will cause a panic.
	View(fn func(*bolt.Tx) error) error

	// Batch calls fn as part of a batch. It behaves similar to Update,
	// except:
	//
	// 1. Concurrent Batch calls can be combined into a single Bolt
	// transaction.
	//
	// 2. The function passed to Batch may be called multiple times,
	// regardless of whether it returns error or not.
	//
	// This means that Batch function side effects must be idempotent and
	// take permanent effect only after a successful return is seen in
	// caller.
	//
	// The maximum batch size and delay can be adjusted with DB.MaxBatchSize
	// and DB.MaxBatchDelay, respectively.
	//
	// Batch is only useful when there are multiple goroutines calling it.
	Batch(fn func(*bolt.Tx) error) error

	// Sync executes fdatasync() against the database file handle.
	//
	// This is not necessary under normal operation, however, if you use NoSync
	// then it allows you to force the database file to sync against the disk.
	Sync() error

	// Stats retrieves ongoing performance stats for the database.
	// This is only updated when a transaction closes.
	Stats() bolt.Stats

	// Info is for internal access to the raw data bytes from the C cursor, use
	// carefully, or not at all.
	Info() *bolt.Info

	// IsReadOnly returns whether the database is opened in read-only mode.
	IsReadOnly() bool

	// Path returns the path to the file backing the database.
	Path() string
}
