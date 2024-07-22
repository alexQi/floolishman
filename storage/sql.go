package storage

import (
	"time"

	"gorm.io/gorm"

	"floolishman/model"
)

type SQL struct {
	db *gorm.DB
}

// FromSQL creates a new SQL connections for orders storage. Example of usage:
//
//	import "github.com/glebarez/sqlite"
//	storage, err := storage.FromSQL(sqlite.Open("sqlite.db"), &gorm.Config{})
//	if err != nil {
//	}
func FromSQL(dialect gorm.Dialector, opts ...gorm.Option) (Storage, error) {
	db, err := gorm.Open(dialect, opts...)
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	err = db.AutoMigrate(&model.Order{})
	if err != nil {
		return nil, err
	}
	err = db.AutoMigrate(&model.Position{})
	if err != nil {
		return nil, err
	}
	err = db.AutoMigrate(&model.GuiderItem{})
	if err != nil {
		return nil, err
	}
	err = db.AutoMigrate(&model.GuiderSymbolConfig{})
	if err != nil {
		return nil, err
	}
	err = db.AutoMigrate(&model.GuiderPosition{})
	if err != nil {
		return nil, err
	}

	return &SQL{
		db: db,
	}, nil
}

func (s *SQL) ResetTables() error {
	tables := []interface{}{
		&model.Order{},
		&model.Position{},
		&model.GuiderItem{},
		&model.GuiderSymbolConfig{},
	}

	// 删除所有表
	for _, table := range tables {
		err := s.db.Migrator().DropTable(table)
		if err != nil {
			return err
		}
	}

	// 重新创建所有表
	for _, table := range tables {
		err := s.db.AutoMigrate(table)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQL) CreateGuiderItems(guiderItems []model.GuiderItem) error {
	var og model.GuiderItem
	liveCopyPortfolioIds := []string{}
	for _, item := range guiderItems {
		liveCopyPortfolioIds = append(liveCopyPortfolioIds, item.CopyPortfolioId)
		og = model.GuiderItem{}
		s.db.Where("copy_portfolio_id=?", item.CopyPortfolioId).First(&og)
		// 更新操作
		if og.ID > 0 {
			item.ID = og.ID
			result := s.db.Save(&item)
			if result.Error != nil {
				return result.Error
			}
		} else {
			//新增
			result := s.db.Create(&item)
			if result.Error != nil {
				return result.Error
			}
		}
	}
	// 清除过期的跟单员
	err := s.db.Where("copy_portfolio_id NOT IN ?", liveCopyPortfolioIds).Delete(&model.GuiderItem{}).Error
	if err != nil {
		return err
	}
	// 清除过期的跟单员交易对配置
	err = s.db.Where("portfolio_id NOT IN ?", liveCopyPortfolioIds).Delete(&model.GuiderSymbolConfig{}).Error
	if err != nil {
		return err
	}
	// 清除过期的跟单员仓位配置
	err = s.db.Where("portfolio_id NOT IN ?", liveCopyPortfolioIds).Delete(&model.GuiderPosition{}).Error
	if err != nil {
		return err
	}
	return nil
}

// Orders filter a list of orders given a filter
func (s *SQL) GetGuiderItems() ([]*model.GuiderItem, error) {
	guiderItems := make([]*model.GuiderItem, 0)
	result := s.db.Find(&guiderItems)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return guiderItems, result.Error
	}
	return guiderItems, nil
}

func (s *SQL) GetGuiderItemByPortfolioId(portfolioId string) (*model.GuiderItem, error) {
	guiderItem := &model.GuiderItem{}
	result := s.db.Where("copy_portfolio_id=?", portfolioId).First(&guiderItem)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return guiderItem, result.Error
	}
	return guiderItem, nil
}

func (s *SQL) GetSymbolConfigByPortfolioId(portfolioId string, pair string) (*model.GuiderSymbolConfig, error) {
	symbolConfig := &model.GuiderSymbolConfig{}
	result := s.db.Where("portfolio_id=?", portfolioId).Where("symbol=?", pair).First(&symbolConfig)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return symbolConfig, result.Error
	}
	return symbolConfig, nil
}

func (s *SQL) CreateSymbolConfigs(guiderSymbolConfigs []model.GuiderSymbolConfig) error {
	var orgConfig model.GuiderSymbolConfig
	// 重新插入跟单交易对数据
	for _, item := range guiderSymbolConfigs {
		orgConfig = model.GuiderSymbolConfig{}
		s.db.Where("portfolio_id=?", item.PortfolioId).Where("symbol=?", item.Symbol).First(&orgConfig)
		// 更新操作
		if orgConfig.ID > 0 {
			item.ID = orgConfig.ID
			result := s.db.Save(&item)
			if result.Error != nil {
				return result.Error
			}
		} else {
			//新增
			result := s.db.Create(&item)
			if result.Error != nil {
				return result.Error
			}
		}
	}
	return nil
}

func (s *SQL) CreateGuiderPositions(portfolioIds []string, guiderPositions []*model.GuiderPosition) error {
	// 清除实时仓位记录
	err := s.db.Where("portfolio_id IN ?", portfolioIds).Delete(&model.GuiderPosition{}).Error
	if err != nil {
		return err
	}
	// 重新插入跟单交易对数据
	for _, item := range guiderPositions {
		result := s.db.Create(item)
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

func (s *SQL) GuiderPositions(portfolioIds []string) ([]*model.GuiderPosition, error) {
	guiderPositions := make([]*model.GuiderPosition, 0)
	result := s.db.Where("portfolio_id NOT IN ?", portfolioIds).Find(&guiderPositions)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return guiderPositions, result.Error
	}
	return guiderPositions, nil
}

func (s *SQL) CreateGuiderOrders(portfolioId string, guiderOrders []model.GuiderOrder) error {
	orgOrder := model.GuiderOrder{}
	// 重新插入跟单交易对数据
	for _, guiderOrder := range guiderOrders {
		s.db.Where("portfolio_id=?", guiderOrder.PortfolioId).Where("origin_id=?", guiderOrder.OriginId).First(&orgOrder)
		// 更新操作
		if orgOrder.ID > 0 {
			guiderOrder.ID = orgOrder.ID
			guiderOrder.PortfolioId = orgOrder.PortfolioId
			result := s.db.Save(&guiderOrder)
			if result.Error != nil {
				return result.Error
			}
		} else {
			//新增
			guiderOrder.PortfolioId = portfolioId
			result := s.db.Create(&guiderOrder)
			if result.Error != nil {
				return result.Error
			}
		}
	}
	return nil
}

// CreateOrder creates a new order in a SQL database
func (s *SQL) CreateOrder(order *model.Order) error {
	result := s.db.Create(order) // pass pointer of data to Create
	return result.Error
}

// UpdateOrder updates a given order
func (s *SQL) UpdateOrder(order *model.Order) error {
	o := model.Order{ID: order.ID}
	s.db.First(&o)
	o = *order
	result := s.db.Save(&o)
	return result.Error
}

func (s *SQL) Orders(filterParams OrderFilterParams) ([]*model.Order, error) {
	orders := make([]*model.Order, 0)
	query := s.db
	if len(filterParams.Pair) > 0 {
		query = query.Where("pair=?", filterParams.Pair)
	}
	if len(filterParams.OrderFlag) > 0 {
		query = query.Where("order_flag=?", filterParams.OrderFlag)
	}
	if len(filterParams.Statuses) > 0 {
		query = query.Where("status in ?", filterParams.Statuses)
	}
	if len(filterParams.OrderTypes) > 0 {
		query = query.Where("type in ?", filterParams.OrderTypes)
	}

	result := query.Find(&orders)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return orders, result.Error
	}
	return orders, nil
}

func (s *SQL) CreatePosition(position *model.Position) error {
	result := s.db.Create(position) // pass pointer of data to Create
	return result.Error
}

// UpdateOrder updates a given order
func (s *SQL) UpdatePosition(position *model.Position) error {
	o := model.Position{ID: position.ID}
	s.db.First(&o)
	o = *position
	result := s.db.Save(&o)
	return result.Error
}

func (s *SQL) Positions(filterParams PositionFilterParams) ([]*model.Position, error) {
	positions := make([]*model.Position, 0)
	query := s.db.Where("status=?", filterParams.Status)
	if len(filterParams.Pair) > 0 {
		query = query.Where("pair=?", filterParams.Pair)
	}
	if len(filterParams.OrderFlag) > 0 {
		query = query.Where("order_flag=?", filterParams.OrderFlag)
	}
	if len(filterParams.Side) > 0 {
		query = query.Where("side=?", filterParams.Side)
	}
	if len(filterParams.PositionSide) > 0 {
		query = query.Where("position_side=?", filterParams.PositionSide)
	}

	result := query.Find(&positions)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return positions, result.Error
	}
	return positions, nil
}

func (s *SQL) GetPosition(filterParams PositionFilterParams) (*model.Position, error) {
	position := &model.Position{}
	query := s.db.Where("status=?", filterParams.Status)
	if len(filterParams.Pair) > 0 {
		query = query.Where("pair=?", filterParams.Pair)
	}
	if len(filterParams.OrderFlag) > 0 {
		query = query.Where("order_flag=?", filterParams.OrderFlag)
	}
	if len(filterParams.Side) > 0 {
		query = query.Where("side=?", filterParams.Side)
	}
	if len(filterParams.PositionSide) > 0 {
		query = query.Where("position_side=?", filterParams.PositionSide)
	}

	result := query.First(position)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return position, result.Error
	}
	return position, nil
}
