package configure

import (
	"context"
	"testing"
	"time"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/domain/settings"
)

func TestSaveWritesSecretsBeforeConfig(t *testing.T) {
	store := &fakeConfigStore{config: baseConfig()}
	shell := &fakeShellWriter{}
	service := Service{Configs: store, Shell: shell}
	previews, err := service.Save(context.Background(), store.config, map[string]SecretChange{
		"work": {Value: "secret"},
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if len(previews) != 1 || len(shell.writes) != 1 {
		t.Fatalf("previews=%#v writes=%#v", previews, shell.writes)
	}
	if shell.writes[0].Variable != "WORK_API_KEY" || shell.writes[0].Value != "secret" {
		t.Fatalf("write = %#v", shell.writes[0])
	}
	if !store.saved {
		t.Fatal("config was not saved")
	}
}

func TestSaveWritesDerivedVariablesForEachProvider(t *testing.T) {
	config := baseConfig()
	second := config.Providers["work"]
	second.DefaultModel = "model-b"
	second.EnabledModels = []string{"model-b"}
	config.Providers["second"] = second

	shell := &fakeShellWriter{}
	service := Service{Configs: &fakeConfigStore{config: config}, Shell: shell}
	_, err := service.Save(context.Background(), config, map[string]SecretChange{
		"work":   {Value: "one"},
		"second": {Value: "two"},
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	variables := map[string]string{}
	for _, write := range shell.writes {
		variables[write.Variable] = write.Value
	}
	if variables["WORK_API_KEY"] != "one" || variables["SECOND_API_KEY"] != "two" {
		t.Fatalf("writes = %#v", shell.writes)
	}
}

func TestDiscoverModelsUsesMemorySecret(t *testing.T) {
	resolver := &fakeProviderResolver{
		catalog: fakeCatalog{models: []provider.Model{{ID: "model-b"}, {ID: "model-a"}}},
	}
	service := Service{
		Secrets:  fakeSecretResolver{},
		Registry: resolver,
	}
	models, err := service.DiscoverModels(context.Background(), baseConfig(), "work", "memory-secret")
	if err != nil {
		t.Fatalf("DiscoverModels() error = %v", err)
	}
	if len(models) != 2 || models[0].ID != "model-a" || resolver.secret.Value != "memory-secret" {
		t.Fatalf("models=%#v secret=%#v", models, resolver.secret)
	}
}

func TestRemoveSecretUsesDerivedVariable(t *testing.T) {
	config := baseConfig()
	shell := &fakeShellWriter{}
	service := Service{Shell: shell}
	if _, removed, err := service.RemoveSecret(context.Background(), config, "work"); err != nil || !removed {
		t.Fatalf("RemoveSecret() removed=%v err=%v", removed, err)
	}
	if len(shell.removes) != 1 || shell.removes[0].Variable != "WORK_API_KEY" {
		t.Fatalf("removes = %#v", shell.removes)
	}
}

type fakeConfigStore struct {
	config settings.Config
	saved  bool
}

func (s *fakeConfigStore) Load(context.Context) (settings.Config, error) {
	return s.config, nil
}

func (s *fakeConfigStore) Save(_ context.Context, config settings.Config) error {
	s.config = config
	s.saved = true
	return nil
}

type fakeShellWriter struct {
	writes  []ports.ShellProfileChange
	removes []ports.ShellProfileChange
}

func (*fakeShellWriter) Defaults() (ports.ShellProfileDefaults, error) {
	return ports.ShellProfileDefaults{
		Shell:       "zsh",
		Syntax:      "posix",
		ProfileFile: "/tmp/.zshrc",
	}, nil
}

func (w *fakeShellWriter) Preview(change ports.ShellProfileChange) (ports.ShellProfilePreview, error) {
	return ports.ShellProfilePreview{
		ProfileFile: change.ProfileFile,
		Content:     change.Variable,
	}, nil
}

func (w *fakeShellWriter) Write(change ports.ShellProfileChange) (ports.ShellProfilePreview, error) {
	w.writes = append(w.writes, change)
	return w.Preview(change)
}

func (w *fakeShellWriter) Remove(change ports.ShellProfileChange) (ports.ShellProfilePreview, error) {
	w.removes = append(w.removes, change)
	return w.Preview(change)
}

type fakeSecretResolver struct{}

func (fakeSecretResolver) Resolve(context.Context, provider.ID) (ports.Secret, error) {
	return ports.Secret{Provider: "work", Value: "environment"}, nil
}

type fakeProviderResolver struct {
	catalog ports.ModelCatalog
	secret  ports.Secret
}

func (r *fakeProviderResolver) Resolve(_ settings.Provider, secret ports.Secret) (ports.ResolvedProvider, error) {
	r.secret = secret
	return ports.ResolvedProvider{Catalog: r.catalog}, nil
}

type fakeCatalog struct {
	models []provider.Model
}

func (c fakeCatalog) ListModels(context.Context, provider.Route) ([]provider.Model, error) {
	return c.models, nil
}

func baseConfig() settings.Config {
	config := settings.Default()
	config.ActiveProvider = "work"
	config.ShellEnv = settings.ShellEnv{
		Shell:        "zsh",
		Syntax:       "posix",
		ProfileFile:  "/tmp/.zshrc",
		ManagedBlock: true,
	}
	config.Providers["work"] = settings.Provider{
		Format:         string(provider.FormatOpenAIResponses),
		BaseURL:        "https://example.com",
		DefaultModel:   "model-a",
		EnabledModels:  []string{"model-a"},
		RequestTimeout: 30 * time.Second,
	}
	return config
}

var _ ports.ConfigStore = (*fakeConfigStore)(nil)
var _ ports.ShellProfileWriter = (*fakeShellWriter)(nil)
var _ ports.SecretResolver = fakeSecretResolver{}
var _ ports.ProviderResolver = (*fakeProviderResolver)(nil)
var _ ports.ModelCatalog = fakeCatalog{}
