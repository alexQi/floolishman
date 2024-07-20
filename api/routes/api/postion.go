package api

import (
	"floolishman/api/controllers"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/core/router"
)

func PositionRoutes(app router.Party) {
	c := controllers.PositionController{}
	// 定义路由handler 加载速率限制中间件
	app.Post("/check", func(ctx iris.Context) {
		err := c.Check(ctx)
		if err != nil {
			return
		}
	})
}
