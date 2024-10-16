#!/bin/bash

export_envVars() {
    az storage file download --account-name acnshared -s aksdev -p ./swiftv2runnerconfigvars-$1.env --auth-mode login --enable-file-backup-request-intent
    export $(xargs < ./swiftv2runnerconfigvars-$1.env)
}

utils::gen_pass() {
    local pass_len
    pass_len="${1:-48}"
  
    if [ -o xtrace ]; then
        set +x
        trap 'set -x' RETURN ERR
    fi
    base64_pass=$(openssl rand -base64 "${pass_len}")
    return 0
}

utils::setvar() {
    local var_name
    var_name="${1}"
    local value
    value="${@:2}"

    local hide="#"
    local taskns="vso"
    echo >&2 "${hide}${hide}${taskns}[task.setvariable name=${var_name};isoutput=true;]$value"
    eval "export "$var_name"="$value""
}

utils::setsecret() {
    local var_name
    var_name="${1}"
    local value
    value="${@:2}"

    local hide="#"
    local taskns="vso"
    echo >&2 "${hide}${hide}${taskns}[task.setvariable name=${var_name};isoutput=true;issecret=true;]$value"
    eval "export "$var_name"="$value""
}

utils::log() {
    local cmd
    cmd=("${@}")
    echo "${@}"
    local outreader
    outreader=$(touch out.log && echo "out.log")
    local errreader
    errreader=$(touch err.log && echo "err.log")

    "${cmd[@]}"  > >(tee ${outreader}) 2> >(tee ${errreader} >&2)
    cmd_code="${PIPESTATUS[0]}"
    cmd_out=$(cat $outreader)
    cmd_err=$(cat $errreader)
    return $cmd_code
}
