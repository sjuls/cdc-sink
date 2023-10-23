#!/usr/bin/env bash
#
# Copyright 2023 The Cockroach Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0
#

set -e

export SRC_DB=movr
export SRC_TABLES="movr.users,movr.vehicles,movr.rides,movr.vehicle_location_histories,movr.promo_codes,movr.user_promo_codes"
export SRC_HTTP=8090
export SRC_SQL_PORT=26267
export SRC_PORT=26268
export SRC_HOST=127.0.0.1

export SINK_HTTP=8080
export SINK_SQL_PORT=26257
export SINK_PORT=26258
export SINK_HOST=127.0.0.1
export CDC_PORT=26259

echo ">>> Build CDC Sink..."
go build ../

echo ">>> Start Source with Movr..."
cockroach start-single-node --advertise-addr=localhost --insecure --background --http-addr=":${SRC_HTTP}" --listen-addr=":${SRC_PORT}" --sql-addr=":${SRC_SQL_PORT}" --store=cockroach_source
cockroach sql --insecure --port=${SRC_SQL_PORT} -e="SET CLUSTER SETTING cluster.organization = 'Cloud Kitchens Test';"
cockroach sql --insecure --port=${SRC_SQL_PORT} -e="SET CLUSTER SETTING enterprise.license = \"${COCKROACH_DEV_LICENSE}\";"
cockroach sql --insecure --port=${SRC_SQL_PORT} -e="SET CLUSTER SETTING kv.rangefeed.enabled = true;"
cockroach workload init movr --num-histories=100000 --num-promo-codes=100000 --num-rides=50000 --num-users=5000 --num-vehicles=1500 "postgresql://root@${SRC_HOST}:${SRC_SQL_PORT}/movr?sslmode=disable"

echo ">>> Start Movr Workload..."
cockroach workload run movr --duration=5m --display-every=10s "postgresql://root@${SRC_HOST}:${SRC_SQL_PORT}/movr?sslmode=disable" > workload.log 2>&1 &

echo ">>> Start Sink..."
cockroach start-single-node --advertise-addr=localhost --insecure --background --http-addr=":${SINK_HTTP}" --listen-addr=":${SINK_PORT}" --sql-addr=":${SINK_SQL_PORT}" --store=cockroach_sink

echo ">>> Backup movr"
timestamp=$(cockroach sql --insecure --port=${SRC_SQL_PORT} -e="SELECT cluster_logical_timestamp();" | grep -E "\d+\.\d+")

cockroach sql --insecure --echo-sql --port=${SRC_SQL_PORT} -e="BACKUP DATABASE movr INTO 'nodelocal://self/backups/movr' AS OF SYSTEM TIME '${timestamp}';"
mkdir -p cockroach_sink/extern/backups
mv cockroach_source/extern/backups/movr cockroach_sink/extern/backups

sleep 60

echo ">>> Restore movr"

cockroach sql --insecure --echo-sql --port=${SINK_SQL_PORT} -e="RESTORE DATABASE movr FROM LATEST IN 'nodelocal://self/backups/movr';"
cockroach sql --insecure --echo-sql --port="$SINK_SQL_PORT" -e="CREATE USER cdcsink WITH LOGIN;"
cockroach sql --insecure --echo-sql --port="$SINK_SQL_PORT" -e="GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA movr.public TO cdcsink;"
cockroach sql --insecure --echo-sql --port="$SINK_SQL_PORT" -e="CREATE SCHEMA movr.cdcsink AUTHORIZATION cdcsink;"

sleep 60

echo ">>> Create Changefeeds on Source..."
cockroach sql --insecure --echo-sql --port=${SRC_SQL_PORT} -e="CREATE CHANGEFEED FOR TABLE ${SRC_TABLES} INTO \"webhook-https://${SINK_HOST}:${CDC_PORT}/movr/public?insecure_tls_skip_verify=true\" WITH cursor='${timestamp}',updated,resolved='1s',min_checkpoint_frequency='1s',webhook_sink_config='{\"Flush\":{\"Messages\":1000,\"Frequency\":\"1s\"}}';"

sleep 60

echo ">>> Start CDC Sink"
./cdc-sink start --targetConn="postgresql://cdcsink@${SINK_HOST}:${SINK_SQL_PORT}/${SRC_DB}?sslmode=disable" --stagingSchema=movr.cdcsink --disableAuthentication --foreignKeys --bindAddr=":${CDC_PORT}" --tlsSelfSigned > cdc-sink.log 2>&1 &

echo ">>> Waiting ..."
sleep 10

cockroach sql --insecure --echo-sql --port="$SINK_SQL_PORT" -e="REVOKE UPDATE ON TABLE movr.public.rides FROM cdcsink;"

echo ">>> Waiting ..."
sleep 10

cockroach sql --insecure --echo-sql --port="$SINK_SQL_PORT" -e="GRANT UPDATE ON TABLE movr.public.rides TO cdcsink;"

echo ">>> Waiting ..."
sleep 300

echo ">>> Starting Molt"
molt verify \
  --schema-filter "public" \
  --source "postgresql://root@$SRC_HOST:$SRC_SQL_PORT/movr?sslmode=disable" \
  --target "postgresql://root@$SINK_HOST:$SINK_SQL_PORT/movr?sslmode=disable" > molt.log 2>&1 &
