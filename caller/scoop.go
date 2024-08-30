package caller

import (
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"time"
)

var ExecInterval time.Duration = 5

type Scoop struct {
	Common
}

func (c *Scoop) Start() {
	now := time.Now()
	next := now.Truncate(time.Minute * ExecInterval).Add(time.Minute * ExecInterval)
	duration := time.Until(next)
	// 在下一个5分钟倍数时执行任务
	time.AfterFunc(duration, func() {
		c.tickCheckForOpen()
	})
	go c.Listen()
}

func (c *Scoop) Listen() {
	// 监听仓位关闭信号重置judger
	go c.RegisterOrderSignal()
	// 非回溯测试时，执行检查仓位关闭
	if c.setting.Backtest == false {
		// 执行超时检查
		go c.tickCheckOrderTimeout()
		// 非回溯测试模式且不是看门狗方式下监听平仓
		if c.setting.FollowSymbol == false {
			go c.tickerCheckForClose()
		}
	}
}

func (c *Scoop) tickCheckForOpen() {
	ticker := time.NewTicker(ExecInterval * time.Minute)
	for {
		select {
		case <-ticker.C:
			utils.Log.Infof("[CALLER] tick check for all pair to open position ...")
			for _, option := range c.pairOptions {
				if option.Status == false {
					continue
				}
				assetPosition, quotePosition, longShortRatio, currentMatchers, matcherStrategy := c.checkPosition(option)
				if longShortRatio >= 0 {
					go func() {
						c.openPosition(option, assetPosition, quotePosition, longShortRatio, matcherStrategy, currentMatchers)
					}()
				}
			}
		}
	}
}

func (c *Scoop) tickerCheckForClose() {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckCloseInterval * time.Millisecond):
			for _, option := range c.pairOptions {
				go c.EventCallClose(option.Pair)
			}
		}
	}
}

func (c *Scoop) EventCallClose(pair string) {
	c.closePosition(c.pairOptions[pair])
}

func (c *Scoop) closePosition(option *model.PairOption) {
	c.mu[option.Pair].Lock()         // 加锁
	defer c.mu[option.Pair].Unlock() // 解锁
	// 获取当前已存在的仓位
	openedPositions, err := c.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if len(openedPositions) == 0 {
		return
	}

	currentPrice, _ := c.pairPrices.Get(option.Pair)
	// 与当前方向相反有仓位,计算相对分界线距离，多空比达到反手标准平仓
	// ***********************
	for _, openedPosition := range openedPositions {
		// 监控已成交仓位，记录订单成交时间+指定时间作为时间止损
		_, ok := c.lossLimitTimes.Get(openedPosition.OrderFlag)
		if !ok {
			c.lossLimitTimes.Set(openedPosition.OrderFlag, openedPosition.UpdatedAt.Add(time.Duration(c.setting.LossTimeDuration)*time.Minute))
		}
		// 记录利润比
		profitRatio := calc.ProfitRatio(
			model.SideType(openedPosition.Side),
			openedPosition.AvgPrice,
			currentPrice,
			float64(option.Leverage),
			openedPosition.Quantity,
		)
		lossLimitTime, _ := c.lossLimitTimes.Get(openedPosition.OrderFlag)
		if c.setting.Backtest == false {
			utils.Log.Infof(
				"[POSITION - WATCH] OrderFlag: %s | Pair: %s | P.Side: %s | Quantity: %v | Price: %v, Current: %v | PR.%%: %s | Create: %s | Stop Cut-off: %s",
				openedPosition.OrderFlag,
				openedPosition.Pair,
				openedPosition.PositionSide,
				openedPosition.Quantity,
				openedPosition.AvgPrice,
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				lossLimitTime.In(Loc).Format("2006-01-02 15:04:05"),
			)
		}
		currentTime := time.Now()
		if c.setting.Backtest {
			currentTime, _ = c.lastUpdate.Get(option.Pair)
		}
		// 时间未达到新的止损限制时间
		if currentTime.After(lossLimitTime) {
			utils.Log.Infof(
				"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s (time out)",
				openedPosition.Pair,
				openedPosition.OrderFlag,
				openedPosition.Quantity,
				openedPosition.AvgPrice,
				openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
			)
			c.resetPairProfit(option.Pair)
			c.finishPosition(SeasonTypeTimeout, openedPosition)
			continue
		}
		// ****************
		if profitRatio > 0 {
			pairCurrentProfit, _ := c.pairCurrentProfit.Get(option.Pair)
			// ---------------------
			// 判断利润比小于等于上次设置的利润比，则平仓 初始时为0
			if profitRatio <= pairCurrentProfit.Close && pairCurrentProfit.Close > 0 {
				utils.Log.Infof(
					"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < ProfitClose: %s",
					openedPosition.Pair,
					openedPosition.OrderFlag,
					openedPosition.Quantity,
					openedPosition.AvgPrice,
					openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", pairCurrentProfit.Close*100),
				)
				// 重置交易对盈利
				c.resetPairProfit(option.Pair)
				//c.PausePairCall(option.Pair)
				c.finishPosition(SeasonTypeProfitBack, openedPosition)
				return
			}
			profitTriggerRatio := pairCurrentProfit.Floor
			// 判断是否已锁定利润比
			if pairCurrentProfit.Close == 0 {
				// 小于触发值时，记录当前利润比
				if profitRatio < profitTriggerRatio {
					utils.Log.Infof(
						"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < ProfitTriggerRatio: %s",
						openedPosition.Pair,
						openedPosition.OrderFlag,
						openedPosition.Quantity,
						openedPosition.AvgPrice,
						openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
						currentPrice,
						fmt.Sprintf("%.2f%%", profitRatio*100),
						fmt.Sprintf("%.2f%%", profitTriggerRatio*100),
					)
					return
				}
			} else {
				if profitRatio < profitTriggerRatio {
					// 当前利润比触发值，之前已经有Close时，判断当前利润比是否比上次设置的利润比大
					if profitRatio <= pairCurrentProfit.Close+pairCurrentProfit.Decrease {
						utils.Log.Infof(
							"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < ProfitCloseRatio: %s",
							openedPosition.Pair,
							openedPosition.OrderFlag,
							openedPosition.Quantity,
							openedPosition.AvgPrice,
							openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
							currentPrice,
							fmt.Sprintf("%.2f%%", profitRatio*100),
							fmt.Sprintf("%.2f%%", pairCurrentProfit.Close*100),
						)
						return
					}
				}
			}
			// 当前利润比大于等于触发利润比，
			profitLevel := c.findProfitLevel(option.Pair, profitRatio)
			if profitLevel != nil {
				pairCurrentProfit.Floor = profitLevel.NextTriggerRatio
				pairCurrentProfit.Decrease = profitLevel.DrawdownRatio
			}

			pairCurrentProfit.Close = profitRatio - pairCurrentProfit.Decrease
			c.pairCurrentProfit.Set(option.Pair, pairCurrentProfit)
			utils.Log.Infof(
				"[POSITION - PROFIT] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > NewProfitRatio: %s",
				openedPosition.Pair,
				openedPosition.OrderFlag,
				openedPosition.Quantity,
				openedPosition.AvgPrice,
				openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				fmt.Sprintf("%.2f%%", pairCurrentProfit.Close*100),
			)
		} else {
			if option.MaxMarginLossRatio > 0 {
				// 亏损盈利比已大于最大
				if calc.Abs(profitRatio) > option.MaxMarginLossRatio {
					utils.Log.Infof(
						"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > MaxLoseRatio %s",
						openedPosition.Pair,
						openedPosition.OrderFlag,
						openedPosition.Quantity,
						openedPosition.AvgPrice,
						openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
						currentPrice,
						fmt.Sprintf("%.2f%%", profitRatio*100),
						fmt.Sprintf("%.2f%%", option.MaxMarginLossRatio*100),
					)
					c.resetPairProfit(option.Pair)
					c.finishPosition(SeasonTypeLossMax, openedPosition)
					return
				}
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < MaxLoseRatio: %s",
					openedPosition.Pair,
					openedPosition.OrderFlag,
					openedPosition.Quantity,
					openedPosition.AvgPrice,
					openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", option.MaxMarginLossRatio*100),
				)
			} else {
				if (openedPosition.PositionSide == string(model.PositionSideTypeLong) && currentPrice <= openedPosition.StopLossPrice) ||
					(openedPosition.PositionSide == string(model.PositionSideTypeShort) && currentPrice >= openedPosition.StopLossPrice) {
					utils.Log.Infof(
						"[POSITION - CLOSE] Pair: %s | PositionSide: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v, StopLoss:%v | PR.%%: %s",
						openedPosition.Pair,
						openedPosition.PositionSide,
						openedPosition.OrderFlag,
						openedPosition.Quantity,
						openedPosition.AvgPrice,
						openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
						currentPrice,
						openedPosition.StopLossPrice,
						fmt.Sprintf("%.2f%%", profitRatio*100),
					)
					c.resetPairProfit(option.Pair)
					c.finishPosition(SeasonTypeLossMax, openedPosition)
					return
				}
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | PositionSide: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v, StopLoss:%v | PR.%%: %s",
					openedPosition.Pair,
					openedPosition.PositionSide,
					openedPosition.OrderFlag,
					openedPosition.Quantity,
					openedPosition.AvgPrice,
					openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					openedPosition.StopLossPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
				)
			}
		}
	}
}
