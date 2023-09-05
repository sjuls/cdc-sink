// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package fslogical

import (
	"context"
	"github.com/cockroachdb/cdc-sink/internal/script"
	"github.com/cockroachdb/cdc-sink/internal/sinktest/all"
	"github.com/cockroachdb/cdc-sink/internal/source/logical"
	"github.com/cockroachdb/cdc-sink/internal/staging/applycfg"
	"github.com/cockroachdb/cdc-sink/internal/staging/memo"
	"github.com/cockroachdb/cdc-sink/internal/staging/version"
	"github.com/cockroachdb/cdc-sink/internal/target/apply"
	"github.com/cockroachdb/cdc-sink/internal/target/schemawatch"
	"github.com/cockroachdb/cdc-sink/internal/util/diag"
)

// Injectors from injector.go:

// Start creates a PostgreSQL logical replication loop using the
// provided configuration.
func Start(contextContext context.Context, config *Config) (*FSLogical, func(), error) {
	diagnostics, cleanup := diag.New(contextContext)
	scriptConfig, err := logical.ProvideUserScriptConfig(config)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	loader, err := script.ProvideLoader(scriptConfig)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	baseConfig, err := logical.ProvideBaseConfig(config, loader)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	stagingPool, cleanup2, err := logical.ProvideStagingPool(contextContext, baseConfig, diagnostics)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	stagingSchema, err := logical.ProvideStagingDB(baseConfig)
	if err != nil {
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	configs, cleanup3, err := applycfg.ProvideConfigs(contextContext, diagnostics, stagingPool, stagingSchema)
	if err != nil {
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	targetSchema := ProvideScriptTarget(config)
	targetPool, cleanup4, err := logical.ProvideTargetPool(contextContext, baseConfig, diagnostics)
	if err != nil {
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	watchers, cleanup5, err := schemawatch.ProvideFactory(targetPool, diagnostics)
	if err != nil {
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	userScript, err := script.ProvideUserScript(contextContext, configs, loader, diagnostics, stagingPool, targetSchema, watchers)
	if err != nil {
		cleanup5()
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	client, cleanup6, err := ProvideFirestoreClient(contextContext, config, userScript)
	if err != nil {
		cleanup5()
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	appliers, cleanup7, err := apply.ProvideFactory(configs, diagnostics, targetPool, watchers)
	if err != nil {
		cleanup6()
		cleanup5()
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	memoMemo, err := memo.ProvideMemo(contextContext, stagingPool, stagingSchema)
	if err != nil {
		cleanup7()
		cleanup6()
		cleanup5()
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	checker := version.ProvideChecker(stagingPool, memoMemo)
	factory, err := logical.ProvideFactory(contextContext, appliers, configs, baseConfig, diagnostics, memoMemo, loader, stagingPool, targetPool, watchers, checker)
	if err != nil {
		cleanup7()
		cleanup6()
		cleanup5()
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	tombstones, cleanup8, err := ProvideTombstones(config, client, factory, userScript)
	if err != nil {
		cleanup7()
		cleanup6()
		cleanup5()
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	v, cleanup9, err := ProvideLoops(contextContext, config, client, factory, memoMemo, stagingPool, tombstones, userScript)
	if err != nil {
		cleanup8()
		cleanup7()
		cleanup6()
		cleanup5()
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	fsLogical := &FSLogical{
		Diagnostics: diagnostics,
		Loops:       v,
	}
	return fsLogical, func() {
		cleanup9()
		cleanup8()
		cleanup7()
		cleanup6()
		cleanup5()
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
	}, nil
}

// Build remaining testable components from a common fixture.
func startLoopsFromFixture(fixture *all.Fixture, config *Config) ([]*logical.Loop, func(), error) {
	baseFixture := fixture.Fixture
	contextContext := baseFixture.Context
	configs := fixture.Configs
	scriptConfig, err := logical.ProvideUserScriptConfig(config)
	if err != nil {
		return nil, nil, err
	}
	loader, err := script.ProvideLoader(scriptConfig)
	if err != nil {
		return nil, nil, err
	}
	diagnostics := fixture.Diagnostics
	baseConfig, err := logical.ProvideBaseConfig(config, loader)
	if err != nil {
		return nil, nil, err
	}
	stagingPool, cleanup, err := logical.ProvideStagingPool(contextContext, baseConfig, diagnostics)
	if err != nil {
		return nil, nil, err
	}
	targetSchema := ProvideScriptTarget(config)
	watchers := fixture.Watchers
	userScript, err := script.ProvideUserScript(contextContext, configs, loader, diagnostics, stagingPool, targetSchema, watchers)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	client, cleanup2, err := ProvideFirestoreClient(contextContext, config, userScript)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	appliers := fixture.Appliers
	typesMemo := fixture.Memo
	targetPool, cleanup3, err := logical.ProvideTargetPool(contextContext, baseConfig, diagnostics)
	if err != nil {
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	checker := fixture.VersionChecker
	factory, err := logical.ProvideFactory(contextContext, appliers, configs, baseConfig, diagnostics, typesMemo, loader, stagingPool, targetPool, watchers, checker)
	if err != nil {
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	tombstones, cleanup4, err := ProvideTombstones(config, client, factory, userScript)
	if err != nil {
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	v, cleanup5, err := ProvideLoops(contextContext, config, client, factory, typesMemo, stagingPool, tombstones, userScript)
	if err != nil {
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	return v, func() {
		cleanup5()
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
	}, nil
}
