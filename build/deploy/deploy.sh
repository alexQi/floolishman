#!/usr/bin/env bash

serviceName="wechat-bot"
registry="registry.cn-shenzhen.aliyuncs.com/qiubo/tkrobot"
initVersion="latest"
``
# 判断用户是否输入版本
echo -en "\033[32m entry $serviceName version \033[0m (default\033[37m $initVersion \033[0m):"
read version
if [ ! -n "$version" ]; then
  version=$initVersion
fi

# 拉取最新镜像
echo -e "\033[32m [$serviceName:$version] \033[0m get image: $version"
docker pull $registry:$version

# 停止服务删除容器
curRunId=$(docker ps -qlf name=$serviceName)
if [ -n "$curRunId" ]; then
  echo -e "\033[32m [$serviceName:$version] \033[0m stoping && remove service:$curRunId"
  docker stop $curRunId >/dev/null 2>&1 && docker rm $curRunId >/dev/null 2>&1
fi

# 重新运行容器
echo -e "\033[32m [$serviceName:$version] \033[0m run $serviceName"

runId=$(docker run -d --name $serviceName \
  --env-file ${PWD}/.env \
  --log-opt max-size=1000m \
  --log-opt max-file=3 \
  -p 18400:18400 \
  -p 18401:18401 \
  --restart=always \
  $registry:$version)

echo -e "\033[32m [$serviceName:$version] \033[0m run success,Container Id:$runId"

# 删除历史镜像
current=$(docker images -q $registry:$version)
imageIds=$(docker images -q $registry)

for imageId in $imageIds; do
  if [ ! $current = $imageId ]; then
    echo -e "\033[32m [$serviceName] \033[0m remove image $imageId"
    docker rmi $imageId >/dev/null 2>&1
  fi
done
