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

package logical

import (
	"context"

	"github.com/cockroachdb/cdc-sink/internal/types"
	"github.com/cockroachdb/cdc-sink/internal/util/ident"
	"github.com/cockroachdb/cdc-sink/internal/util/stamp"
	"github.com/pkg/errors"
)

// serialEvents is a transaction-preserving implementation of Events.
type serialEvents struct {
	appliers   types.Appliers
	loop       *loop
	targetPool *types.TargetPool
}

var _ Events = (*serialEvents)(nil)

// Backfill implements Events. It delegates to the enclosing loop.
func (e *serialEvents) Backfill(_ context.Context, source string, backfiller Backfiller) error {
	return e.loop.doBackfill(source, backfiller)
}

// GetConsistentPoint implements State. It delegates to the loop.
func (e *serialEvents) GetConsistentPoint() (stamp.Stamp, <-chan struct{}) {
	return e.loop.GetConsistentPoint()
}

// GetTargetDB implements State. It delegates to the loop.
func (e *serialEvents) GetTargetDB() ident.Schema { return e.loop.GetTargetDB() }

// OnBegin implements Events.
func (e *serialEvents) OnBegin(ctx context.Context) (Batch, error) {
	tx, err := e.targetPool.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &serialBatch{e, tx}, nil
}

// SetConsistentPoint implements State.
func (e *serialEvents) SetConsistentPoint(ctx context.Context, cp stamp.Stamp) error {
	return e.loop.SetConsistentPoint(ctx, cp)
}

// Stopping implements State and delegates to the enclosing loop.
func (e *serialEvents) Stopping() <-chan struct{} {
	return e.loop.Stopping()
}

// A serialBatch corresponds exactly to a database transaction.
type serialBatch struct {
	parent *serialEvents

	tx types.TargetTx
}

var _ Batch = (*serialBatch)(nil)

// Flush returns nil, since OnData() writes values immediately.
func (e *serialBatch) Flush(context.Context) error {
	return nil
}

// OnCommit implements Events.
func (e *serialBatch) OnCommit(_ context.Context) <-chan error {
	if e.tx == nil {
		return singletonChannel(errors.New("OnCommit called without matching OnBegin"))
	}

	err := e.tx.Commit()
	e.tx = nil
	if err != nil {
		return singletonChannel(errors.WithStack(err))
	}

	return singletonChannel[error](nil)
}

// OnData implements Events.
func (e *serialBatch) OnData(
	ctx context.Context, _ ident.Ident, target ident.Table, muts []types.Mutation,
) error {
	app, err := e.parent.appliers.Get(ctx, target)
	if err != nil {
		return err
	}
	return app.Apply(ctx, e.tx, muts)
}

// OnRollback implements Events and delegates to drain.
func (e *serialBatch) OnRollback(_ context.Context) error {
	if e.tx != nil {
		_ = e.tx.Rollback()
		e.tx = nil
	}
	return nil
}
