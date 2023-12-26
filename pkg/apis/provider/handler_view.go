package provider

import (
	"errors"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/util/sets"
)

type (
	GetMetadataRequest struct {
		_ struct{} `route:"GET=/:hostname/:namespace/:type/:action"`

		Hostname  string `path:"hostname"`
		Namespace string `path:"namespace"`
		Type      string `path:"type"`
		Action    string `path:"action"` // Eg. Index.json for list versions, {version}.json for list versioned package.

		Context *gin.Context
	}

	GetMetadataResponse struct {
		Versions sets.Set[string]   `json:"versions,omitempty"`
		Archives map[string]Archive `json:"archives,omitempty"`
	}

	Archive struct {
		URL    string   `json:"url"`
		Hashes []string `json:"hashes"`
	}
)

func (r *GetMetadataRequest) SetGinContext(ctx *gin.Context) {
	r.Context = ctx
}

func (r *GetMetadataRequest) Validate() error {
	if len(r.Action) <= 5 {
		return errors.New("invalid action")
	}

	return nil
}

func (r *GetMetadataRequest) Version() string {
	return r.Action[:len(r.Action)-5]
}

type (
	DownloadArchiveRequest struct {
		_ struct{} `route:"GET=/:hostname/:namespace/:type/download/:archive"`

		Hostname  string `path:"hostname"`
		Namespace string `path:"namespace"`
		Type      string `path:"type"`
		Archive   string `path:"archive"`

		Version string
		OS      string
		Arch    string

		Context *gin.Context
	}
)

func (r *DownloadArchiveRequest) SetGinContext(ctx *gin.Context) {
	r.Context = ctx
}

var regexValidArchive = regexp.MustCompile(
	`^terraform-provider-(?P<type>\w+)_(?P<version>[\w|\\.]+)_(?P<os>[a-z]+)_(?P<arch>[a-z0-9]+)\.zip$`,
)

func (r *DownloadArchiveRequest) Validate() error {
	ps := regexValidArchive.FindStringSubmatch(r.Archive)
	if len(ps) != 5 {
		return errors.New("invalid archive")
	}
	ps = ps[1:]

	if r.Type != ps[0] {
		return errors.New("invalid type")
	}

	r.Version = ps[1]
	r.OS = ps[2]
	r.Arch = ps[3]

	return nil
}

type (
	SyncMetadataRequest struct {
		_ struct{} `route:"PUT=/sync"`

		Timeout time.Duration `query:"timeout,default=2m"`

		Context *gin.Context
	}
)

func (r *SyncMetadataRequest) SetGinContext(ctx *gin.Context) {
	r.Context = ctx
}
