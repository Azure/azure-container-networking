#!/usr/bin/env bash

# parse_override_image returns three values:
# <repoKey> <imageName> <versionOrDigest>
# Input -> output examples:
# 1) image="__use-default__" => "ACN <defaultName> <defaultVersion>"
# 2) image="acnpublic.azurecr.io/azure-cns:v1.2.3" => "ACN azure-cns v1.2.3"
# 3) image="mcr.microsoft.com/containernetworking/azure-cni:v1.2.3@sha256:abc" => "MCR azure-cni v1.2.3@sha256:abc"
parse_override_image() {
  image="$1"
  defaultName="$2"
  defaultVersion="$3"

  if [ -z "$image" ] || [ "$image" = "__use-default__" ]; then
    echo "ACN ${defaultName} ${defaultVersion}"
    return
  fi

  registry=""
  pathAndTag="$image"
  if [[ "$image" == */* ]]; then
    firstSegment="${image%%/*}"
    if [[ "$firstSegment" == *.* ]]; then
      registry="$firstSegment"
      pathAndTag="${image#*/}"
    fi
  fi

  repo="ACN"
  if [ "$registry" = "mcr.microsoft.com" ]; then
    repo="MCR"
    pathAndTag="${pathAndTag#containernetworking/}"
  fi
  if [ "$registry" = "acnpublic.azurecr.io" ]; then
    repo="ACN"
  fi

  name="$pathAndTag"
  version="$defaultVersion"
  if [[ "$pathAndTag" == *@* ]]; then
    beforeAt="${pathAndTag%@*}"
    digest="@${pathAndTag##*@}"
    if [[ "$beforeAt" == *:* ]]; then
      name="${beforeAt%:*}"
      version="${beforeAt##*:}${digest}"
    else
      name="$beforeAt"
      version="$digest"
    fi
  elif [[ "$pathAndTag" == *:* ]]; then
    name="${pathAndTag%:*}"
    version="${pathAndTag##*:}"
  fi

  echo "${repo} ${name} ${version}"
}

resolve_override_image() {
  local image="$1"

  if [ -z "$image" ] || [ "$image" = "__use-default__" ]; then
    format_image "acnpublic.azurecr.io" "$2" "$3"
    return
  fi

  echo "$image"
}

split_image_reference() {
  local image
  image="$(resolve_override_image "$1" "$2" "$3")"
  local expected_name="$2"
  local path_and_tag="$image"
  local digest=""
  local version=""

  if [[ "$path_and_tag" == *@* ]]; then
    digest="@${path_and_tag##*@}"
    path_and_tag="${path_and_tag%@*}"
  fi

  local image_path="$path_and_tag"
  local last_segment="${path_and_tag##*/}"
  if [[ "$last_segment" == *:* ]]; then
    version="${last_segment##*:}"
    image_path="${path_and_tag%:*}"
  elif [[ -n "$digest" ]]; then
    echo "Digest-only image references are not supported for ${expected_name}: ${image}" >&2
    return 1
  else
    version="$3"
  fi

  local image_name="${image_path##*/}"
  if [[ "$image_name" != "$expected_name" ]]; then
    echo "Expected image name '${expected_name}', got '${image_name}' from '${image}'" >&2
    return 1
  fi

  local registry_path="${image_path%/*}"
  if [[ "$registry_path" == "$image_path" || -z "$registry_path" ]]; then
    echo "Image reference must include a registry or repository prefix: ${image}" >&2
    return 1
  fi

  printf '%s %s%s\n' "$registry_path" "$version" "$digest"
}

format_image() {
  local registry="$1"
  local name="$2"
  local version="$3"

  if [[ "$version" == @* ]]; then
    echo "${registry}/${name}${version}"
    return
  fi

  echo "${registry}/${name}:${version}"
}
