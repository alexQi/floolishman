package logic

import (
	"floolishman/utils"
)

const TradingPairs = "trading:futures:paris"

// 结构 hashmap field 为pair ,值为json {杠杆，保证金模式，pair}
func SetTradingPair(pair string, option string) error {
	_, err := utils.Redis.HSet(TradingPairs, pair, option).Result()
	if err != nil {
		return err
	}
	return nil
}

func GetTradingPairs() ([]interface{}, error) {
	option, err := utils.Redis.HMGet(TradingPairs).Result()
	if err != nil {
		return option, err
	}
	return option, nil
}
