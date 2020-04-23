#!/bin/bash

echo "check-version.sh test"

function normalize_version() {
    local major=0
    local minor=0
    local patch=0

    # Only parses purely numeric version numbers, 1.2.3
    # Everything after the first three values are ignored
    if [[ $1 =~ ^([0-9]+)\.([0-9]+)\.?([0-9]*)([^ ])* ]]; then
        major=${BASH_REMATCH[1]}
        minor=${BASH_REMATCH[2]}
        patch=${BASH_REMATCH[3]}
    fi

    printf %02d%02d%02d $major $minor $patch
}

function compare_versions() {
    local version_1="$1"
    local op=$2
    local version_2="$3"

    if [[ -z $version_1 || -z $version_2 ]]; then
        return 1
    fi

    version_1="$( normalize_version "$version_1" )"
    version_2="$( normalize_version "$version_2" )"

    if [[ ! " = == > >= < <= != " =~ " $op " ]]; then
        return 1
    fi

    [[ $op == "<"  &&   $version_1 <  $version_2 ]] && return 0
    [[ $op == "<=" && ! $version_1 >  $version_2 ]] && return 0
    [[ $op == "="  &&   $version_1 == $version_2 ]] && return 0
    [[ $op == "==" &&   $version_1 == $version_2 ]] && return 0
    [[ $op == ">"  &&   $version_1 >  $version_2 ]] && return 0
    [[ $op == ">=" && ! $version_1 <  $version_2 ]] && return 0
    [[ $op == "!=" &&   $version_1 != $version_2 ]] && return 0

    return 1
}
