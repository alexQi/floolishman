package service

import (
	"context"
	"floolishman/model"
	"floolishman/source"
	"floolishman/storage"
	"floolishman/types"
	"floolishman/utils"
	"time"
)

type ServiceGuider struct {
	ctx           context.Context
	storage       storage.Storage
	guiderConfigs map[string]map[string]string
	source        source.BaseSourceInterface
	ProxyOption   types.ProxyOption
	pairOptions   map[string]model.PairOption
}

func NewServiceGuider(ctx context.Context, guiderConfigs map[string]map[string]string, storage storage.Storage, proxyOption types.ProxyOption, pairOptions []model.PairOption) *ServiceGuider {
	binanceSource := &source.BinanceSource{}
	binanceSource.InitHttpClient(proxyOption)
	options := map[string]model.PairOption{}
	for _, pairOption := range pairOptions {
		options[pairOption.Pair] = pairOption
	}
	return &ServiceGuider{
		ctx:           ctx,
		guiderConfigs: guiderConfigs,
		source:        binanceSource,
		storage:       storage,
		pairOptions:   options,
	}
}

func (s *ServiceGuider) Start() {
	go func() {
		checkGuiderTick := time.NewTicker(5 * time.Second)
		checkConfigTick := time.NewTicker(5 * time.Second)
		for {
			select {
			// 5秒查询一次当前跟单情况
			case <-checkGuiderTick.C:
				s.syncGuider()
			case <-checkConfigTick.C:
				s.syncGuiderSymbolConfig()
			}
		}
	}()
	utils.Log.Info("Guider started.")
}

func (s *ServiceGuider) syncGuider() {
	guiderItems := []model.GuiderItem{}
	for account, config := range s.guiderConfigs {
		items, err := s.source.GetUserPortfolioList(config)
		if err != nil {
			utils.Log.Error(err)
		}
		for i := range items {
			items[i].Account = account
		}
		// 保存跟单项目
		guiderItems = append(guiderItems, items...)
	}
	// 保存项目列表
	err := s.storage.CreateGuiderItems(guiderItems)
	if err != nil {
		utils.Log.Error(err)
	}
	utils.Log.Infof("[Watchdog] synced all guiders, total:%v", len(guiderItems))
}
func (s *ServiceGuider) syncGuiderSymbolConfig() {
	guiderItems, err := s.storage.GetGuiderItems()
	if err != nil {
		return
	}
	symbolConfigs := []model.GuiderSymbolConfig{}
	for _, guiderItem := range guiderItems {
		itemConfigs, err := s.source.CheckGuiderSymbolConfig(guiderItem.CopyPortfolioId, s.guiderConfigs[guiderItem.Account])
		if err != nil {
			utils.Log.Error(err)
		}

		for i := range itemConfigs {
			if _, ok := s.pairOptions[itemConfigs[i].Symbol]; !ok {
				continue
			}
			itemConfigs[i].Account = guiderItem.Account
			itemConfigs[i].PortfolioId = guiderItem.CopyPortfolioId
			symbolConfigs = append(symbolConfigs, itemConfigs[i])
		}
	}
	// 保存每个项目的交易对配置
	err = s.storage.CreateSymbolConfigs(symbolConfigs)
	if err != nil {
		utils.Log.Error(err)
	}
	utils.Log.Infof("[Watchdog] synced all guider symbol config, total:%v", len(symbolConfigs))
}

// FetchPairNewOrder 获取当前交易对委托单 map[pair][PositionSide][]model.GuiderPosition
func (s *ServiceGuider) FetchPosition() (map[string]map[model.PositionSideType][]model.GuiderPosition, error) {
	guiderPosition := map[string]map[model.PositionSideType][]model.GuiderPosition{}
	guiderItems, err := s.storage.GetGuiderItems()
	if err != nil {
		return guiderPosition, err
	}
	tempUserPositions := []model.GuiderPosition{}
	for _, guiderItem := range guiderItems {
		userPositions, err := s.source.CheckUserPosition(guiderItem.CopyPortfolioId, s.guiderConfigs[guiderItem.Account])
		if err != nil {
			utils.Log.Error(err)
		}
		for i := range userPositions {
			userPositions[i].AvailQuote = guiderItem.MarginBalance + guiderItem.UnrealizedPnl
		}
		tempUserPositions = append(tempUserPositions, userPositions...)
	}
	for _, position := range tempUserPositions {
		if _, ok := guiderPosition[position.Symbol]; !ok {
			guiderPosition[position.Symbol] = make(map[model.PositionSideType][]model.GuiderPosition)
		}
		if _, ok := guiderPosition[position.Symbol][model.PositionSideType(position.PositionSide)]; !ok {
			guiderPosition[position.Symbol][model.PositionSideType(position.PositionSide)] = []model.GuiderPosition{}
		}
		guiderPosition[position.Symbol][model.PositionSideType(position.PositionSide)] = append(guiderPosition[position.Symbol][model.PositionSideType(position.PositionSide)], position)
	}
	return guiderPosition, nil
}

func (s *ServiceGuider) FetchPairConfig(portfolioId string, pair string) (*model.GuiderSymbolConfig, error) {
	return s.storage.GetSymbolConfigByPortfolioId(portfolioId, pair)
}

func (s *ServiceGuider) FetchGuiderDetail(portfolioId string) (*model.GuiderItem, error) {
	return s.storage.GetGuiderItemByPortfolioId(portfolioId)
}
