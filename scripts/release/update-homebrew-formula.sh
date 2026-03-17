#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${TAG_NAME:-}" ]]; then
  echo "TAG_NAME is required (example: v0.1.0)"
  exit 1
fi

if [[ -z "${SOURCE_REPOSITORY:-}" ]]; then
  echo "SOURCE_REPOSITORY is required (example: mhrabovcin/troubleshoot-live)"
  exit 1
fi

if [[ -z "${TAP_DIR:-}" ]]; then
  echo "TAP_DIR is required"
  exit 1
fi

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  echo "GITHUB_TOKEN is required"
  exit 1
fi

api_url="https://api.github.com/repos/${SOURCE_REPOSITORY}/releases/tags/${TAG_NAME}"
release_json="$(mktemp)"
checksums_txt="$(mktemp)"

cleanup() {
  rm -f "${release_json}" "${checksums_txt}"
}
trap cleanup EXIT

curl -fsSL \
  -H "Authorization: Bearer ${GITHUB_TOKEN}" \
  -H "Accept: application/vnd.github+json" \
  "${api_url}" > "${release_json}"

checksums_url="$(jq -r '.assets[] | select(.name == "checksums.txt") | .browser_download_url' "${release_json}")"
if [[ -z "${checksums_url}" || "${checksums_url}" == "null" ]]; then
  echo "Unable to find checksums.txt in release ${TAG_NAME}"
  exit 1
fi

curl -fsSL \
  -H "Authorization: Bearer ${GITHUB_TOKEN}" \
  -H "Accept: application/octet-stream" \
  -o "${checksums_txt}" \
  "${checksums_url}"

version="${TAG_NAME#v}"
asset_base="https://github.com/${SOURCE_REPOSITORY}/releases/download/${TAG_NAME}"

sha_for() {
  local filename="$1"
  local value
  value="$(awk -v f="${filename}" '$2 == f { print $1 }' "${checksums_txt}")"

  if [[ -z "${value}" ]]; then
    echo "Missing checksum for ${filename}"
    exit 1
  fi

  printf '%s' "${value}"
}

darwin_amd64="troubleshoot-live_v${version}_darwin_amd64.tar.gz"
darwin_arm64="troubleshoot-live_v${version}_darwin_arm64.tar.gz"
linux_amd64="troubleshoot-live_v${version}_linux_amd64.tar.gz"
linux_arm64="troubleshoot-live_v${version}_linux_arm64.tar.gz"

formula_path="${TAP_DIR}/Formula/troubleshoot-live.rb"
mkdir -p "$(dirname "${formula_path}")"

cat > "${formula_path}" <<FORMULA
class TroubleshootLive < Formula
  desc "Expose support bundle resources via a local Kubernetes API server"
  homepage "https://github.com/${SOURCE_REPOSITORY}"
  version "${version}"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "${asset_base}/${darwin_arm64}"
      sha256 "$(sha_for "${darwin_arm64}")"
    else
      url "${asset_base}/${darwin_amd64}"
      sha256 "$(sha_for "${darwin_amd64}")"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "${asset_base}/${linux_arm64}"
      sha256 "$(sha_for "${linux_arm64}")"
    else
      url "${asset_base}/${linux_amd64}"
      sha256 "$(sha_for "${linux_amd64}")"
    end
  end

  def install
    bin.install "troubleshoot-live"
  end

  test do
    assert_match "troubleshoot-live", shell_output("#{bin}/troubleshoot-live --help")
  end
end
FORMULA

echo "Updated ${formula_path} for ${TAG_NAME}"
