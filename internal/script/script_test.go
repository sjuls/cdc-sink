// Copyright 2023 The Cockroach Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package script

import (
	"context"
	"embed"
	"fmt"
	"testing"
	"time"

	"github.com/cockroachdb/cdc-sink/internal/sinktest/all"
	"github.com/cockroachdb/cdc-sink/internal/types"
	"github.com/cockroachdb/cdc-sink/internal/util/applycfg"
	"github.com/cockroachdb/cdc-sink/internal/util/ident"
	"github.com/cockroachdb/cdc-sink/internal/util/merge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/*
var testData embed.FS

type mapOptions struct {
	data map[string]string
}

func (o *mapOptions) Set(k, v string) error {
	if o.data == nil {
		o.data = map[string]string{k: v}
	} else {
		o.data[k] = v
	}
	return nil
}

func TestScript(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	fixture, err := all.NewFixture(t)
	r.NoError(err)

	ctx := fixture.Context
	schema := fixture.TargetSchema.Schema()

	size := 2048
	switch fixture.TargetPool.Product {
	case types.ProductMariaDB, types.ProductMySQL:
		size = 512
	default:
		// nothing to do.
	}

	// Create tables that will be referenced by the user-script.
	_, err = fixture.TargetPool.ExecContext(ctx,
		fmt.Sprintf("CREATE TABLE %s.table1(msg VARCHAR(%d) PRIMARY KEY)", schema, size))
	r.NoError(err)
	_, err = fixture.TargetPool.ExecContext(ctx,
		fmt.Sprintf("CREATE TABLE %s.table2(idx INT PRIMARY KEY)", schema))
	r.NoError(err)
	_, err = fixture.TargetPool.ExecContext(ctx,
		fmt.Sprintf("CREATE TABLE %s.all_features(msg VARCHAR(%d) PRIMARY KEY)", schema, size))
	r.NoError(err)
	r.NoError(fixture.Watcher.Refresh(ctx, fixture.TargetPool))

	var opts mapOptions

	s, err := newScriptFromFixture(fixture, &Config{
		FS:       testData,
		MainPath: "/testdata/main.ts",
		Options:  &opts,
	}, TargetSchema(schema))
	r.NoError(err)
	a.Equal(3, s.Sources.Len())
	a.Equal(5, s.Targets.Len())
	a.Equal(map[string]string{"hello": "world"}, opts.data)

	tbl1 := ident.NewTable(schema, ident.New("table1"))
	tbl2 := ident.NewTable(schema, ident.New("table2"))
	tblS := ident.NewTable(schema, ident.New("some_table"))

	if cfg := s.Sources.GetZero(ident.New("expander")); a.NotNil(cfg) {
		a.Equal(tbl1, cfg.DeletesTo)
		mut := types.Mutation{Before: []byte(`{"before":true}`), Data: []byte(`{"msg":true}`)}
		mapped, err := cfg.Dispatch(context.Background(), mut)
		if a.NoError(err) && a.NotNil(mapped) {
			if docs := mapped.GetZero(tbl1); a.Len(docs, 1) {
				a.Equal(`{"before":true}`, string(docs[0].Before))
				a.Equal(`{"dest":"table1","msg":true}`, string(docs[0].Data))
				a.Equal(`[true]`, string(docs[0].Key))
			}

			if docs := mapped.GetZero(tbl2); a.Len(docs, 2) {
				a.Equal(`{"before":true}`, string(docs[0].Before))
				a.Equal(`{"dest":"table2","idx":0,"msg":true}`, string(docs[0].Data))
				a.Equal(`[0]`, string(docs[0].Key))

				a.Equal(`{"before":true}`, string(docs[1].Before))
				a.Equal(`{"dest":"table2","idx":1,"msg":true}`, string(docs[1].Data))
				a.Equal(`[1]`, string(docs[1].Key))
			}
		}
	}

	if cfg := s.Sources.GetZero(ident.New("passthrough")); a.NotNil(cfg) {
		a.Equal(tblS, cfg.DeletesTo)
		mut := types.Mutation{Data: []byte(`{"passthrough":true}`)}
		mapped, err := cfg.Dispatch(context.Background(), mut)
		if a.NoError(err) && a.NotNil(mapped) {
			tbl := ident.NewTable(schema, ident.New("some_table"))
			expanded := mapped.GetZero(tbl)
			if a.Len(expanded, 1) {
				a.Equal(mut, expanded[0])
			}
		}
	}

	if cfg := s.Sources.GetZero(ident.New("recursive")); a.NotNil(cfg) {
		a.True(cfg.Recurse)
	}

	tbl := ident.NewTable(schema, ident.New("all_features"))
	if cfg := s.Targets.GetZero(tbl); a.NotNil(cfg) {
		expectedApply := applycfg.Config{
			CASColumns: []ident.Ident{ident.New("cas0"), ident.New("cas1")},
			Deadlines: ident.MapOf[time.Duration](
				ident.New("dl0"), time.Hour,
				ident.New("dl1"), time.Minute,
			),
			Exprs: ident.MapOf[string](
				ident.New("expr0"), "fnv32($0::BYTES)",
				ident.New("expr1"), "Hello Library!",
			),
			Extras: ident.New("overflow_column"),
			Ignore: ident.MapOf[bool](
				ident.New("ign0"), true,
				ident.New("ign1"), true,
				// The false value is dropped.
			),
			// SourceName not used; that can be handled by the function.
			SourceNames: &ident.Map[applycfg.SourceColumn]{},
		}
		a.True(expectedApply.Equal(&cfg.Config))

		if filter := cfg.Map; a.NotNil(filter) {
			mapped, keep, err := filter(context.Background(), types.Mutation{Data: []byte(`{"hello":"world!"}`)})
			a.NoError(err)
			a.True(keep)
			a.Equal(`{"hello":"world!","msg":"Hello World!","num":42}`, string(mapped.Data))
		}

		if merger := cfg.Merger; a.NotNil(merger) {
			result, err := merger.Merge(context.Background(), &merge.Conflict{
				Before:   merge.NewBagOf(nil, nil, "val", 1),
				Proposed: merge.NewBagOf(nil, nil, "val", 3),
				Target:   merge.NewBagOf(nil, nil, "val", 40),
			})
			a.NoError(err)
			if a.NotNil(result.Apply) {
				if v, ok := result.Apply.Get(ident.New("val")); a.True(ok) {
					a.Equal(int64(42), v)
				}
			}
		}
	}

	// A map function that unconditionally filters all mutations.
	tbl = ident.NewTable(schema, ident.New("drop_all"))
	if cfg := s.Targets.GetZero(tbl); a.NotNil(cfg) {
		if filter := cfg.Map; a.NotNil(filter) {
			_, keep, err := filter(context.Background(), types.Mutation{Data: []byte(`{"hello":"world!"}`)})
			a.NoError(err)
			a.False(keep)
		}
	}

	// A merge function that sends all conflicts to a DLQ.
	tbl = ident.NewTable(schema, ident.New("merge_dlq_all"))
	if cfg := s.Targets.GetZero(tbl); a.NotNil(cfg) {
		if merger := cfg.Merger; a.NotNil(merger) {
			result, err := merger.Merge(context.Background(), &merge.Conflict{
				Before:   merge.NewBagOf(nil, nil, "val", 1),
				Proposed: merge.NewBagOf(nil, nil, "val", 2),
				Target:   merge.NewBagOf(nil, nil, "val", 0),
			})
			a.NoError(err)
			a.Equal(&merge.Resolution{DLQ: "dead"}, result)
		}
	}

	// A merge function that drops all conflicts.
	tbl = ident.NewTable(schema, ident.New("merge_drop_all"))
	if cfg := s.Targets.GetZero(tbl); a.NotNil(cfg) {
		if merger := cfg.Merger; a.NotNil(merger) {
			result, err := merger.Merge(context.Background(), &merge.Conflict{
				Before:   merge.NewBagOf(nil, nil, "val", 1),
				Proposed: merge.NewBagOf(nil, nil, "val", 2),
				Target:   merge.NewBagOf(nil, nil, "val", 0),
			})
			a.NoError(err)
			a.Equal(&merge.Resolution{Drop: true}, result)
		}
	}

	// A merge function that sends to a DLQ on conflict.
	tbl = ident.NewTable(schema, ident.New("merge_or_dlq"))
	if cfg := s.Targets.GetZero(tbl); a.NotNil(cfg) {
		if merger := cfg.Merger; a.NotNil(merger) {
			result, err := merger.Merge(context.Background(), &merge.Conflict{
				Before:   merge.NewBagOf(nil, nil, "val", 1),
				Proposed: merge.NewBagOf(nil, nil, "val", 2),
				Target:   merge.NewBagOf(nil, nil, "val", 0),
			})
			a.NoError(err)
			a.Equal(&merge.Resolution{DLQ: "dead"}, result)
		}
	}
}
