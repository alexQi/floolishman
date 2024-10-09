package controllers

import (
	"floolishman/types"
	"github.com/kataras/iris/v12"
)

type CallerController struct {
	BaseController
}

// Check 目标用户检测接口
func (c *CallerController) SwitchStatus(ctx iris.Context) error {
	data := map[string]interface{}{
		"code":    "0",
		"message": "success",
	}
	status := ctx.URLParamTrim("status")
	var callerStatus bool
	if status == "true" {
		callerStatus = true
	} else {
		callerStatus = false
	}
	types.CallerPauserChan <- types.CallerStatus{Status: callerStatus, PairStatuses: make([]types.PairStatus, 0)}
	// 返回响应
	return ctx.JSON(data)
}
