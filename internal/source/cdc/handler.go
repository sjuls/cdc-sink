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

// Package cdc contains a http.Handler which can receive webhook events
// from a CockroachDB CDC changefeed. Row updates and resolved
// timestamps are written to staging tables. The resolved timestamps are
// processed as a logical loop.
package cdc

import (
	"context"
	"net/http"
	"strings"

	"github.com/cockroachdb/cdc-sink/internal/types"
	"github.com/cockroachdb/cdc-sink/internal/util/httpauth"
	"github.com/cockroachdb/cdc-sink/internal/util/ident"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// sanitizer removes line breaks from the input to address log injection
// (CWE-117). This isn't strictly necessary with logrus, but:
// https://github.com/github/codeql/issues/11657
var sanitizer = strings.NewReplacer("\n", " ", "\r", " ")

// Handler is an http.Handler for processing webhook requests
// from a CockroachDB changefeed.
type Handler struct {
	Authenticator types.Authenticator // Access checks.
	Config        *Config             // Runtime options.
	Immediate     *Immediate          // Non-transactional mutations.
	Resolvers     *Resolvers          // Process resolved timestamps.
	StagingPool   *types.StagingPool  // Access to the staging cluster.
	Stores        types.Stagers       // Record incoming json blobs.
	TargetPool    *types.TargetPool   // Access to the target cluster.
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sendErr := func(err error) {
		if err == nil {
			http.Error(w, "OK", http.StatusOK)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.WithError(err).WithField("uri", r.RequestURI).Error()
	}

	defer func() {
		if thrown := recover(); thrown != nil {
			err, ok := thrown.(error)
			if !ok {
				err = errors.Errorf("unexpected error: %v", thrown)
			}
			sendErr(err)
		}
	}()

	log.WithField(
		"path", sanitizer.Replace(r.URL.Path),
	).Trace("request")

	req, err := h.newRequest(r)
	if err != nil {
		log.WithError(err).WithField(
			"path", sanitizer.Replace(r.URL.Path),
		).Trace("could not match URL")
		http.NotFound(w, r)
		return
	}

	allowed, err := h.checkAccess(ctx, r, req.target.Schema())
	switch {
	case err != nil:
		sendErr(err)
	case !allowed:
		http.Error(w, "missing or invalid access token", http.StatusUnauthorized)
	default:
		sendErr(req.leaf(ctx, req))
	}
}

func (h *Handler) checkAccess(
	ctx context.Context, r *http.Request, target ident.Schema,
) (bool, error) {
	token := httpauth.Token(r)
	// It's OK if token is empty here, we might be using a trivial
	// Authenticator.
	ok, err := h.Authenticator.Check(ctx, target, token)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	return false, nil
}
