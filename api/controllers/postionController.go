package controllers

import (
	"github.com/kataras/iris/v12"
)

type PositionController struct {
	BaseController
}

// Check 目标用户检测接口
func (c *PositionController) Check(ctx iris.Context) error {
	// 返回响应
	return ctx.JSON(map[string]interface{}{
		"status": "success",
	})
}
