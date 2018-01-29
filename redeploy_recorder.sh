#!/bin/bash
git pull
./build_recorder.sh && \
  sudo systemctl restart tesler-recorder

