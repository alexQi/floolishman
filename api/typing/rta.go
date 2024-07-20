package typing

// 登录请求验证
type RtaRequest struct {
	Channel  string `json:"channel" validate:"required"`
	MediaNo  string `json:"mediaNo"`
	Platform string `json:"platform" validate:"required"`
	ItemTask string `json:"itemTask"`
	Device   Device `json:"device" validate:"required"`
	Os       string `json:"os"`
}

type RtaResult struct {
	Platform string `json:"platform"`
	Status   bool   `json:"status"`
}

type RtaBatchResult struct {
	RtaResult
	ItemTask string `json:"itemTask"`
}

type RtaMultiResult struct {
	Platform string   `json:"platform"`
	TaskIds  []string `json:"taskIds"`
}
