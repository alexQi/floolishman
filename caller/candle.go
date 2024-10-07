package caller

import (
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"time"
)

type Candle struct {
	Common
}

func (c *Candle) Start() {
	go c.Listen()
}

func (s *Candle) EventCallClose(pair string) {
	s.closePosition(s.pairOptions[pair])
}

func (c *Candle) finishPosition(seasonType SeasonType, position *model.Position, limit float64, stopPrice float64) {
	var closeSideType model.SideType
	if model.PositionSideType(position.PositionSide) == model.PositionSideTypeLong {
		closeSideType = model.SideTypeSell
	} else {
		closeSideType = model.SideTypeBuy
	}
	// 判断仓位方向为反方向，平掉现有仓位
	_, err := c.broker.CreateOrderStopLimit(
		closeSideType,
		model.PositionSideType(position.PositionSide),
		position.Pair,
		position.Quantity,
		limit,
		stopPrice,
		model.OrderExtra{
			Leverage:             position.Leverage,
			OrderFlag:            position.OrderFlag,
			LongShortRatio:       position.LongShortRatio,
			MatcherStrategyCount: position.MatcherStrategyCount,
		},
	)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 删除止损时间限制配置
	c.positionTimeouts.Delete(position.OrderFlag)
	utils.Log.Infof("[POSITION - %s] %s", seasonType, position.String())
	// 查询当前orderFlag所有的止损单，全部取消
	lossOrders, err := c.broker.GetOrdersForPostionLossUnfilled(position.OrderFlag)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	for _, lossOrder := range lossOrders {
		// 取消之前的止损单
		err = c.broker.Cancel(*lossOrder)
		if err != nil {
			utils.Log.Error(err)
			return
		}
	}
}

func (c *Candle) closePosition(option *model.PairOption) {
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
	currentTime := time.Now()
	if c.setting.CheckMode == "candle" {
		currentTime, _ = c.lastUpdate.Get(option.Pair)
	}
	var stopLossPrice float64
	// 与当前方向相反有仓位,计算相对分界线距离，多空比达到反手标准平仓
	// ***********************
	for _, openedPosition := range openedPositions {
		// 计算止损价格
		if option.MaxMarginLossRatio > 0 {
			stopLossDistance := calc.StopLossDistance(option.MaxMarginLossRatio, openedPosition.AvgPrice, float64(option.Leverage))
			if openedPosition.PositionSide == string(model.PositionSideTypeLong) {
				stopLossPrice = openedPosition.AvgPrice - stopLossDistance
			} else {
				stopLossPrice = openedPosition.AvgPrice + stopLossDistance
			}
		}
		// 记录利润比
		profitRatio := calc.ProfitRatio(
			model.SideType(openedPosition.Side),
			openedPosition.AvgPrice,
			currentPrice,
			float64(option.Leverage),
			openedPosition.Quantity,
		)
		pairCurrentProfit, _ := c.pairCurrentProfit.Get(option.Pair)
		// 监控已成交仓位，记录订单成交时间+指定时间作为时间止损
		positionTimeout, ok := c.positionTimeouts.Get(openedPosition.OrderFlag)
		if !ok {
			positionTimeout = openedPosition.CreatedAt.Add(time.Duration(c.setting.PositionTimeOut) * time.Minute)
			c.positionTimeouts.Set(openedPosition.OrderFlag, positionTimeout)
		}
		// 时间未达到新的止损限制时间
		if currentTime.After(positionTimeout) {
			utils.Log.Infof(
				"[POSITION - CLOSE] %s | Current: %v | PR.%%: %.2f%%, MaxProfit: %.2f%% (Position Timeout)",
				openedPosition.String(),
				currentPrice,
				profitRatio*100,
				pairCurrentProfit.MaxProfit*100,
			)
			c.resetPairProfit(option.Pair)
			c.finishPosition(SeasonTypeTimeout, openedPosition, currentPrice, currentPrice)
			continue
		}
		// 判断当前利润比是否小于锁定利润比，小于则平仓
		if profitRatio <= pairCurrentProfit.Close && pairCurrentProfit.IsLock {
			utils.Log.Infof(
				"[POSITION - CLOSE] %s | Current: %v | PR.%%: %.2f%% < ProfitClose: %.2f%%, MaxProfit: %.2f%%",
				openedPosition.String(),
				currentPrice,
				profitRatio*100,
				pairCurrentProfit.Close*100,
				pairCurrentProfit.MaxProfit*100,
			)
			// 重置交易对盈利
			c.resetPairProfit(option.Pair)
			c.finishPosition(SeasonTypeProfitBack, openedPosition, currentPrice, currentPrice)
			return
		}
		// ****************
		if profitRatio > 0 {
			// 判断是否已锁定利润比
			if pairCurrentProfit.IsLock == false {
				// 小于触发值时，记录当前利润比
				if profitRatio < pairCurrentProfit.Floor {
					utils.Log.Infof(
						"[POSITION - WATCH] %s | Current: %v | PR.%%: %.2f%% < ProfitTriggerRatio: %.2f%%",
						openedPosition.String(),
						currentPrice,
						profitRatio*100,
						pairCurrentProfit.Floor*100,
					)
					return
				}
			} else {
				if profitRatio < pairCurrentProfit.Floor {
					// 当前利润比触发值，之前已经有Close时，判断当前利润比是否比上次设置的利润比大
					if profitRatio <= pairCurrentProfit.Close+pairCurrentProfit.Decrease {
						utils.Log.Infof(
							"[POSITION - WATCH] %s | Current: %v | PR.%%: %.2f%% < ProfitFloorRatio: %.2f%%, LockProfit: %.2f%%, MaxProfit: %.2f%%",
							openedPosition.String(),
							currentPrice,
							profitRatio*100,
							pairCurrentProfit.Floor*100,
							pairCurrentProfit.Close*100,
							pairCurrentProfit.MaxProfit*100,
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

			pairCurrentProfit.IsLock = true
			pairCurrentProfit.MaxProfit = profitRatio
			pairCurrentProfit.Close = profitRatio - pairCurrentProfit.Decrease
			c.pairCurrentProfit.Set(option.Pair, pairCurrentProfit)
			// 盈利递增时修改时间止损结束时间
			c.positionTimeouts.Set(openedPosition.OrderFlag, currentTime.Add(time.Duration(c.setting.PositionTimeOut)*time.Minute))

			utils.Log.Infof(
				"[POSITION - PROFIT] %s | Current: %v | PR.%%: %.2f%% > NewProfitRatio: %.2f%%",
				openedPosition.String(),
				currentPrice,
				profitRatio*100,
				pairCurrentProfit.Close*100,
			)
		} else {
			if pairCurrentProfit.IsLock {
				utils.Log.Infof(
					"[POSITION - HOLD] %s | Current: %v | PR.%%: %.2f%% < ProfitCloseRatio: %.2f%%, MaxProfit: %.2f%%",
					openedPosition.String(),
					currentPrice,
					profitRatio*100,
					pairCurrentProfit.Close*100,
					pairCurrentProfit.MaxProfit*100,
				)
			} else {
				if option.MaxMarginLossRatio > 0 {
					// 亏损盈利比已大于最大
					if calc.Abs(profitRatio) > option.MaxMarginLossRatio {
						utils.Log.Infof(
							"[POSITION - CLOSE] %s | Current: %v | PR.%%: %.2f%% > MaxLoseRatio %.2f%%, MaxProfit: %.2f%%",
							openedPosition.String(),
							currentPrice,
							profitRatio*100,
							option.MaxMarginLossRatio*100,
							pairCurrentProfit.MaxProfit*100,
						)
						c.resetPairProfit(option.Pair)
						c.finishPosition(SeasonTypeLossMax, openedPosition, stopLossPrice, stopLossPrice)
						return
					}
					utils.Log.Infof(
						"[POSITION - HOLD] %s | Current: %v | PR.%%: %.2f%% < MaxLoseRatio: %.2f%%, MaxProfit: %.2f%%",
						openedPosition.String(),
						currentPrice,
						profitRatio*100,
						option.MaxMarginLossRatio*100,
						pairCurrentProfit.MaxProfit*100,
					)
				} else {
					if (openedPosition.PositionSide == string(model.PositionSideTypeLong) && currentPrice <= openedPosition.StopLossPrice) ||
						(openedPosition.PositionSide == string(model.PositionSideTypeShort) && currentPrice >= openedPosition.StopLossPrice) {
						utils.Log.Infof(
							"[POSITION - CLOSE] %s | Current: %v, StopLoss:%v | PR.%%: %.2f%%, MaxProfit: %.2f%%",
							openedPosition.String(),
							currentPrice,
							openedPosition.StopLossPrice,
							profitRatio*100,
							pairCurrentProfit.MaxProfit*100,
						)
						c.resetPairProfit(option.Pair)
						c.finishPosition(SeasonTypeLossMax, openedPosition, openedPosition.StopLossPrice, openedPosition.StopLossPrice)
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
}
