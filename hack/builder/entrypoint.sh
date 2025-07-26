#!/usr/bin/env bash
set -e
set -o pipefail

if [[ "$KUBEVIRT_CREATE_BAZELRCS" == "true" ]]; then
    /create_bazel_cache_rcs.sh
fi
export PULLER_TIMEOUT=120000

echo "-------- docker cache $DOCKER_REPO_CACHE"

source /etc/profile.d/gimme.sh
export GOPATH="/root/go"
eval "$@"
