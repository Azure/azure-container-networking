#!/bin/bash

# ./check-cni-log-contents.sh <directory> <file_pattern> <regex to search for>

if [ $# -ne 3 ]; then
    echo "Usage: $0 <directory> <file_pattern> <regex to search for>"
    exit 1
fi

directory="$1"
file_pattern="$2"
content_pattern="$3"

# Find files matching pattern and grep for content-- use -aP so we can detect null (\x00) characters
matches=$(find "$directory" -name "$file_pattern" -type f -exec grep -aP -l "$content_pattern" {} +)

if [ -n "$matches" ]; then
    echo "$matches"
    echo ""
    echo "Matching lines:"
    find "$directory" -name "$file_pattern" -type f -exec grep -aP -n "$content_pattern" {} +
    exit 0
else
    exit 1
fi
