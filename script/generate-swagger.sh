#!/usr/bin/env bash

set -euo pipefail

readonly SWAG_VERSION="v1.16.6"
readonly OUTPUT_ROOT="build/swagger"

generate() {
  local name="$1"
  shift

  local output_dir="${OUTPUT_ROOT}/${name}"

  go run "github.com/swaggo/swag/cmd/swag@${SWAG_VERSION}" init \
    --quiet \
    --generalInfo main.go \
    --parseInternal \
    --output "${output_dir}" \
    --outputTypes json \
    "$@"

  cp "${output_dir}/swagger.json" "${OUTPUT_ROOT}/${name}.json"
}

mkdir -p "${OUTPUT_ROOT}"

generate ppanel
generate admin --tags admin
generate user --tags user
generate common --tags common
generate node --tags node
generate edge --tags edge

cp "${OUTPUT_ROOT}/ppanel.json" ppanel.json
