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

format_override_image() {
  repo="$1"
  name="$2"
  version="$3"

  case "$repo" in
    MCR) registry="mcr.microsoft.com/containernetworking" ;;
    *) registry="acnpublic.azurecr.io" ;;
  esac

  if [[ "$version" == @* ]]; then
    echo "${registry}/${name}${version}"
    return
  fi

  echo "${registry}/${name}:${version}"
}
