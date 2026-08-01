// Package app is the static composition root for CLIProxyAPI Pro modules.
package app

import (
	"context"
	"fmt"
	"sync"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/host"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/modelpolicy"
	modelconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/modelpolicy/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/proxypool"
	proxyconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/proxypool/config"
	proxyengine "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/proxypool/engine"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/settings"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// App owns the lifecycle and wiring of static Pro modules. It intentionally
// contains no proxy-pool or model-policy business state of its own.
type App struct {
	proxyPool   *proxypool.Service
	modelPolicy *modelpolicy.Service
	closeOnce   sync.Once
}

func New(ctx context.Context, configFilePath, baseProxyURL string) (*App, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	migrated, err := migrateLegacySettings(ctx, configFilePath)
	if err != nil {
		return nil, err
	}
	if migrated {
		baseProxyURL = baseProxyURLFromConfigFile(configFilePath, baseProxyURL)
	}
	store := settings.NewEmbeddedStore()
	proxyPool, err := proxypool.New(ctx, store, host.NewProxyOverride(), baseProxyURL)
	if err != nil {
		return nil, fmt.Errorf("initialize proxy pool module: %w", err)
	}
	modelPolicy, err := modelpolicy.New(ctx, store)
	if err != nil {
		proxyPool.Close()
		return nil, fmt.Errorf("initialize model policy module: %w", err)
	}
	return &App{proxyPool: proxyPool, modelPolicy: modelPolicy}, nil
}

func (a *App) Close() {
	if a == nil {
		return
	}
	a.closeOnce.Do(func() {
		if a.modelPolicy != nil {
			a.modelPolicy.Close()
		}
		if a.proxyPool != nil {
			a.proxyPool.Close()
		}
	})
}

func (a *App) ProxyPool() *proxypool.Service {
	if a == nil {
		return nil
	}
	return a.proxyPool
}

func (a *App) ModelPolicy() *modelpolicy.Service {
	if a == nil {
		return nil
	}
	return a.modelPolicy
}

func (a *App) SetBaseProxyURL(value string) {
	if a != nil && a.proxyPool != nil {
		a.proxyPool.SetBaseProxyURL(value)
	}
}

func (a *App) BaseProxyURL() string {
	if a == nil || a.proxyPool == nil {
		return ""
	}
	return a.proxyPool.BaseProxyURL()
}

func (a *App) SetModelPolicyChangeHandler(handler func(context.Context)) {
	if a != nil && a.modelPolicy != nil {
		a.modelPolicy.SetChangeHandler(handler)
	}
}

func (a *App) ProxyConfig() proxyconfig.Config {
	if a == nil || a.proxyPool == nil {
		return proxyconfig.Default()
	}
	return a.proxyPool.Config()
}

func (a *App) UpdateProxyConfig(ctx context.Context, cfg proxyconfig.Config) error {
	if a == nil || a.proxyPool == nil {
		return fmt.Errorf("proxy pool module is unavailable")
	}
	return a.proxyPool.UpdateConfig(ctx, cfg)
}

func (a *App) ProxyStatus() proxyengine.Status {
	if a == nil || a.proxyPool == nil {
		return proxyengine.Status{LastError: "proxy pool module is unavailable"}
	}
	return a.proxyPool.Status()
}

func (a *App) ProbeProxy(ctx context.Context, nodeID, proxyURL, testURL string) proxyengine.ProbeResult {
	return a.proxyPool.Probe(ctx, nodeID, proxyURL, testURL)
}

func (a *App) ProbeAllProxies(ctx context.Context, concurrency int) []proxyengine.ProbeResult {
	return a.proxyPool.ProbeAll(ctx, concurrency)
}

func (a *App) RecoverProxy(nodeID string) error { return a.proxyPool.Recover(nodeID) }

func (a *App) ResetProxyStats() { a.proxyPool.ResetStats() }

func (a *App) ModelConfig() modelconfig.Config {
	if a == nil || a.modelPolicy == nil {
		cfg, _ := modelconfig.Parse(nil)
		return cfg
	}
	return a.modelPolicy.Config()
}

func (a *App) UpdateModelConfig(ctx context.Context, cfg modelconfig.Config) error {
	if a == nil || a.modelPolicy == nil {
		return fmt.Errorf("model policy module is unavailable")
	}
	return a.modelPolicy.UpdateConfig(ctx, cfg)
}

func (a *App) ModelStatus() modelpolicy.Status {
	if a == nil || a.modelPolicy == nil {
		return modelpolicy.Status{LastError: "model policy module is unavailable"}
	}
	return a.modelPolicy.Status()
}

func (a *App) FilterModels(ctx context.Context, hostCfg *internalconfig.Config, auth *coreauth.Auth, models []*registry.ModelInfo) []*registry.ModelInfo {
	if a == nil || a.modelPolicy == nil {
		return models
	}
	return host.FilterModels(ctx, hostCfg, auth, models, a.modelPolicy)
}
