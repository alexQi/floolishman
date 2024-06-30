#!/usr/bin/env zsh

serviceName="wechat-bot"
registry="registry.cn-shenzhen.aliyuncs.com/qiubo/tkrobot"
initVersion="latest"

# 判断用户是否输入版本
echo -en "\033[32m entry $serviceName version \033[0m (default\033[37m $initVersion \033[0m):"
read version
if [ ! -n "$version" ]; then
  version=$initVersion
fi

echo -e "\033[32m [$serviceName:$version] \033[0m compiling application"
${PWD}/build/builder/builder.sh

echo -e "\033[32m [$serviceName:$version] \033[0m build image"
docker build -t $serviceName:$version .

echo -e "\033[32m [$serviceName:$version] \033[0m clean application pkg..."
rm -rf ${PWD}/run

currentImageId=$(docker images -q $serviceName | awk 'NR==1{print}')

if [ ! -n "$currentImageId" ]; then
  echo -e "\033[32m [$serviceName:$version] \033[0m image id not found"
  exit
else
  echo -e "\033[32m [$serviceName:$version] \033[0m tag this image : $currentImageId"
  docker tag $currentImageId $registry:$version

  echo -e "\033[32m [$serviceName:$version] \033[0m push this image : $currentImageId"
  docker push $registry:$version
fi

current=$(docker images -q $registry:$version)
imageIds=$(docker images -q $registry)

for imageId in $imageIds; do
  if [ ! $current = $imageId ]; then
    runId=$(docker ps -qlf name=$serviceName)
    if [ -n "$runId" ]; then
      docker stop $runId >/dev/null 2>&1 && docker rm $runId >/dev/null 2>&1
    fi
    echo "[$serviceName] remove image $imageId"
    docker rmi $imageId >/dev/null 2>&1
  fi
done
