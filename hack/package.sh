#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
source "${ROOT_DIR}/hack/lib/init.sh"

PACKAGE_DIR="${ROOT_DIR}/.dist/package"
mkdir -p "${PACKAGE_DIR}"
PACKAGE_TMP_DIR="${PACKAGE_DIR}/tmp"
mkdir -p "${PACKAGE_TMP_DIR}"

function setup_image_package() {
  if [[ "${PACKAGE_PUSH:-false}" == "true" ]]; then
    seal::image::login
  fi
}

function setup_image_package_context() {
  local target="$1"
  local task="$2"
  local path="$3"

  local context="${PACKAGE_DIR}/${target}/${task}"
  # create targeted dist
  rm -rf "${context}"
  mkdir -p "${context}"
  # copy targeted source to dist
  cp -rf "${path}/image" "${context}/image"
  # copy built result to dist
  cp -rf "${ROOT_DIR}/.dist/build/${target}" "${context}/build"

  echo -n "${context}"
}

function package() {
  local target="$1"
  local task="$2"
  local path="$3"

  # shellcheck disable=SC2155
  local tag="${REPO:-sealio}/${target}:$(seal::image::tag)"
  # shellcheck disable=SC2155
  local platform="$(seal::target::package_platform)"

  # shellcheck disable=SC2155
  local context="$(setup_image_package_context "${target}" "${task}" "${path}")"

  if [[ "${PACKAGE_BUILD:-true}" == "true" ]]; then
    # shellcheck disable=SC2086
    local no_cache_filter=""
    if [[ "${tag##*:}" != "dev" ]]; then
      task="${task}-release"
    else
      no_cache_filter="fetch"
    fi

    local cache="type=registry,ref=sealio/build-cache:${target}-${task}"
    local output="type=image,push=${PACKAGE_PUSH:-false}"

    seal::image::build \
      --tag="${tag}" \
      --platform="${platform}" \
      --cache-from="${cache}" \
      --output="${output}" \
      --progress="plain" \
      --no-cache-filter="${no_cache_filter}" \
      --file="${context}/image/Dockerfile" \
      "${context}"
  fi
}

function before() {
  setup_image_package
}

function dispatch() {
  local target="$1"
  local path="$2"

  shift 2
  local specified_targets="$*"
  if [[ -n ${specified_targets} ]] && [[ ! ${specified_targets} =~ ${target} ]]; then
    return
  fi

  local tasks=()
  # shellcheck disable=SC2086
  IFS=" " read -r -a tasks <<<"$(seal::util::find_subdirs ${path}/pack)"

  for task in "${tasks[@]}"; do
    seal::log::debug "packaging ${target} ${task}"
    if [[ "${PARALLELIZE:-true}" == "false" ]]; then
      package "${target}" "${task}" "${path}/pack/${task}"
    else
      package "${target}" "${task}" "${path}/pack/${task}" &
    fi
  done
}

#
# main
#

seal::log::info "+++ PACKAGE +++" "tag: $(seal::image::tag)"

before

dispatch "hermitcrab" "${ROOT_DIR}" "$@"

if [[ "${PARALLELIZE:-true}" == "true" ]]; then
  seal::util::wait_jobs || seal::log::fatal "--- PACKAGE ---"
fi
seal::log::info "--- PACKAGE ---"
