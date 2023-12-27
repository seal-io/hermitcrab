package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/seal-io/hermitcrab/pkg/apis/runtime"
	"github.com/seal-io/hermitcrab/pkg/download"
)

type (
	LoadArchiveOptions struct {
		Hostname    string
		Namespace   string
		Type        string
		Filename    string
		Shasum      string
		DownloadURL string
	}

	Archive = runtime.ResponseFile

	// Service holds the operation of provider storage.
	// Takes a look of the filesystem layer structure:
	// {hostname}
	// └── {namespace}
	//  └── {type}
	//   └── terraform-provider-{type}_{version}_{os}_{arch}.zip
	Service interface {
		// LoadArchive loads the archive from the storage.
		LoadArchive(context.Context, LoadArchiveOptions) (Archive, error)
	}
)

func NewService(dir string) (Service, error) {
	providerDir := filepath.Join(dir, "providers")

	err := os.Mkdir(providerDir, 0o700)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}

	impliedDir := os.Getenv("TF_PLUGIN_MIRROR_DIR")
	if impliedDir != "" {
		impliedDir = os.ExpandEnv(impliedDir)
	}

	return &service{
		impliedDir:  impliedDir,
		explicitDir: providerDir,
		downloadCli: download.NewClient(nil),
	}, nil
}

type service struct {
	barriers sync.Map

	impliedDir  string
	explicitDir string
	downloadCli *download.Client
}

func (s *service) LoadArchive(ctx context.Context, opts LoadArchiveOptions) (Archive, error) {
	// Check whether the archive is in the implied directory.
	if s.impliedDir != "" {
		p := filepath.Join(
			s.impliedDir,
			opts.Hostname, opts.Namespace, opts.Type,
			opts.Filename)

		fi, err := os.Stat(p)
		if err != nil {
			if !os.IsNotExist(err) {
				return Archive{}, fmt.Errorf("error stating archive: %w", err)
			}

			goto ExplicitDir
		}

		if fi.IsDir() {
			goto ExplicitDir
		}

		f, err := os.Open(p)
		if err != nil {
			goto ExplicitDir
		}

		return Archive{
			ContentType:   "application/zip",
			ContentLength: fi.Size(),
			Headers: map[string]string{
				"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, fi.Name()),
			},
			Reader: f,
		}, nil
	}

ExplicitDir:
	// Check whether the archive is in the explicit directory.

	d := filepath.Join(s.explicitDir, opts.Hostname, opts.Namespace, opts.Type)
	p := filepath.Join(d, opts.Filename)

	fi, err := os.Stat(p)
	if err != nil {
		if !os.IsNotExist(err) {
			return Archive{}, fmt.Errorf("error stating archive: %w", err)
		}

		err = os.MkdirAll(d, 0o700)
		if err != nil && !os.IsExist(err) {
			return Archive{}, fmt.Errorf("error creating archive directory: %w", err)
		}
	}

	if fi != nil && fi.IsDir() {
		err = os.RemoveAll(p)
		if err != nil {
			return Archive{}, fmt.Errorf("error correcting invalid archive: %w", err)
		}

		fi = nil
	}

	if fi != nil {
		var f *os.File

		f, err := os.Open(p)
		if err != nil {
			return Archive{}, fmt.Errorf("error opening file: %w", err)
		}

		return Archive{
			ContentType:   "application/zip",
			ContentLength: fi.Size(),
			Headers: map[string]string{
				"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, fi.Name()),
			},
			Reader: f,
		}, nil
	}

	var (
		br *barrier
		rd bool
	)
	{
		var v any
		v, rd = s.barriers.LoadOrStore(d, newBarrier())
		br = v.(*barrier)
	}

	br.Lock()

	if rd {
		// Wait for the download to complete.
		br.Wait()

		return s.LoadArchive(ctx, opts)
	}

	defer func() {
		s.barriers.Delete(d)
		br.Done()
	}()

	// Download the archive.
	err = s.downloadCli.Get(ctx, download.GetOptions{
		DownloadURL: opts.DownloadURL,
		Directory:   d,
		Filename:    opts.Filename,
		Shasum:      opts.Shasum,
	})
	if err != nil {
		return Archive{}, fmt.Errorf("error downloading archive: %w", err)
	}

	return s.LoadArchive(ctx, opts)
}

type barrier struct {
	cond *sync.Cond
	done bool
}

func newBarrier() *barrier {
	return &barrier{
		cond: sync.NewCond(&sync.Mutex{}),
	}
}

func (br *barrier) Lock() {
	br.cond.L.Lock()
}

func (br *barrier) Wait() {
	for !br.done {
		br.cond.Wait()
	}
	br.cond.L.Unlock()
}

func (br *barrier) Done() {
	br.done = true
	br.cond.L.Unlock()
	br.cond.Broadcast()
}
