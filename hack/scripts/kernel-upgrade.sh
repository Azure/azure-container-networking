#!/bin/bash
# This script will upgrade a ubuntu kernel to the proposed kernel
# Requires a restart to complete the upgrade

apt update && apt-get install software-properties-common -y

echo "-- Add proposed repository --"
add-apt-repository ppa:canonical-kernel-team/proposed -y
add-apt-repository ppa:canonical-kernel-team/proposed2 -y

echo "-- Check apt-cache --"
apt-cache madison linux-azure-edge

echo "-- Install proposed kernel --"
apt install -y linux-azure-edge

echo "-- Check current Ubuntu kernel --"
uname -r
