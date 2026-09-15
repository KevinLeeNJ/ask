package configure

import (
	"context"
	"strings"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/config/validation"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

type SecretChange struct {
	Value     string
	Overwrite bool
}

type Service struct {
	Configs  ports.ConfigStore
	Secrets  ports.SecretResolver
	Registry ports.ProviderResolver
	Shell    ports.ShellProfileWriter
	Routes   ports.ModelRouteStore
}

func (s *Service) Load(ctx context.Context) (settings.Config, error) {
	return s.Configs.Load(ctx)
}

func (s *Service) Save(
	ctx context.Context,
	config settings.Config,
	secrets map[string]SecretChange,
) ([]ports.ShellProfilePreview, error) {
	normalized, err := validation.Normalize(config)
	if err != nil {
		return nil, err
	}
	if err := validation.Validate(normalized); err != nil {
		return nil, err
	}
	if err := s.ensureShellEnv(&normalized); err != nil {
		return nil, err
	}
	if err := validateSecretConflicts(normalized, secrets); err != nil {
		return nil, err
	}

	previews := make([]ports.ShellProfilePreview, 0, len(secrets))
	written := make(map[string]struct{})
	for providerID, change := range secrets {
		_, ok := normalized.Providers[providerID]
		if !ok {
			return nil, failure.New(
				failure.KindConfig,
				"config.active_provider_unknown",
				map[string]string{"provider": providerID},
			)
		}
		envVar := provider.EnvVar(provider.ID(providerID))
		key := envVar + "\x00" + change.Value
		if _, exists := written[key]; exists {
			continue
		}
		profileChange := ports.ShellProfileChange{
			Shell:        normalized.ShellEnv.Shell,
			ProfileFile:  normalized.ShellEnv.ProfileFile,
			Variable:     envVar,
			Value:        change.Value,
			ManagedBlock: normalized.ShellEnv.ManagedBlock,
			Overwrite:    change.Overwrite,
		}
		preview, err := s.Shell.Write(profileChange)
		if err != nil {
			return nil, err
		}
		previews = append(previews, preview)
		written[key] = struct{}{}
	}
	if err := s.Configs.Save(ctx, normalized); err != nil {
		return previews, err
	}
	return previews, nil
}

func (s *Service) PreviewSecret(
	config settings.Config,
	providerID string,
	change SecretChange,
) (ports.ShellProfilePreview, error) {
	if err := s.ensureShellEnv(&config); err != nil {
		return ports.ShellProfilePreview{}, err
	}
	_, ok := config.Providers[providerID]
	if !ok {
		return ports.ShellProfilePreview{}, failure.New(
			failure.KindConfig,
			"config.active_provider_unknown",
			map[string]string{"provider": providerID},
		)
	}
	return s.Shell.Preview(ports.ShellProfileChange{
		Shell:        config.ShellEnv.Shell,
		ProfileFile:  config.ShellEnv.ProfileFile,
		Variable:     provider.EnvVar(provider.ID(providerID)),
		Value:        change.Value,
		ManagedBlock: config.ShellEnv.ManagedBlock,
		Overwrite:    change.Overwrite,
	})
}

func (s *Service) DiscoverModels(
	ctx context.Context,
	config settings.Config,
	providerID string,
	memorySecret string,
) ([]provider.Model, error) {
	profile, ok := config.Providers[providerID]
	if !ok {
		return nil, failure.New(
			failure.KindConfig,
			"config.active_provider_unknown",
			map[string]string{"provider": providerID},
		)
	}
	secret := ports.Secret{Provider: provider.ID(providerID), EnvVar: provider.EnvVar(provider.ID(providerID))}
	if strings.TrimSpace(memorySecret) != "" {
		secret.Value = memorySecret
	} else {
		resolved, err := s.Secrets.Resolve(ctx, provider.ID(providerID))
		if err != nil {
			return nil, err
		}
		secret = resolved
	}
	resolvedProvider, err := s.Registry.Resolve(profile, secret)
	if err != nil {
		return nil, err
	}
	if resolvedProvider.Catalog == nil {
		return nil, failure.New(failure.KindProtocol, "catalog.unsupported", nil)
	}
	models, err := resolvedProvider.Catalog.ListModels(
		ctx,
		provider.Route{Provider: provider.ID(providerID), Model: profile.DefaultModel},
	)
	if err != nil {
		return nil, err
	}
	return provider.NormalizeModels(models), nil
}

func (s *Service) RecentRoutes(ctx context.Context, limit int) ([]provider.Route, error) {
	if s.Routes == nil {
		return nil, nil
	}
	return s.Routes.RecentRoutes(ctx, limit)
}

func (s *Service) RemoveSecret(
	ctx context.Context,
	config settings.Config,
	providerID string,
) (ports.ShellProfilePreview, bool, error) {
	if _, ok := config.Providers[providerID]; !ok {
		return ports.ShellProfilePreview{}, false, nil
	}
	for id := range config.Providers {
		if id != providerID && provider.EnvVar(provider.ID(id)) == provider.EnvVar(provider.ID(providerID)) {
			return ports.ShellProfilePreview{}, false, nil
		}
	}
	if err := s.ensureShellEnv(&config); err != nil {
		return ports.ShellProfilePreview{}, false, err
	}
	preview, err := s.Shell.Remove(ports.ShellProfileChange{
		Shell:        config.ShellEnv.Shell,
		ProfileFile:  config.ShellEnv.ProfileFile,
		Variable:     provider.EnvVar(provider.ID(providerID)),
		ManagedBlock: config.ShellEnv.ManagedBlock,
	})
	return preview, true, err
}

func (s *Service) ensureShellEnv(config *settings.Config) error {
	if strings.TrimSpace(config.ShellEnv.Shell) != "" && strings.TrimSpace(config.ShellEnv.ProfileFile) != "" {
		return nil
	}
	defaults, err := s.Shell.Defaults()
	if err != nil {
		return err
	}
	config.ShellEnv = settings.ShellEnv{
		Shell:        defaults.Shell,
		Syntax:       defaults.Syntax,
		ProfileFile:  defaults.ProfileFile,
		ManagedBlock: true,
	}
	return nil
}

func validateSecretConflicts(config settings.Config, secrets map[string]SecretChange) error {
	values := map[string]string{}
	for providerID, change := range secrets {
		if _, ok := config.Providers[providerID]; !ok {
			continue
		}
		envVar := provider.EnvVar(provider.ID(providerID))
		if existing, ok := values[envVar]; ok && existing != change.Value {
			return failure.New(
				failure.KindConfig,
				"shell.variable_conflict",
				map[string]string{
					"variable": envVar,
					"path":     config.ShellEnv.ProfileFile,
				},
			)
		}
		values[envVar] = change.Value
	}
	return nil
}
