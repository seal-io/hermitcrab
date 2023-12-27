package provider

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin/render"
	"github.com/seal-io/walrus/utils/errorx"
	"github.com/seal-io/walrus/utils/gopool"
	"github.com/seal-io/walrus/utils/log"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/seal-io/hermitcrab/pkg/provider"
	"github.com/seal-io/hermitcrab/pkg/provider/metadata"
	"github.com/seal-io/hermitcrab/pkg/provider/storage"
)

func Handle(service *provider.Service) *Handler {
	return &Handler{
		s: service,
	}
}

type Handler struct {
	m sync.Mutex

	s *provider.Service
}

func (h *Handler) GetMetadata(req GetMetadataRequest) (GetMetadataResponse, error) {
	version := req.Version()

	if version == "index" {
		opts := metadata.GetVersionsOptions{
			Hostname:  req.Hostname,
			Namespace: req.Namespace,
			Type:      req.Type,
		}

		mr, err := h.s.Metadata.GetVersions(req.Context, opts)
		if err != nil {
			return GetMetadataResponse{}, err
		}

		resp := GetMetadataResponse{
			Versions: sets.New[string](),
		}
		for _, v := range mr {
			resp.Versions.Insert(v.Version)
		}

		return resp, nil
	}

	opts := metadata.GetVersionOptions{
		Hostname:  req.Hostname,
		Namespace: req.Namespace,
		Type:      req.Type,
		Version:   version,
	}

	mr, err := h.s.Metadata.GetVersion(req.Context, opts)
	if err != nil {
		return GetMetadataResponse{}, err
	}

	resp := GetMetadataResponse{
		Archives: map[string]Archive{},
	}

	for _, v := range mr.Platforms {
		archiveName := v.OS + "_" + v.Arch

		archive := Archive{
			URL: "download/" + v.Filename,
		}
		if v.Shasum != "" {
			archive.Hashes = []string{
				"zh:" + v.Shasum,
			}
		}

		resp.Archives[archiveName] = archive
	}

	return resp, nil
}

func (h *Handler) DownloadArchive(req DownloadArchiveRequest) (render.Render, error) {
	getPlatformOpts := metadata.GetPlatformOptions{
		Hostname:  req.Hostname,
		Namespace: req.Namespace,
		Type:      req.Type,
		Version:   req.Version,
		OS:        req.OS,
		Arch:      req.Arch,
	}

	mr, err := h.s.Metadata.GetPlatform(req.Context, getPlatformOpts)
	if err != nil {
		return nil, err
	}

	loadOrFetchOpts := storage.LoadArchiveOptions{
		Hostname:    req.Hostname,
		Namespace:   req.Namespace,
		Type:        req.Type,
		Filename:    mr.Filename,
		Shasum:      mr.Shasum,
		DownloadURL: mr.DownloadURL,
	}

	return h.s.Storage.LoadArchive(req.Context, loadOrFetchOpts)
}

func (h *Handler) SyncMetadata(req SyncMetadataRequest) error {
	if !h.m.TryLock() {
		return errorx.HttpErrorf(http.StatusLocked, "previous sync is not finished")
	}

	gopool.Go(func() {
		defer h.m.Unlock()

		logger := log.WithName("apis").WithName("provider").WithName("sync_metadata")

		timeout := req.Timeout
		if timeout == 0 {
			timeout = 2 * time.Minute
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		err := h.s.Metadata.Sync(ctx)
		if err != nil {
			logger.Warnf("error syncing: %v", err)
		}
	})

	return nil
}
