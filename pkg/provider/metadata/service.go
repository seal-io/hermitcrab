package metadata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/seal-io/walrus/utils/gopool"
	"github.com/seal-io/walrus/utils/json"
	"github.com/seal-io/walrus/utils/log"
	"github.com/seal-io/walrus/utils/pointer"
	"github.com/seal-io/walrus/utils/strs"
	"github.com/tidwall/gjson"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/multierr"

	"github.com/seal-io/hermitcrab/pkg/database"
	"github.com/seal-io/hermitcrab/pkg/registry"
)

var (
	ErrTypedNotFound       = errors.New("typed not found")
	ErrVersionNotFound     = errors.New("version not found")
	ErrPlatformNotFound    = errors.New("platform not found")
	ErrPlatformsIncomplete = errors.New("platforms incomplete")
)

type (
	// GetVersionsOptions holds the options of listing provider versions.
	GetVersionsOptions struct {
		Hostname  string
		Namespace string
		Type      string
	}

	// GetVersionOptions holds the options of getting provider version.
	GetVersionOptions struct {
		Hostname  string
		Namespace string
		Type      string
		Version   string
	}

	// GetPlatformOptions holds the options of getting provider platform.
	GetPlatformOptions struct {
		Hostname  string
		Namespace string
		Type      string
		Version   string
		OS        string
		Arch      string
	}

	// Version holds the information of provider version.
	Version struct {
		Version   string     `json:"version"`
		Platforms []Platform `json:"platforms"`
	}

	// Platform holds the information of provider platform.
	Platform struct {
		OS          string `json:"os"`
		Arch        string `json:"arch"`
		Filename    string `json:"filename"`
		Shasum      string `json:"shasum"`
		DownloadURL string `json:"download_url"`
	}

	// Service holds the operation of providers.
	// Value always be json.RawBytes, takes a look of the bucket structure:
	//
	//	BUCKET(providers)
	//	  BUCKET({hostname}/{namespace}/{type})
	//	    KEY(modified): string, RFC3339 *
	//	    BUCKET({version}):
	//	      KEY(data): struct{
	//	        version: string
	//	        protocols: []string
	//	        platforms: []struct{
	//	          os: string
	//	          arch: string
	//	        }
	//	      }
	//	      BUCKET({platform}):
	//	        KEY(modified): string, RFC3339 *
	//	        KEY(data): {
	//	          protocols: []string
	//	          os: string
	//	          arch: string
	//	          filename: string
	//	          download_url: string
	//	          shasums_url: string
	//	          shasums_signature_url: string
	//	          shasum: string
	//	          signing_keys: {
	//	            gpg_public_keys: []{
	//	              key_id: string
	//	              ascii_armor: string
	//	              trust_signature: string
	//	              source: string
	//	              source_url: string
	//	          }
	//	        }
	//	      }
	Service interface {
		// GetVersions gets the list provider version.
		GetVersions(context.Context, GetVersionsOptions) ([]Version, error)
		// GetVersion gets a specified provider version.
		GetVersion(context.Context, GetVersionOptions) (Version, error)
		// GetPlatform gets detail of a specified provider version.
		GetPlatform(context.Context, GetPlatformOptions) (Platform, error)
		// Sync does synchronization from remote to local.
		Sync(context.Context) error
	}
)

const domain = "providers"

// NewService returns a new metadata service.
func NewService(boltDriver database.BoltDriver) (Service, error) {
	err := boltDriver.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(toBytes(domain))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("error creating providers bucket: %w", err)
	}

	return &service{
		boltDriver: boltDriver,
	}, nil
}

type service struct {
	syncing sync.Map

	boltDriver database.BoltDriver
}

func (s *service) GetVersions(ctx context.Context, opts GetVersionsOptions) ([]Version, error) {
	return s.Query(ctx, QueryOptions{
		Hostname:  opts.Hostname,
		Namespace: opts.Namespace,
		Type:      opts.Type,
	})
}

func (s *service) GetVersion(ctx context.Context, opts GetVersionOptions) (Version, error) {
	if opts.Version == "" {
		return Version{}, errors.New("invalid options")
	}

	versions, err := s.Query(ctx, QueryOptions{
		Hostname:  opts.Hostname,
		Namespace: opts.Namespace,
		Type:      opts.Type,
		Version:   opts.Version,
	})
	if err != nil {
		return Version{}, err
	}

	return versions[0], nil
}

func (s *service) GetPlatform(ctx context.Context, opts GetPlatformOptions) (Platform, error) {
	if opts.Version == "" || opts.OS == "" || opts.Arch == "" {
		return Platform{}, errors.New("invalid options")
	}

	versions, err := s.Query(ctx, QueryOptions(opts))
	if err != nil {
		return Platform{}, err
	}

	return versions[0].Platforms[0], nil
}

// QueryOptions holds the options of querying provider versions.
type QueryOptions struct {
	Hostname  string
	Namespace string
	Type      string
	Version   string
	OS        string
	Arch      string
}

// Query is the underlay of GetVersions, GetVersion and GetPlatform.
func (s *service) Query(ctx context.Context, opts QueryOptions) ([]Version, error) {
	if opts.Hostname == "" || opts.Namespace == "" || opts.Type == "" {
		return nil, errors.New("invalid options")
	}

	var queried []Version

	err := s.boltDriver.View(func(tx *bolt.Tx) error {
		typedBucket := tx.
			Bucket(toBytes(domain)).
			Bucket(toBytes(path.Join(opts.Hostname, opts.Namespace, opts.Type)))
		if typedBucket == nil {
			return ErrTypedNotFound
		}

		// Deep in one version.
		if opts.Version != "" {
			versionBucket := typedBucket.Bucket(toBytes(opts.Version))
			if versionBucket == nil {
				return ErrVersionNotFound
			}

			var version Version

			err := json.Unmarshal(bytes.Clone(versionBucket.Get(toBytes("data"))), &version)
			if err != nil {
				return fmt.Errorf("error unmarshaling version: %w", err)
			}

			// Deep in a platform.
			if opts.OS != "" && opts.Arch != "" {
				platformBucket := versionBucket.Bucket(toBytes(path.Join(opts.OS, opts.Arch)))
				if platformBucket == nil {
					return ErrPlatformNotFound
				}

				var platform Platform

				err = json.Unmarshal(bytes.Clone(platformBucket.Get(toBytes("data"))), &platform)
				if err != nil {
					return fmt.Errorf("error unmarshaling platform: %w", err)
				}

				version.Platforms = []Platform{
					platform,
				}

				queried = []Version{
					version,
				}

				return nil
			}

			// Otherwise, iterate over all available platforms.
			for _, p := range version.Platforms {
				platformBucket := versionBucket.Bucket(toBytes(path.Join(p.OS, p.Arch)))
				if platformBucket == nil {
					return ErrPlatformsIncomplete
				}

				var platform Platform

				err = json.Unmarshal(bytes.Clone(platformBucket.Get(toBytes("data"))), &platform)
				if err != nil {
					return fmt.Errorf("error unmarshaling platform: %w", err)
				}

				version.Platforms = append(version.Platforms, platform)
			}

			queried = []Version{
				version,
			}

			return nil
		}

		// Otherwise, iterate over all versions.
		queried = make([]Version, 0, typedBucket.Stats().BucketN)

		err := typedBucket.ForEachBucket(func(versionBucketName []byte) error {
			versionBucket := typedBucket.Bucket(versionBucketName)

			var version Version

			err := json.Unmarshal(bytes.Clone(versionBucket.Get(toBytes("data"))), &version)
			if err != nil {
				return fmt.Errorf("error unmarshaling version: %w", err)
			}

			queried = append(queried, version)

			return nil
		})
		if err != nil {
			return fmt.Errorf("error iterating over typed bucket: %w", err)
		}

		return nil
	})
	if err == nil {
		return queried, nil
	}

	const wait = 500 * time.Millisecond

	switch {
	case errors.Is(err, ErrPlatformNotFound):
		// Wait a while to get the latest platform.
		if s.isSyncing(path.Join(opts.Hostname, opts.Namespace, opts.Type, opts.Version, opts.OS, opts.Arch)) {
			time.Sleep(wait)
			return s.Query(ctx, opts)
		}

		// Otherwise, sync the platform.
		err = s.syncPlatform(ctx,
			opts.Hostname, opts.Namespace, opts.Type, opts.Version, opts.OS, opts.Arch)
		if err == nil {
			runtime.Gosched()
			return s.Query(ctx, opts)
		}
	case errors.Is(err, ErrPlatformsIncomplete):
		// Wait a while to get the full platforms.
		if s.isSyncing(path.Join(opts.Hostname, opts.Namespace, opts.Type, opts.Version)) {
			time.Sleep(wait)
			return s.Query(ctx, opts)
		}

		// Otherwise, sync all platforms.
		err = s.syncPlatforms(ctx,
			opts.Hostname, opts.Namespace, opts.Type, opts.Version)
		if err == nil {
			runtime.Gosched()
			return s.Query(ctx, opts)
		}
	case errors.Is(err, ErrTypedNotFound):
		// Wait a while to get the latest versions.
		if s.isSyncing(path.Join(opts.Hostname, opts.Namespace, opts.Type)) {
			time.Sleep(wait)
			return s.Query(ctx, opts)
		}

		// Otherwise, sync versions.
		err = s.syncVersions(ctx,
			opts.Hostname, opts.Namespace, opts.Type)
		if err == nil {
			runtime.Gosched()
			return s.Query(ctx, opts)
		}
	}

	return queried, err
}

func (s *service) Sync(ctx context.Context) error {
	typedBucketNames := make([][3][]byte, 0, 64)

	err := s.boltDriver.View(func(tx *bolt.Tx) error {
		sp := []byte("/")

		return tx.Bucket(toBytes(domain)).ForEachBucket(func(k []byte) error {
			keys := bytes.SplitN(bytes.Clone(k), sp, 3)
			if len(keys) == 3 {
				typedBucketNames = append(typedBucketNames, [3][]byte{
					bytes.Clone(keys[0]), // Hostname.
					bytes.Clone(keys[1]), // Namespace.
					bytes.Clone(keys[2]), // Type.
				})
			}

			return nil
		})
	})
	if err != nil {
		return err
	}

	if len(typedBucketNames) == 0 {
		return nil
	}

	const batch = 10
	wg := gopool.Group()

	for i, t := 0, len(typedBucketNames); i < t; {
		j := i + batch
		if j >= t {
			j = t
		}

		func(typedBucketNames [][3][]byte) {
			wg.Go(func() (err error) {
				for k := range typedBucketNames {
					typedBucketName := typedBucketNames[k]

					err = multierr.Append(err,
						s.syncVersions(ctx,
							string(typedBucketName[0]),
							string(typedBucketName[1]),
							string(typedBucketName[2]),
						),
					)
				}

				return err
			})
		}(typedBucketNames[i:j])

		i = j
	}

	return wg.Wait()
}

func (s *service) isSyncing(k string) bool {
	_, syncing := s.syncing.Load(k)
	return syncing
}

func (s *service) syncVersions(ctx context.Context, h, n, t string) error {
	key := path.Join(h, n, t)
	if s.isSyncing(key) {
		return nil
	}

	s.syncing.Store(key, struct{}{})
	defer s.syncing.Delete(key)

	var versions []string

	err := s.boltDriver.Update(func(tx *bolt.Tx) error {
		typedBucket, err := tx.
			Bucket(toBytes(domain)).
			CreateBucketIfNotExists(toBytes(path.Join(h, n, t)))
		if err != nil {
			return fmt.Errorf("error creating typed bucket: %w", err)
		}

		var since time.Time
		if sinceB := typedBucket.Get(toBytes("modified")); len(sinceB) != 0 {
			since, _ = time.Parse(time.RFC3339, string(sinceB))
		}

		versionsB, err := registry.Host(h).
			Provider(ctx).
			GetVersions(ctx, n, t, since)
		if err != nil {
			return fmt.Errorf("error getting remote versions: %w", err)
		}

		if len(versionsB) == 0 {
			_ = typedBucket.Put(toBytes("modified"), toBytes(time.Now().Format(time.RFC3339)))

			return nil
		}

		versionsJ := json.Get(versionsB, "versions")
		versions = make([]string, 0, int(versionsJ.Get("#").Int()))
		versionsJ.ForEach(func(_, versionJ gjson.Result) bool {
			version := versionJ.Get("version").String()
			if version == "" {
				return true
			}

			var versionBucket *bolt.Bucket

			versionBucket, err = typedBucket.CreateBucketIfNotExists(toBytes(version))
			if err != nil {
				err = fmt.Errorf("error creating version bucket: %w", err)
				return false
			}

			err = versionBucket.Put(toBytes("data"), toBytes(versionJ.Raw))
			if err != nil {
				err = fmt.Errorf("error putting version bucket: %w", err)
			}

			if err == nil {
				versions = append(versions, version)
			}

			return err == nil
		})

		if err != nil {
			return fmt.Errorf("error iterating over versions: %w", err)
		}

		_ = typedBucket.Put(toBytes("modified"), toBytes(time.Now().Format(time.RFC3339)))

		return nil
	})
	if err != nil {
		return err
	}

	if len(versions) == 0 {
		return nil
	}

	// Sort versions.
	semvers := make([]*semver.Version, len(versions))
	for i := range versions {
		semvers[i], _ = semver.NewVersion(versions[i])
	}

	sort.Slice(semvers, func(i, j int) bool {
		if semvers[i] != nil && semvers[j] != nil {
			return semvers[i].GreaterThan(semvers[j])
		}

		return false
	})

	// Sync latest platforms in background.
	gopool.Go(func() {
		logger := log.WithName("provider").WithName("metadata")

		if len(semvers) >= 10 {
			semvers = semvers[:10]
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		for i := range semvers {
			if semvers[i] == nil {
				continue
			}

			version := semvers[i].String()

			err := s.syncPlatforms(ctx,
				h, n, t, version)
			if err != nil {
				logger.Errorf("error syncing platforms: %v", err)
				continue
			}

			logger.Debugf("synced platforms: %s/%s/%s/%s", h, n, t, version)
		}
	})

	return nil
}

func (s *service) syncPlatforms(ctx context.Context, h, n, t, v string) error {
	key := path.Join(h, n, t, v)
	if s.isSyncing(key) {
		return nil
	}

	s.syncing.Store(key, struct{}{})
	defer s.syncing.Delete(key)

	var platforms [][2]string

	err := s.boltDriver.View(func(tx *bolt.Tx) error {
		typedBucket := tx.
			Bucket(toBytes(domain)).
			Bucket(toBytes(path.Join(h, n, t)))
		if typedBucket == nil {
			return nil
		}

		versionBucket := typedBucket.Bucket(toBytes(v))
		if versionBucket == nil {
			return nil
		}

		platformsJ := json.Get(bytes.Clone(versionBucket.Get(toBytes("data"))), "platforms")
		platforms = make([][2]string, 0, int(platformsJ.Get("#").Int()))
		platformsJ.ForEach(func(_, platformJ gjson.Result) bool {
			platforms = append(platforms, [2]string{
				platformJ.Get("os").String(),
				platformJ.Get("arch").String(),
			})

			return true
		})

		return nil
	})
	if err != nil {
		return err
	}

	if len(platforms) == 0 {
		return nil
	}

	// Sort platforms.
	sort.Slice(platforms, func(i, j int) bool {
		return platforms[i][0] < platforms[j][0] ||
			platforms[i][0] == platforms[j][0] && platforms[i][1] < platforms[j][1]
	})

	wg := gopool.Group()

	for i := range platforms {
		o, a := platforms[i][0], platforms[i][1]

		wg.Go(func() error {
			return s.syncPlatform(ctx,
				h, n, t, v, o, a)
		})
	}

	return wg.Wait()
}

func (s *service) syncPlatform(ctx context.Context, h, n, t, v, o, a string) error {
	key := path.Join(h, n, t, v, o, a)
	if s.isSyncing(key) {
		return nil
	}

	s.syncing.Store(key, struct{}{})
	defer s.syncing.Delete(key)

	return s.boltDriver.Update(func(tx *bolt.Tx) error {
		typedBucket := tx.
			Bucket(toBytes(domain)).
			Bucket(toBytes(path.Join(h, n, t)))
		if typedBucket == nil {
			return nil
		}

		versionBucket := typedBucket.Bucket(toBytes(v))
		if versionBucket == nil {
			return nil
		}

		platformBucket, err := versionBucket.CreateBucketIfNotExists(toBytes(path.Join(o, a)))
		if err != nil {
			return fmt.Errorf("error creating platform bucket: %w", err)
		}

		var since time.Time
		if sinceB := platformBucket.Get(toBytes("modified")); len(sinceB) != 0 {
			since, _ = time.Parse(time.RFC3339, string(sinceB))
		}

		platformB, err := registry.Host(h).
			Provider(ctx).
			GetPlatform(ctx, n, t, v, o, a, since)
		if err != nil {
			return fmt.Errorf("error getting remote platform: %w", err)
		}

		if len(platformB) == 0 {
			_ = platformBucket.Put(toBytes("modified"), toBytes(time.Now().Format(time.RFC3339)))

			return nil
		}

		err = platformBucket.Put(toBytes("data"), platformB)
		if err != nil {
			return fmt.Errorf("error putting platform bucket: %w", err)
		}

		_ = platformBucket.Put(toBytes("modified"), toBytes(time.Now().Format(time.RFC3339)))

		return nil
	})
}

func toBytes(s string) []byte {
	return strs.ToBytes(pointer.String(s))
}
