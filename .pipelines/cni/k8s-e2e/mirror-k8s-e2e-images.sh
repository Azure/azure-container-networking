#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 3 ]]; then
	echo "usage: $0 <e2e.test path> <ACR name> <ACR subscription>" >&2
	exit 1
fi

e2e_test=$1
acr_name=$2
acr_subscription=$3

if [[ ! -x "$e2e_test" ]]; then
	echo "e2e.test is not executable: $e2e_test" >&2
	exit 1
fi

image_list=$(mktemp)
trap 'rm -f "$image_list"' EXIT
"$e2e_test" --list-images >"$image_list"

mapfile -t images < <(sort -u "$image_list")
if [[ ${#images[@]} -eq 0 ]]; then
	echo "e2e.test returned no images" >&2
	exit 1
fi

echo "Processing ${#images[@]} Kubernetes test images for $acr_name"
for source in "${images[@]}"; do
	case "$source" in
	registry.k8s.io/*)
		destination=${source#registry.k8s.io/}
		;;
	mcr.microsoft.com/*)
		destination=mcr/${source#mcr.microsoft.com/}
		;;
	docker.io/library/*)
		destination=library/${source#docker.io/library/}
		;;
	gcr.io/authenticated-image-pulling/* | gcr.io/k8s-authenticated-test/* | invalid.registry.k8s.io/*)
		echo "Skipping intentionally unavailable test image $source"
		continue
		;;
	*)
		echo "Skipping image without a Kubernetes registry override: $source"
		continue
		;;
	esac

	echo "Importing $source as $destination"
	az acr import \
		--subscription "$acr_subscription" \
		--name "$acr_name" \
		--source "$source" \
		--image "$destination" \
		--force \
		--output none
done
