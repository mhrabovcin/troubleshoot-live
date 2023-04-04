#!/bin/bash
set -euo pipefail

ROOT_DIR=${ROOT_DIR:-"$(git rev-parse --show-toplevel)"}

HELM_REPO_DIR=${HELM_REPO_DIR:-"${ROOT_DIR}/third-party/troubleshoot-live"}
HELM_REPO_BRANCH=${HELM_REPO_BRANCH:-"pages"}
GIT_ORIGIN=${GIT_ORIGIN:-"$(git config --get remote.origin.url)"}

# Clone or update repository.

if [[ ! -d ${HELM_REPO_DIR} ]]; then
  git clone --recursive --branch ${HELM_REPO_BRANCH} ${GIT_ORIGIN} ${HELM_REPO_DIR}
else
  pushd "${HELM_REPO_DIR}"
  git pull origin
  popd
fi

# Push new chart.

# We want to push the new chart and update the index file, this would normally be straightforward except both helm and chartmuseum
# clobbers the `created` date for old releases with the current timestamp when merging the index.yaml file.
# You need to use the following workaround via temporary folders to prevent that.
# Source: https://github.com/helm/helm/issues/4482#issuecomment-452013778

# Create temporary directories
TEMP_DIR=`mktemp -d`
mkdir -p ${TEMP_DIR}/charts

# Build new helm chart package
helm package -d ${TEMP_DIR}/charts ${ROOT_DIR}/charts/troubleshoot-live

# Create merged index.yaml file
helm repo index --merge ${HELM_REPO_DIR}/index.yaml ${TEMP_DIR}

mv ${TEMP_DIR}/charts/*   ${HELM_REPO_DIR}/charts
cp ${TEMP_DIR}/index.yaml ${HELM_REPO_DIR}/index.yaml

echo ""
echo "Charts published to ${HELM_REPO_DIR}."
echo "To verify, issue: cd ${HELM_REPO_DIR} && git diff"
echo "To release charts, commit new files and index.yaml"

# Cleanup temporary directories.
rm -rf ${TEMP_MANAGEMENT_DIR}