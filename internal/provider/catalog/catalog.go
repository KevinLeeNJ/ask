package catalog

import (
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
)

const (
	MaxPages  = 100
	MaxModels = 10000
)

func Normalize(models []provider.Model) []provider.Model {
	return provider.NormalizeModels(models)
}
