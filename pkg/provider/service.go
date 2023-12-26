package provider

import (
	"fmt"

	"github.com/seal-io/hermitcrab/pkg/database"
	"github.com/seal-io/hermitcrab/pkg/provider/metadata"
	"github.com/seal-io/hermitcrab/pkg/provider/storage"
)

type Service struct {
	Metadata metadata.Service
	Storage  storage.Service
}

func NewService(boltDriver database.BoltDriver, dataSourceDir string) (*Service, error) {
	ms, err := metadata.NewService(boltDriver)
	if err != nil {
		return nil, fmt.Errorf("error creating metadata service: %w", err)
	}

	ss, err := storage.NewService(dataSourceDir)
	if err != nil {
		return nil, fmt.Errorf("error creating storage service: %w", err)
	}

	return &Service{
		Metadata: ms,
		Storage:  ss,
	}, nil
}
