#!/usr/bin/env bash

# parse_override_image returns four values:
# <useProvidedImage> <repoKey> <imageName> <versionOrDigest>
# Input -> output examples:
# 1) image="__use-default__" => "false ACN <defaultName> <defaultVersion>"
# 2) image="acnpublic.azurecr.io/azure-cns:v1.2.3" => "true ACN azure-cns v1.2.3"
# 3) image="mcr.microsoft.com/containernetworking/azure-cni@sha256:abc" => "true MCR azure-cni @sha256:abc"
parse_override_image() {
  image="$1"
  defaultName="$2"
  defaultVersion="$3"

  if [ -z "$image" ] || [ "$image" = "__use-default__" ]; then
    echo "false ACN ${defaultName} ${defaultVersion}"
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
    name="${pathAndTag%@*}"
    version="@${pathAndTag##*@}"
  elif [[ "$pathAndTag" == *:* ]]; then
    name="${pathAndTag%:*}"
    version="${pathAndTag##*:}"
  fi

  echo "true ${repo} ${name} ${version}"
}
