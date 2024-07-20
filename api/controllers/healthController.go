package controllers

import (
	"github.com/kataras/iris/v12"
)

type HealthController struct {
	BaseController
}

// Live 存活探针
func (c *HealthController) Live(ctx iris.Context) error {
	return ctx.JSON(map[string]string{
		"code":  "200",
		"error": "welcome to floolishman.co",
	})
}
