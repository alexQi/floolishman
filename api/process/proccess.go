package process

import (
	"time"
)

var cacheSettingTime = 0 * time.Second

func CacheSetting() {
	for {
		select {
		case <-time.After(cacheSettingTime):
			// 设置延迟时间
			cacheSettingTime = 5 * time.Second
		}
	}
}
