//go:build !linux

package database

import (
	"time"

	bolt "go.etcd.io/bbolt"
)

func getBoltOpts() *bolt.Options {
	return &bolt.Options{
		Timeout:         2 * time.Second,
		PreLoadFreelist: true,
		FreelistType:    bolt.FreelistMapType,
		MmapFlags:       0,
		Mlock:           true,
	}
}
