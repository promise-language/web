#!/usr/bin/env bash
go run -C "$(dirname "$0")/tools/build" ./cmd/make "$@"
