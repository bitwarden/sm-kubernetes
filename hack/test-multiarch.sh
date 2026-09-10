#!/usr/bin/env bash
#
# Integration test that verifies the manager image can actually be executed on
# every platform it is published for (e.g. linux/amd64 and linux/arm64).
#
# This is a regression test for https://github.com/bitwarden/sm-kubernetes/issues/131,
# where the published image for ARM platforms failed to start with:
#   exec /manager: exec format error
#
# The test builds the manager image for each requested platform using
# `docker buildx` (loading a single-platform image at a time, since
# multi-platform builds cannot be loaded into the local image store), then runs
# the resulting image under that platform (using QEMU emulation when the
# platform doesn't match the host) to confirm the binary starts and responds
# normally instead of failing with an architecture/format error.
#
# Usage:
#   ./hack/test-multiarch.sh
#   PLATFORMS=linux/arm64 ./hack/test-multiarch.sh
#
# Can be run locally (requires Docker with buildx and, for non-native
# platforms, QEMU user-mode emulation registered, e.g. via
# `docker run --privileged --rm tonistiigi/binfmt --install all`) and is also
# wired into the GitHub Actions build workflow.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

CONTAINER_TOOL=${CONTAINER_TOOL:-docker}
PLATFORMS=${PLATFORMS:-linux/amd64,linux/arm64}
IMAGE_TAG_BASE=${IMAGE_TAG_BASE:-localhost/sm-operator-multiarch-test}
DOCKERFILE=${DOCKERFILE:-Dockerfile}

# GitHub Actions log grouping/annotations are only useful (and only rendered)
# when running inside a GitHub Actions job. When run locally, plain output is
# less noisy.
in_github_actions() {
  [[ "${GITHUB_ACTIONS:-}" == "true" ]]
}

group_start() {
  if in_github_actions; then
    echo "::group::$1"
  else
    echo "=== $1 ==="
  fi
}

group_end() {
  if in_github_actions; then
    echo "::endgroup::"
  fi
}

log_error() {
  if in_github_actions; then
    echo "::error::$1"
  else
    echo "ERROR: $1" >&2
  fi
}

IFS=',' read -r -a platform_array <<< "$PLATFORMS"

failures=()

for platform in "${platform_array[@]}"; do
  safe_platform="${platform//\//-}"
  tag="${IMAGE_TAG_BASE}:${safe_platform}"

  group_start "Building image for platform ${platform}"
  if ! "$CONTAINER_TOOL" buildx build \
    --platform "$platform" \
    --file "$DOCKERFILE" \
    --tag "$tag" \
    --load \
    "$REPO_ROOT"; then
    log_error "Failed to build image for platform ${platform}"
    failures+=("$platform (build failed)")
    group_end
    continue
  fi
  group_end

  group_start "Running image for platform ${platform}"
  # `--help` is used as a lightweight smoke invocation: the manager binary
  # parses flags and prints usage before starting the controller, so this
  # exercises the entrypoint without requiring a Kubernetes API server.
  output=""
  status=0
  output=$("$CONTAINER_TOOL" run --rm --platform "$platform" "$tag" --help 2>&1) || status=$?

  echo "$output"

  if [[ "$output" == *"exec format error"* ]]; then
    log_error "Image for platform ${platform} failed with exec format error"
    failures+=("$platform (exec format error)")
  elif [[ $status -ne 0 ]]; then
    log_error "Image for platform ${platform} exited with status ${status}"
    failures+=("$platform (exit status ${status})")
  elif [[ "$output" != *"Usage of /manager"* ]]; then
    log_error "Image for platform ${platform} did not produce the expected usage output"
    failures+=("$platform (unexpected output)")
  else
    echo "Platform ${platform}: OK"
  fi
  group_end

  "$CONTAINER_TOOL" image rm "$tag" >/dev/null 2>&1 || true
done

if [[ ${#failures[@]} -gt 0 ]]; then
  log_error "Multi-arch integration test failed for: ${failures[*]}"
  exit 1
fi

echo "Multi-arch integration test passed for platforms: ${PLATFORMS}"
