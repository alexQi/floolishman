package routes

import (
	"floolishman/api/routes/api"
	"github.com/kataras/iris/v12"
)

// ApiRoutes api路由加载
func ApiRoutes(app *iris.Application) {
	// 默认路由
	api.BaseRoutes(app)
	// Debug路由
	api.PprofRoutes(app)
	// 存活探针
	healthRoutes := app.Party("/health")
	{
		api.HealthRoutes(healthRoutes)
	}
	PositionRoutes := app.Party("/v1/position")
	{
		api.PositionRoutes(PositionRoutes)
	}
	CallerRoutes := app.Party("/v1/caller")
	{
		api.CallerRoutes(CallerRoutes)
	}
	PairRoutes := app.Party("/v1/pair")
	{
		api.PairRoutes(PairRoutes)
	}
}
