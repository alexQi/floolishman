#!/bin/bash

# 开始编译
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o run ${PWD}/cmd/futures/main.go

# 交叉编译windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o robot.exe ${PWD}/cmd/futures/main.go