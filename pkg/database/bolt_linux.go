package database

import (
	"syscall"
	"time"

	bolt "go.etcd.io/bbolt"
)

func getBoltOpts() *bolt.Options {
	return &bolt.Options{
		Timeout:         2 * time.Second,
		PreLoadFreelist: true,
		FreelistType:    bolt.FreelistMapType,
		MmapFlags:       syscall.MAP_POPULATE,
		Mlock:           true,
	}
}
