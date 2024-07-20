package api

import (
	"github.com/kataras/iris/v12"
)

func BaseRoutes(app *iris.Application) {
	app.Get("/", func(ctx iris.Context) {
		_ = ctx.JSON(map[string]string{
			"code":  "200",
			"error": "welcome to floolishman.co",
		})
	})
}
