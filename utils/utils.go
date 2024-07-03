package utils

import (
	"floolishman/internal/redisClient"
	"floolishman/utils/config"
	"floolishman/utils/log"
	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger
var Redis *redis.Client

func init() {
	config.LoadConf()
	Log = log.InitLogger()
	Redis = redisClient.New()

	Log.Infof("------------------------------------")
	Log.Infof("----- Application Initializing -----")
	Log.Infof("------------------------------------")
}
