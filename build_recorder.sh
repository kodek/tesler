#!/bin/bash
go build -o ./recorder_main recorder/server/* && \
	docker build -t tesler/recorder -f recorder/Dockerfile .
