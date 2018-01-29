#!/bin/bash
go build -o ./state_tracker tools/state_tracker/main.go && \
	docker build -t tesler/state_tracker -f tools/state_tracker/Dockerfile .
