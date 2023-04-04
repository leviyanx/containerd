#!/usr/bin/env bash
# stop containerd
kill $(ps aux | grep '[s]udo ./bin/containerd' | awk '{print $2}')

sleep 2

sudo systemctl stop containerd
