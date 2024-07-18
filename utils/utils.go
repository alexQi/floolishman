package utils

import (
	"floolishman/utils/config"
	"floolishman/utils/log"
	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
	"time"
)

var Log *logrus.Logger
var Redis *redis.Client

func init() {
	config.LoadConf()
	Log = log.InitLogger()
	//Redis = redisClient.New()

	time.Local = time.FixedZone("CST", 8*3600) // 东八

	Log.Infof("------------------------------------")
	Log.Infof("----- Application Initializing -----")
	Log.Infof("------------------------------------")
}
