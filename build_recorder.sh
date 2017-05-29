#!/bin/bash
go build -o ./recorder_main recorder/server/recorder_main.go && \
	docker build -t tesler/recorder -f recorder/Dockerfile .
