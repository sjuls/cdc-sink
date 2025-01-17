// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package fslogical

import (
	"github.com/cockroachdb/cdc-sink/internal/script"
	"github.com/cockroachdb/cdc-sink/internal/sinktest/all"
	"github.com/cockroachdb/cdc-sink/internal/source/logical"
	"github.com/cockroachdb/cdc-sink/internal/staging/memo"
	"github.com/cockroachdb/cdc-sink/internal/staging/version"
	"github.com/cockroachdb/cdc-sink/internal/target/apply"
	"github.com/cockroachdb/cdc-sink/internal/target/dlq"
	"github.com/cockroachdb/cdc-sink/internal/target/schemawatch"
	"github.com/cockroachdb/cdc-sink/internal/util/applycfg"
	"github.com/cockroachdb/cdc-sink/internal/util/diag"
	"github.com/cockroachdb/cdc-sink/internal/util/stopper"
)

// Injectors from injector.go:

// Start creates a PostgreSQL logical replication loop using the
// provided configuration.
func Start(context *stopper.Context, config *Config) (*FSLogical, error) {
	diagnostics := diag.New(context)
	configs, err := applycfg.ProvideConfigs(diagnostics)
	if err != nil {
		return nil, err
	}
	scriptConfig, err := logical.ProvideUserScriptConfig(config)
	if err != nil {
		return nil, err
	}
	loader, err := script.ProvideLoader(scriptConfig)
	if err != nil {
		return nil, err
	}
	targetSchema := ProvideScriptTarget(config)
	baseConfig, err := logical.ProvideBaseConfig(config, loader)
	if err != nil {
		return nil, err
	}
	targetPool, err := logical.ProvideTargetPool(context, baseConfig, diagnostics)
	if err != nil {
		return nil, err
	}
	watchers, err := schemawatch.ProvideFactory(context, targetPool, diagnostics)
	if err != nil {
		return nil, err
	}
	userScript, err := script.ProvideUserScript(configs, loader, diagnostics, targetSchema, watchers)
	if err != nil {
		return nil, err
	}
	client, err := ProvideFirestoreClient(context, config, userScript)
	if err != nil {
		return nil, err
	}
	targetStatements, err := logical.ProvideTargetStatements(context, baseConfig, targetPool, diagnostics)
	if err != nil {
		return nil, err
	}
	dlqConfig := logical.ProvideDLQConfig(baseConfig)
	dlQs := dlq.ProvideDLQs(dlqConfig, targetPool, watchers)
	appliers, err := apply.ProvideFactory(context, targetStatements, configs, diagnostics, dlQs, targetPool, watchers)
	if err != nil {
		return nil, err
	}
	stagingPool, err := logical.ProvideStagingPool(context, baseConfig, diagnostics)
	if err != nil {
		return nil, err
	}
	stagingSchema, err := logical.ProvideStagingDB(baseConfig)
	if err != nil {
		return nil, err
	}
	memoMemo, err := memo.ProvideMemo(context, stagingPool, stagingSchema)
	if err != nil {
		return nil, err
	}
	checker := version.ProvideChecker(stagingPool, memoMemo)
	factory, err := logical.ProvideFactory(context, appliers, configs, baseConfig, diagnostics, memoMemo, loader, stagingPool, targetPool, watchers, checker)
	if err != nil {
		return nil, err
	}
	tombstones, err := ProvideTombstones(config, client, factory, userScript)
	if err != nil {
		return nil, err
	}
	v, err := ProvideLoops(config, client, factory, memoMemo, stagingPool, tombstones, userScript)
	if err != nil {
		return nil, err
	}
	fsLogical := &FSLogical{
		Diagnostics: diagnostics,
		Loops:       v,
	}
	return fsLogical, nil
}

// Build remaining testable components from a common fixture.
func startLoopsFromFixture(fixture *all.Fixture, config *Config) ([]*logical.Loop, error) {
	baseFixture := fixture.Fixture
	context := baseFixture.Context
	configs := fixture.Configs
	scriptConfig, err := logical.ProvideUserScriptConfig(config)
	if err != nil {
		return nil, err
	}
	loader, err := script.ProvideLoader(scriptConfig)
	if err != nil {
		return nil, err
	}
	diagnostics := diag.New(context)
	targetSchema := ProvideScriptTarget(config)
	baseConfig, err := logical.ProvideBaseConfig(config, loader)
	if err != nil {
		return nil, err
	}
	targetPool, err := logical.ProvideTargetPool(context, baseConfig, diagnostics)
	if err != nil {
		return nil, err
	}
	watchers, err := schemawatch.ProvideFactory(context, targetPool, diagnostics)
	if err != nil {
		return nil, err
	}
	userScript, err := script.ProvideUserScript(configs, loader, diagnostics, targetSchema, watchers)
	if err != nil {
		return nil, err
	}
	client, err := ProvideFirestoreClient(context, config, userScript)
	if err != nil {
		return nil, err
	}
	targetStatements, err := logical.ProvideTargetStatements(context, baseConfig, targetPool, diagnostics)
	if err != nil {
		return nil, err
	}
	dlqConfig := logical.ProvideDLQConfig(baseConfig)
	dlQs := dlq.ProvideDLQs(dlqConfig, targetPool, watchers)
	appliers, err := apply.ProvideFactory(context, targetStatements, configs, diagnostics, dlQs, targetPool, watchers)
	if err != nil {
		return nil, err
	}
	typesMemo := fixture.Memo
	stagingPool, err := logical.ProvideStagingPool(context, baseConfig, diagnostics)
	if err != nil {
		return nil, err
	}
	checker := fixture.VersionChecker
	factory, err := logical.ProvideFactory(context, appliers, configs, baseConfig, diagnostics, typesMemo, loader, stagingPool, targetPool, watchers, checker)
	if err != nil {
		return nil, err
	}
	tombstones, err := ProvideTombstones(config, client, factory, userScript)
	if err != nil {
		return nil, err
	}
	v, err := ProvideLoops(config, client, factory, typesMemo, stagingPool, tombstones, userScript)
	if err != nil {
		return nil, err
	}
	return v, nil
}
