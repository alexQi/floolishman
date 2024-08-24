package caller

import (
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"time"
)

type Scoop struct {
	Common
}

func (c *Scoop) Start() {
	go c.Listen()
}

func (c *Scoop) closePosition(option *model.PairOption) {
	c.mu.Lock()         // 加锁
	defer c.mu.Unlock() // 解锁
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
			lossRatio := option.MaxMarginLossRatio * float64(option.Leverage)
			// 亏损盈利比已大于最大
			if calc.Abs(profitRatio) > lossRatio {
				utils.Log.Infof(
					"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > MaxLoseRatio %s",
					openedPosition.Pair,
					openedPosition.OrderFlag,
					openedPosition.Quantity,
					openedPosition.AvgPrice,
					openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", lossRatio*100),
				)
				c.resetPairProfit(option.Pair)
				c.finishPosition(SeasonTypeLossMax, openedPosition)
				return
			}
			utils.Log.Infof(
				"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < LossStepRatio: %s",
				openedPosition.Pair,
				openedPosition.OrderFlag,
				openedPosition.Quantity,
				openedPosition.AvgPrice,
				openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				fmt.Sprintf("%.2f%%", StepMoreRatio*100),
			)
		}
	}
}
