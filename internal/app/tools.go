package app

import (
	"log"
	"net/http"

	sdk "github.com/github/copilot-sdk/go"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/store"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace/copilottools"
	"github.com/SCWPretorius/CONTROL/internal/tools/privileged"
)

type runtimeToolset struct {
	Tools             []sdk.Tool
	PrivilegedEnabled bool
	GoogleEnabled     bool
}

func buildRuntimeToolset(cfg config.Config, logger *log.Logger, auditStore store.PrivilegedToolEventStore) (runtimeToolset, error) {
	logger = defaultLogger(logger)

	privilegedLayer, err := privileged.NewLayer(cfg.Tools.Privileged, cfg.Tools.Runtime, privileged.Options{
		AuditStore: auditStore,
	})
	if err != nil {
		return runtimeToolset{}, err
	}

	toolset := runtimeToolset{
		Tools:             append([]sdk.Tool(nil), privilegedLayer.Tools()...),
		PrivilegedEnabled: true,
	}

	if !cfg.Tools.Google.Enabled() {
		return toolset, nil
	}
	if !cfg.Tools.Google.RuntimeEnabled() {
		logger.Printf("google workspace tools disabled: GOOGLE_OAUTH_ACCESS_TOKEN is not set")
		return toolset, nil
	}

	googleLayer, err := copilottools.NewLayer(cfg.Tools.Google, cfg.Tools.Runtime, nil, &http.Client{
		Timeout: cfg.Tools.Runtime.HTTPTimeout,
	})
	if err != nil {
		return runtimeToolset{}, err
	}

	toolset.Tools = append(toolset.Tools, googleLayer.Tools()...)
	toolset.GoogleEnabled = true
	return toolset, nil
}
