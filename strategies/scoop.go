package strategies

import (
	"bytes"
	"encoding/json"
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"reflect"
)

type Scoop struct {
	BaseStrategy
}

// SortScore
func (s Scoop) SortScore() float64 {
	return 90
}

// Timeframe
func (s Scoop) Timeframe() string {
	return "30m"
}

func (s Scoop) WarmupPeriod() int {
	return 96 // 预热期设定为50个数据点
}

func (s Scoop) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	// 计算布林带宽度
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower
	df.Metadata["bbWidth"] = bbWidth
	// 检查插针
	upperPinRates, lowerPinRates, upperShadows, lowerShadows := indicator.PinBars(df.Open, df.Close, df.High, df.Low)
	df.Metadata["upperPinRates"] = upperPinRates
	df.Metadata["lowerPinRates"] = lowerPinRates
	df.Metadata["upperShadows"] = upperShadows
	df.Metadata["lowerShadows"] = lowerShadows
	// 计算MACD指标
	macdLine, signalLine, hist := indicator.MACD(df.Close, 8, 17, 5)
	df.Metadata["macd"] = macdLine
	df.Metadata["signal"] = signalLine
	df.Metadata["hist"] = hist
	// 其他指标
	df.Metadata["priceRate"] = indicator.PriceRate(df.Open, df.Close)
	df.Metadata["rsi"] = indicator.RSI(df.Close, 7)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Scoop) OnCandle(option *model.PairOption, df *model.Dataframe) model.PositionStrategy {
	lastPrice := df.Close.Last(0)
	prevPrice := df.Close.Last(1)

	prevHigh := df.High.Last(1)
	lastHigh := df.High.Last(0)

	prevLow := df.Low.Last(1)
	lastLow := df.Low.Last(0)

	prevBbUpper := df.Metadata["bbUpper"].Last(1)
	lastBbUpper := df.Metadata["bbUpper"].Last(0)
	prevBbLower := df.Metadata["bbLower"].Last(1)
	lastBbLower := df.Metadata["bbLower"].Last(0)

	strategyPosition := model.PositionStrategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		LastAtr:      df.Metadata["atr"].Last(1) * 1.5,
		OpenPrice:    lastPrice,
	}

	penuRsi := df.Metadata["rsi"].Last(2)
	prevRsi := df.Metadata["rsi"].Last(1)
	lastRsi := df.Metadata["rsi"].Last(0)

	prevPriceRate := calc.Abs(df.Metadata["priceRate"].Last(1))

	macd := df.Metadata["macd"]
	signal := df.Metadata["signal"]
	upperPinRates := df.Metadata["upperPinRates"]
	lowerPinRates := df.Metadata["lowerPinRates"]
	upperShadows := df.Metadata["upperShadows"]
	lowerShadows := df.Metadata["lowerShadows"]

	prevSignal := signal.Last(1)
	prevMacd := macd.Last(1)
	penuUpperPinRate := upperPinRates.Last(2)
	penuLowerPinRate := lowerPinRates.Last(2)
	prevUpperPinRate := upperPinRates.Last(1)
	prevLowerPinRate := lowerPinRates.Last(1)
	lastUpperPinRate := upperPinRates.Last(0)
	lastLowerPinRate := lowerPinRates.Last(0)

	var upperShadowChangeRate, lowerShadowChangeRate float64
	lastUpperShadow := upperShadows.Last(0)
	lastLowerShadow := lowerShadows.Last(0)
	prevUpperShadow := upperShadows.Last(1)
	prevLowerShadow := lowerShadows.Last(1)
	if prevUpperShadow == 0 {
		upperShadowChangeRate = 0
	} else {
		upperShadowChangeRate = lastUpperShadow / prevUpperShadow
	}
	if prevLowerShadow == 0 {
		lowerShadowChangeRate = 0
	} else {
		lowerShadowChangeRate = lastLowerShadow / prevLowerShadow
	}
	prevMacdDiffRate := (prevMacd - prevSignal) / prevSignal

	penuAmplitude := indicator.AMP(df.Open.Last(2), df.High.Last(2), df.Low.Last(2))
	prevAmplitude := indicator.AMP(df.Open.Last(1), df.High.Last(1), df.Low.Last(1))

	openParams := map[string]interface{}{
		"prevPriceRate":    prevPriceRate / 0.02,
		"prevMacdDiffRate": prevMacdDiffRate,
		"prevPinLenRate":   calc.Abs(prevUpperPinRate - prevLowerPinRate),

		"lastRsi":               lastRsi,
		"prevRsi":               prevRsi,
		"penuRsi":               penuRsi,
		"penuAmplitude":         penuAmplitude,
		"prevAmplitude":         prevAmplitude,
		"penuUpperPinRate":      penuUpperPinRate,
		"penuLowerPinRate":      penuLowerPinRate,
		"prevUpperPinRate":      prevUpperPinRate,
		"prevLowerPinRate":      prevLowerPinRate,
		"upperShadowChangeRate": upperShadowChangeRate,
		"lowerShadowChangeRate": lowerShadowChangeRate,
		"openAt":                df.LastUpdate.In(Loc).Format("2006-01-02 15:04:05"),

		"prevPrice": prevPrice,
		"lastPrice": lastPrice,
	}

	var lastRsiChange, rsiSeedRate, decayFactorFloor, decayFactorAmplitude, decayFactorDistance, floor, upper, distanceRate, limitShadowChangeRate float64

	floorK := 1.077993
	distanceK := 1.445
	shadowK := 0.4405
	amplitudeK := 1.1601
	datum := 1.2
	deltaRsiRatio := 0.1
	baseFloor := 10.0
	baseUpper := 13.0
	baseDistanceRate := 0.275

	if prevRsi > 40 && prevRsi > lastRsi && prevPriceRate*prevUpperPinRate > 0.00125 {
		lastRsiChange = prevRsi - lastRsi
		rsiSeedRate = (prevRsi - 50.0) / 50.0

		openParams["positionSide"] = string(model.SideTypeSell)
		openParams["prevBollingCrossRate"] = prevHigh / prevBbUpper
		openParams["prevCloseCrossRate"] = prevPrice / prevBbUpper
		openParams["lastBollingCrossRate"] = lastHigh / lastBbUpper
		openParams["lastCloseCrossRate"] = lastPrice / lastBbUpper
		openParams["prevPricePinRate"] = prevPriceRate / 0.02 * prevUpperPinRate
		openParams["lastShadowChangeRate"] = upperShadowChangeRate
		if penuLowerPinRate > 0 {
			openParams["penuPinDiffRate"] = penuUpperPinRate / penuLowerPinRate
		} else {
			openParams["penuPinDiffRate"] = 0.0
		}
		if prevLowerPinRate > 0 {
			openParams["prevPinDiffRate"] = prevUpperPinRate / prevLowerPinRate
		} else {
			openParams["prevPinDiffRate"] = 0.0
		}
		if lastLowerPinRate > 0 {
			openParams["lastPinDiffRate"] = lastUpperPinRate / lastLowerPinRate
		} else {
			openParams["lastPinDiffRate"] = 0.0
		}
		openParams["lastRsiExtreme"] = (lastRsi - 50.0) / 50.0
		openParams["prevRsiExtreme"] = rsiSeedRate
		openParams["penuRsiExtreme"] = (penuRsi - 50.0) / 50.0
		openParams["prevAmplitudePinRate"] = prevAmplitude * prevUpperPinRate
		openParams["prevReserveAmplitudePinRate"] = prevAmplitude * prevLowerPinRate

		openParams["prevRsiChange"] = prevRsi - penuRsi
		openParams["lastRsiChange"] = lastRsiChange

		strategyPosition.Side = string(model.SideTypeSell)

		decayFactorFloor = calc.CalculateFactor(rsiSeedRate, floorK)
		decayFactorAmplitude = calc.CalculateFactor(prevAmplitude, amplitudeK)
		decayFactorDistance = calc.CalculateFactor(rsiSeedRate, distanceK-decayFactorAmplitude)

		if prevRsi >= 50 {
			floor = decayFactorFloor * baseFloor
			upper = math.Exp(floorK*deltaRsiRatio) * floor
			distanceRate = decayFactorDistance * baseDistanceRate
		} else {
			floor = baseFloor
			upper = baseUpper
			distanceRate = baseDistanceRate
		}

		limitShadowChangeRate = calc.CalculateRate(prevAmplitude*rsiSeedRate, datum, shadowK)

		strategyPosition.Useable = 1
		//strategyPosition.Score = 100 * rsiSeedRate
		//
		//if lastRsiChange > floor &&
		//	lastRsiChange < upper {
		//	if upperShadowChangeRate > limitShadowChangeRate {
		//		strategyPosition.Useable = 1
		//		strategyPosition.Score = 100 * rsiSeedRate
		//	}
		//}
	}
	if prevRsi < 60 && lastRsi > prevRsi && prevPriceRate*prevUpperPinRate > 0.00125 {
		lastRsiChange = lastRsi - prevRsi
		rsiSeedRate = (50 - prevRsi) / 50

		openParams["positionSide"] = string(model.SideTypeBuy)
		openParams["prevBollingCrossRate"] = prevBbLower / prevLow
		openParams["prevCloseCrossRate"] = prevBbLower / prevPrice
		openParams["lastBollingCrossRate"] = lastBbLower / lastLow
		openParams["lastCloseCrossRate"] = lastBbLower / lastPrice
		openParams["prevPricePinRate"] = prevPriceRate / 0.02 * prevLowerPinRate
		openParams["lastShadowChangeRate"] = lowerShadowChangeRate
		if penuUpperPinRate > 0 {
			openParams["penuPinDiffRate"] = penuLowerPinRate / penuUpperPinRate
		} else {
			openParams["penuPinDiffRate"] = 0.0
		}
		if prevUpperPinRate > 0 {
			openParams["prevPinDiffRate"] = prevLowerPinRate / prevUpperPinRate
		} else {
			openParams["prevPinDiffRate"] = 0.0
		}
		if lastUpperPinRate > 0 {
			openParams["lastPinDiffRate"] = lastLowerPinRate / lastUpperPinRate
		} else {
			openParams["lastPinDiffRate"] = 0.0
		}
		openParams["lastRsiExtreme"] = (50.0 - lastRsi) / 50.0
		openParams["prevRsiExtreme"] = rsiSeedRate
		openParams["penuRsiExtreme"] = (50.0 / penuRsi) / 50.0
		openParams["prevAmplitudePinRate"] = prevAmplitude * prevLowerPinRate
		openParams["prevReserveAmplitudePinRate"] = prevAmplitude * prevUpperPinRate

		openParams["prevRsiChange"] = penuRsi - prevRsi
		openParams["lastRsiChange"] = lastRsiChange

		strategyPosition.Side = string(model.SideTypeBuy)

		decayFactorFloor = calc.CalculateFactor(rsiSeedRate, floorK)
		decayFactorAmplitude = calc.CalculateFactor(prevAmplitude, amplitudeK)
		decayFactorDistance = calc.CalculateFactor(rsiSeedRate, distanceK-decayFactorAmplitude)

		if prevRsi < 50 {
			floor = decayFactorFloor * baseFloor
			upper = math.Exp(floorK*deltaRsiRatio) * floor
			distanceRate = decayFactorDistance * baseDistanceRate
		} else {
			floor = baseFloor
			upper = baseUpper
			distanceRate = baseDistanceRate
		}

		limitShadowChangeRate = calc.CalculateRate(prevAmplitude*rsiSeedRate, datum, shadowK)

		strategyPosition.Useable = 1
		//strategyPosition.Score = 100 * rsiSeedRate

		//if lastRsiChange > floor &&
		//	lastRsiChange < upper {
		//	if lowerShadowChangeRate > limitShadowChangeRate {
		//		strategyPosition.Useable = 1
		//		strategyPosition.Score = 100 * rsiSeedRate
		//	}
		//}
	}

	if strategyPosition.Useable > 0 {
		// 应用权重，获取加权后的参数
		weightedParams := s.applyWeights(openParams)

		// 构造预测模型的输入
		payload := map[string]interface{}{
			"instances": []map[string][]float64{weightedParams},
		}

		// 调用预测模型
		prediction, err := callPredictionAPI(payload)
		if err != nil {
			utils.Log.Error("预测API调用失败:", err)
		}

		// 判断预测结果并进行操作
		if prediction > 0.5 {
			utils.Log.Info("预测结果支持交易，继续执行策略...")
			strategyPosition.Score = prediction * 100
		} else {
			strategyPosition.Useable = 0
		}
	}

	if strategyPosition.Useable > 0 {
		stopLossDistance := calc.StopLossDistance(distanceRate, strategyPosition.OpenPrice, float64(option.Leverage))
		if strategyPosition.Side == string(model.SideTypeBuy) {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice - stopLossDistance
		} else {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice + stopLossDistance
		}
		openParams["floor"] = floor
		openParams["upper"] = upper
		openParams["limitShadowChangeRate"] = limitShadowChangeRate
		openParams["distanceRate"] = distanceRate
		openParams["openPrice"] = strategyPosition.OpenPrice
		// 将 map 转换为 JSON 字符串
		openParamsBytes, err := json.Marshal(openParams)
		if err != nil {
			utils.Log.Error("错误：", err)
		}
		strategyPosition.OpenParams = string(openParamsBytes)
		utils.Log.Tracef("[PARAMS] %s", strategyPosition.OpenParams)
	}

	return strategyPosition
}

// 获取权重并加权参数
func (s Scoop) applyWeights(params map[string]interface{}) map[string][]float64 {
	// 获取策略的权重
	weights := s.getWeight()
	weightsFeatures := s.getWeightedFeatures()

	weightedParams := map[string][]float64{}
	for code, weight := range weights {
		// 确保参数存在且是 float64 类型
		paramValue, exists := params[weightsFeatures[code]]
		if !exists {
			continue
		}
		paramFloat, ok := paramValue.(float64)
		if !ok {
			continue
		}
		weightedParams[fmt.Sprintf("%s_weighted", weightsFeatures[code])] = []float64{paramFloat * weight}
	}
	return weightedParams
}

// get_weighted_features: 根据策略的getWeight函数动态生成带权重的特征
func (s *Scoop) getWeightedFeatures() map[string]string {
	return map[string]string{
		"PV_PR":   "prevPriceRate",
		"LT_MDR":  "prevMacdDiffRate",
		"PV_PLR":  "prevPinLenRate",
		"PV_BRC":  "prevBollingCrossRate",
		"PV_CCR":  "prevCloseCrossRate",
		"LT_BCR":  "lastBollingCrossRate",
		"LT_CCR":  "lastCloseCrossRate",
		"PV_PPR":  "prevPricePinRate",
		"LT_SCR":  "lastShadowChangeRate",
		"PN_PDR":  "penuPinDiffRate",
		"PV_PDR":  "prevPinDiffRate",
		"LT_PDR":  "lastPinDiffRate",
		"LT_RE":   "lastRsiExtreme",
		"PV_RE":   "prevRsiExtreme",
		"PN_RE":   "penuRsiExtreme",
		"PV_APR":  "prevAmplitudePinRate",
		"PV_RAPR": "prevReserveAmplitudePinRate",
		"PV_RC":   "prevRsiChange",
		"LT_RC":   "lastRsiChange",
	}
}

func (s *Scoop) getWeight() map[string]float64 {
	return map[string]float64{"LT_BCR": 0.44337187592256133, "LT_CCR": 0.3190061287414348, "LT_MDR": 0.43602577293074507, "LT_PDR": 0.38856345261608627, "LT_RC": 0.18921887928950035, "LT_RE": 0.46113026793669787, "LT_SCR": 0.3599852150992393, "PN_PDR": 0.376417729475162, "PN_RE": 0.2300049452871201, "PV_APR": 0.3861133690203762, "PV_BRC": 0.30786708627313897, "PV_CCR": 0.43853565222629937, "PV_PDR": 0.29004068594416466, "PV_PLR": 0.2815615480092416, "PV_PPR": 0.3923169199411263, "PV_PR": 0.41724154861594753, "PV_RAPR": -0.3137366372544079, "PV_RC": 0.21646002915119092, "PV_RE": 0.43599669279005493}
}

// 调用预测模型的函数
func callPredictionAPI(payload map[string]interface{}) (float64, error) {
	// 定义预测API的URL
	apiUrl := "http://localhost:8501/v1/models/BASIC/versions/1:predict"

	// 将 payload 转换为 JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	// 发起 HTTP POST 请求
	resp, err := http.Post(apiUrl, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// 读取 API 响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// 解析 API 响应
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, err
	}

	// 获取预测结果
	predictions := result["predictions"].([]interface{})
	firstPrediction := predictions[0].([]interface{})[0].(float64)
	return firstPrediction, nil
}
