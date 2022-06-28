#!/bin/bash

export MAJOR=0
export MINOR=0
export PATCH=0
export FULL_TAG="v0.0.0"

function create_release () {
    parse_git_tag

    if increment_git_tag
    then
        FULL_TAG="v${MAJOR}.${MINOR}.${PATCH}"
        set_git_tag
    else
        echo "local git tag unchanged"
    fi  
}

function get_git_tag () {
    
    return echo "v${MAJOR}${MINOR}${PATCH}"
}

function set_git_tag () {
    read -p "Set git tag to $FULL_TAG? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$  ]]
    then
        git tag $FULL_TAG
        echo "git tag set to $FULL_TAG"

        push_git_tag
    fi
    
    return
}

function push_git_tag {
    remote=$(git rev-parse --abbrev-ref --symbolic-full-name @{u} | cut -f1 -d"/")

    read -p "Push git tag to remote $remote? [y/N] " -n 1 
    echo
    if [[ $REPLY =~ ^[Yy]$  ]]
    then
        git push $remote $FULL_TAG 
    fi
}

function increment_git_tag () {
    read -p "Does this release contain incompatible API changes? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$  ]]
    then
        MAJOR=$((MAJOR+1))
        MINOR=0
        PATCH=0
        return 
    fi

    read -p "Does this release contain added functionality that's backwards compatible? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$  ]]
    then
        MINOR=$((MINOR+1))
        PATCH=0
        return 
    fi

    read -p "Does this release only contain backwards compatible bug fixes [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$  ]]
    then
        PATCH=$((PATCH+1))
        return 
    fi

    false
}

function parse_git_tag () {
    FULL_TAG=$(git describe --tags --abbrev=0)
    echo Current git tag: $FULL_TAG
    echo "--------"
    tagArr=(${FULL_TAG//./ })
    MAJOR=${tagArr[0]:1}
    MINOR=${tagArr[1]}
    PATCH=${tagArr[2]}
}

create_release