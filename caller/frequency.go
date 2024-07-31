package caller

import (
	"floolishman/model"
	"floolishman/utils"
	"time"
)

var (
	OpenPassCountLimit = 10
)

type PositionJudger struct {
	Pair          string           //交易对
	Matchers      []model.Strategy // 策略通过结果数组
	TendencyCount map[string]int   // 趋势得分Map
	Count         int              // 当前周期执行次数
	CreatedAt     time.Time        // 本次Counter创建时间
}

type CallerFrequency struct {
	CallerCommon
}

func (c *CallerFrequency) Start() {
	tickerCheck := time.NewTicker(CheckStrategyInterval * time.Second)
	tickerReset := time.NewTicker(ResetStrategyInterval * time.Second)
	for {
		select {
		case <-tickerCheck.C:
			for _, option := range c.pairOptions {
				go c.checkPosition(option.Pair)
			}
		case <-tickerReset.C:
			for _, option := range c.pairOptions {
				utils.Log.Infof("[JUDGE RESET] Pair: %s | TendencyCount: %v", option.Pair, c.positionJudgers[option.Pair].TendencyCount)
				c.ResetJudger(option.Pair)
			}
		}
	}
}

func (c *CallerFrequency) checkPosition(pair string) {
	// 执行策略
	finalTendency := c.Process(pair)
	// 获取多空比
	longShortRatio, matcherStrategy := c.getStrategyLongShortRatio(finalTendency, c.positionJudgers[pair].Matchers)
	if c.setting.Backtest == false && len(c.positionJudgers[pair].Matchers) > 0 {
		utils.Log.Infof(
			"[JUDGE] Pair: %s | LongShortRatio: %.2f | TendencyCount: %v | MatcherStrategy:【%v】",
			pair,
			longShortRatio,
			c.positionJudgers[pair].TendencyCount,
			matcherStrategy,
		)
	}
	// 加权因子计算复合策略的趋势判断待调研是否有用 todo
	// 多空比不满足开仓条件
	if longShortRatio < 0 {
		return
	}
	if len(matcherStrategy) < 2 {
		return
	}
	// 计算当前方向通过总数
	passCount := 0
	for _, i := range matcherStrategy {
		passCount += i
	}
	// 当前方向通过次数少于阈值 不开仓
	if passCount < OpenPassCountLimit {
		return
	}
	// 执行开仓检查
	assetPosition, quotePosition, err := c.broker.PairAsset(pair)
	if err != nil {
		utils.Log.Error(err)
	}
	c.openPosition(
		c.pairOptions[pair],
		assetPosition,
		quotePosition,
		longShortRatio,
		matcherStrategy,
		c.positionJudgers[pair].Matchers,
	)
}

func (c *CallerFrequency) Process(pair string) string {
	c.mu.Lock()         // 加锁
	defer c.mu.Unlock() // 解锁
	// 如果 pair 在 positionJudgers 中不存在，则初始化
	if _, ok := c.positionJudgers[pair]; !ok {
		c.ResetJudger(pair)
	}
	// 执行计数器+1
	c.positionJudgers[pair].Count++
	// 执行策略检查
	matchers := c.strategy.CallMatchers(c.samples[pair])
	// 清洗策略结果
	finalTendency, currentMatchers := c.Sanitizer(matchers)
	// 重组匹配策略数据
	c.positionJudgers[pair].Matchers = append(c.positionJudgers[pair].Matchers, currentMatchers...)
	// 更新趋势计数
	c.positionJudgers[pair].TendencyCount[finalTendency]++
	// 返回当前趋势
	return finalTendency
}
