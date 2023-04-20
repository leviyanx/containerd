#!/usr/bin/env bash
# stop containerd
kill -9 $(ps aux | grep './bin/containerd' | grep -v grep | awk '{print $2}')

sleep 2

systemctl stop containerd
