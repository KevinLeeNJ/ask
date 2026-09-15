package bootstrap

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/KevinLeeNJ/ask/internal/auth"
	"github.com/KevinLeeNJ/ask/internal/cli/command"
	"github.com/KevinLeeNJ/ask/internal/cli/exitcode"
	"github.com/KevinLeeNJ/ask/internal/cli/flags"
	configtoml "github.com/KevinLeeNJ/ask/internal/config/toml"
	"github.com/KevinLeeNJ/ask/internal/i18n/catalog"
	"github.com/KevinLeeNJ/ask/internal/i18n/locale"
	"github.com/KevinLeeNJ/ask/internal/platform/clock"
	"github.com/KevinLeeNJ/ask/internal/platform/id"
	"github.com/KevinLeeNJ/ask/internal/provider/capability"
	"github.com/KevinLeeNJ/ask/internal/provider/registry"
	"github.com/KevinLeeNJ/ask/internal/shellenv/profilefile"
	"github.com/KevinLeeNJ/ask/internal/storage/sqlite"
)

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, parseErr := flags.Parse(args)
	language := locale.Resolve(options.LanguageOverride, "auto")
	translator := catalog.New(language)

	if parseErr == nil && (options.Command == flags.CommandHelp || options.Command == flags.CommandVersion) {
		return command.NewRunner(command.Dependencies{}).Run(ctx, args, stdin, stdout, stderr)
	}

	configPath, err := configtoml.DefaultPath()
	if err != nil {
		fmt.Fprintf(stderr, "ask: %s\n", translator.Error(err))
		return exitcode.FromError(err)
	}
	store := configtoml.NewStore(configPath)
	client := &http.Client{Timeout: 0}
	providerRegistry := registry.New(client)

	dependencies := command.Dependencies{
		Configs:  store,
		Secrets:  auth.EnvironmentResolver{},
		Registry: providerRegistry,
		Shell:    profilefile.Writer{},
		Clock:    clock.System{},
		IDs:      id.UUIDv7{},
	}

	if parseErr == nil &&
		(options.Command == flags.CommandAsk ||
			options.Command == flags.CommandConversations ||
			options.Command == flags.CommandConfig) {
		databasePath, err := sqlite.DefaultPath()
		if err != nil {
			fmt.Fprintf(stderr, "ask: %s\n", translator.Error(err))
			return exitcode.FromError(err)
		}
		repository, err := sqlite.Open(databasePath)
		if err != nil {
			fmt.Fprintf(stderr, "ask: %s\n", translator.Error(err))
			return exitcode.FromError(err)
		}
		defer repository.Close()
		dependencies.Conversations = repository
		dependencies.Leases = repository
		dependencies.Retention = repository
		dependencies.Routes = repository
		dependencies.Capabilities = capability.New(client, repository)
	}

	return command.NewRunner(dependencies).Run(ctx, args, stdin, stdout, stderr)
}
