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

func (s *Scoop) Indicators(df *model.Dataframe) {
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
	df.Metadata["avgVolume"] = indicator.EMA(df.Volume, 7)
	df.Metadata["priceRate"] = indicator.PriceRate(df.Open, df.Close)
	df.Metadata["rsi"] = indicator.RSI(df.Close, 7)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Scoop) OnCandle(option *model.PairOption, df *model.Dataframe) model.PositionStrategy {
	feature := s.extraFeatures(df)
	strategyPosition := model.PositionStrategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		LastAtr:      df.Metadata["atr"].Last(1) * 1.5,
		OpenPrice:    feature.LastPrice,
	}
	var decayFactorFloor, decayFactorAmplitude, decayFactorDistance, datum, shadowK float64

	floorK := 1.077993
	distanceK := 1.445
	amplitudeK := 1.1601
	deltaRsiRatio := 0.1
	baseFloor := 10.0
	baseDistanceRate := 0.275

	if feature.PrevRsi > feature.LastRsi {
		datum = 1.2
		shadowK = 0.4405

		strategyPosition.Side = string(model.SideTypeSell)

		decayFactorFloor = calc.CalculateFactor(feature.PrevRsiExtreme, floorK)
		decayFactorAmplitude = calc.CalculateFactor(feature.PrevAmplitude, amplitudeK)
		decayFactorDistance = calc.CalculateFactor(feature.PrevRsiExtreme, distanceK-decayFactorAmplitude)

		feature.RsiFloor = decayFactorFloor * baseFloor
		feature.RsiUpper = math.Exp(floorK*deltaRsiRatio) * feature.RsiFloor
		feature.DistanceRate = decayFactorDistance * baseDistanceRate
		feature.LimitShadowChangeRate = calc.CalculateRate(feature.PrevAmplitude*feature.PrevRsiExtreme, datum, shadowK)
		feature.PositionSide = strategyPosition.Side

		if calc.Abs(feature.LastRsiDiff) > feature.RsiFloor && calc.Abs(feature.LastRsiDiff) < feature.RsiUpper {
			strategyPosition.Useable = 1
			strategyPosition.Score = 100 * feature.PrevRsiExtreme
		}
	} else {
		datum = 2.6
		shadowK = 0.26

		strategyPosition.Side = string(model.SideTypeBuy)

		decayFactorFloor = calc.CalculateFactor(feature.PrevRsiExtreme, floorK)
		decayFactorAmplitude = calc.CalculateFactor(feature.PrevAmplitude, amplitudeK)
		decayFactorDistance = calc.CalculateFactor(feature.PrevRsiExtreme, distanceK-decayFactorAmplitude)

		feature.RsiFloor = decayFactorFloor * baseFloor
		feature.RsiUpper = math.Exp(floorK*deltaRsiRatio) * feature.RsiFloor
		feature.DistanceRate = decayFactorDistance * baseDistanceRate
		feature.LimitShadowChangeRate = calc.CalculateRate(feature.PrevAmplitude*feature.PrevRsiExtreme, datum, shadowK)
		feature.PositionSide = strategyPosition.Side

		if feature.LastRsiDiff > feature.RsiFloor && feature.LastRsiDiff < feature.RsiUpper {
			strategyPosition.Useable = 1
			strategyPosition.Score = 100 * feature.PrevRsiExtreme
		}
	}

	if strategyPosition.Useable > 0 {
		// 应用权重，获取加权后的参数
		weightedParams := s.applyWeights(feature)

		// 构造预测模型的输入
		payload := map[string]interface{}{
			"instances": []map[string][]float64{weightedParams},
		}

		// 调用预测模型
		prediction, regression, err := callPredictionAPI(payload)
		if err != nil {
			utils.Log.Error("预测API调用失败:", err)
		}

		// 判断预测结果并进行操作
		if prediction > 0.5 {
			utils.Log.Info("预测结果支持交易，继续执行策略...")
			strategyPosition.Score = prediction * 100
			feature.DistanceRate = regression
		} else {
			strategyPosition.Useable = 0
			strategyPosition.Score = prediction * 100
		}
	}

	if strategyPosition.Useable > 0 {
		stopLossDistance := calc.StopLossDistance(feature.DistanceRate, strategyPosition.OpenPrice, float64(option.Leverage))
		if strategyPosition.Side == string(model.SideTypeBuy) {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice - stopLossDistance
		} else {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice + stopLossDistance
		}
		feature.OpenPrice = strategyPosition.OpenPrice
		// 将 map 转换为 JSON 字符串
		openParamsBytes, err := json.Marshal(feature)
		if err != nil {
			utils.Log.Error("错误：", err)
		}
		strategyPosition.OpenParams = string(openParamsBytes)
		utils.Log.Tracef("[PARAMS] %s", strategyPosition.OpenParams)
	}

	return strategyPosition
}

// applyWeights 计算加权后的参数值
func (s *Scoop) applyWeights(feature *model.StrategyFeature) map[string][]float64 {
	// 获取策略的权重和对应的特性字段名
	weights := s.getWeight()                   // map[string]float64
	weightsFeatures := s.getWeightedFeatures() // map[string]string
	weightedParams := map[string][]float64{}

	for code, weight := range weights {
		// 获取对应字段名
		fieldName, exists := weightsFeatures[code]
		if !exists {
			continue
		}
		// 获取字段值，并确保它是 float64 类型
		value, exists := feature.GetFeatureValue(fieldName)
		if !exists {
			continue
		}
		// 类型断言，将 interface{} 转换为 float64
		floatValue, ok := value.(float64)
		if !ok {
			fmt.Printf("Warning: field %s is not of type float64\n", fieldName)
			continue
		}
		// 计算加权值并存储
		weightedParams[fmt.Sprintf("%s_weighted", fieldName)] = []float64{floatValue * weight}
	}

	return weightedParams
}

// get_weighted_features: 根据策略的getWeight函数动态生成带权重的特征
func (s *Scoop) getWeightedFeatures() map[string]string {
	return map[string]string{
		"LT_RE": "LastRsiExtreme",
		"PV_RE": "PrevRsiExtreme",
		"PN_RE": "PenuRsiExtreme",

		"LT_RC": "LastRsiDiff",
		"PV_RC": "PrevRsiDiff",
		"PN_RC": "PenuRsiDiff",

		"PV_AVR": "PrevAvgVolumeRate",
		"PN_AVR": "PenuAvgVolumeRate",

		"PV_PR": "PrevPriceRate",
		"PN_PR": "PenuPriceRate",

		"LT_SR": "LastShadowRate",
		"PV_SR": "PrevShadowRate",
		"PN_SR": "PenuShadowRate",

		"LT_USCR": "LastUpperShadowChangeRate",
		"PV_USCR": "PrevUpperShadowChangeRate",
		"PN_USCR": "PenuUpperShadowChangeRate",

		"LT_LSCR": "LastLowerShadowChangeRate",
		"PV_LSCR": "PrevLowerShadowChangeRate",
		"PN_LSCR": "PenuLowerShadowChangeRate",

		"LT_UPR": "LastUpperPinRate",
		"PV_UPR": "PrevUpperPinRate",
		"PN_UPR": "PenuUpperPinRate",

		"LT_LPR": "LastLowerPinRate",
		"PV_LPR": "PrevLowerPinRate",
		"PN_LPR": "PenuLowerPinRate",

		"LT_MDR": "LastMacdDiffRate",
		"PV_MDR": "PrevMacdDiffRate",
		"PN_MDR": "PenuMacdDiffRate",

		"LT_APR": "LastAmplitude",
		"PV_APR": "PrevAmplitude",
		"PN_APR": "PenuAmplitude",

		"LT_BCR": "LastBollingCrossRate",
		"PV_BCR": "PrevBollingCrossRate",
		"PN_BCR": "PenuBollingCrossRate",

		"LT_CCR": "LastCloseCrossRate",
		"PV_CCR": "PrevCloseCrossRate",
		"PN_CCR": "PenuCloseCrossRate",
	}
}

func (s *Scoop) getWeight() map[string]float64 {
	// JSON 数据
	jsonData := `{"LT_APR": 0.22901816012672907, "LT_BCR": 0.2707011889097887, "LT_CCR": 0.26832796104353657, "LT_LPR": 0.4882013284824216, "LT_LSCR": 0.45480472906360525, "LT_MDR": 0.3159608401805638, "LT_RC": 0.17445854016275036, "LT_RE": 0.37290396877920806, "LT_SR": 0.36571511977319804, "LT_UPR": 0.3381044113802033, "LT_USCR": 0.39544736283235404, "PN_APR": 0.29962618359934334, "PN_AVR": 0.48475502941566495, "PN_BCR": 0.4087734127169109, "PN_CCR": 0.2158564034202156, "PN_LPR": 0.3824147752178237, "PN_LSCR": 0.437689181457132, "PN_MDR": 0.32609769050303067, "PN_PR": 0.5569469523564967, "PN_RC": 0.31467340790473625, "PN_RE": 0.24073360677035743, "PN_SR": 0.22469906159015046, "PN_UPR": 0.44133224120538944, "PN_USCR": 0.30583248831915677, "PV_APR": 0.34644389531836584, "PV_AVR": 0.5820690724274873, "PV_BCR": 0.39386854446896974, "PV_CCR": 0.3358594741722827, "PV_LPR": 0.4415234080573177, "PV_LSCR": 0.4008156359081336, "PV_MDR": 0.34096587626057717, "PV_PR": 0.5824394426329575, "PV_RC": 0.24652565945019356, "PV_RE": 0.49029764095597855, "PV_SR": 0.5168727246846223, "PV_UPR": 0.3430740496536452, "PV_USCR": 0.3152594861925364}`

	// 定义 map[string]float64
	var data map[string]float64

	// 解析 JSON 数据
	err := json.Unmarshal([]byte(jsonData), &data)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
		return map[string]float64{}
	}
	return data
}

// 调用预测模型的函数
func callPredictionAPI(payload map[string]interface{}) (float64, float64, error) {
	// 定义预测API的URL
	apiUrl := "http://127.0.0.1:8501/v1/models/BASIC/versions/1:predict"

	// 将 payload 转换为 JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return 0, 0, err
	}

	// 发起 HTTP POST 请求
	resp, err := http.Post(apiUrl, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	// 读取 API 响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}

	// 解析 API 响应
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, 0, err
	}

	// 获取预测结果
	predictions := result["predictions"].([]interface{})
	firstPrediction := predictions[0].(map[string]interface{})

	// 获取回归结果
	regression := firstPrediction["regression"].([]interface{})[0].(float64)

	// 获取分类结果
	classification := firstPrediction["classification"].([]interface{})[0].(float64)

	return classification, calc.Abs(regression), nil
}
