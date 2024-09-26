#!/bin/bash

if ! [[ "$0" =~ scripts/genproto.sh ]]; then
  echo "must be run from repository root"
  exit 255
fi

# 列出protos目录下的所有子目录
PROTO_DIR="${PWD}/protos"

# 查找所有子目录并排除当前目录
find "$PROTO_DIR" -type d ! -path "$PROTO_DIR" | while read -r dir; do
  /opt/homebrew/bin/protoc --go_out=plugins=grpc:./ -I=$dir $dir/*.proto
done


/opt/homebrew/bin/protoc --go_out=plugins=grpc:./ -I=$dir $dir/*.proto