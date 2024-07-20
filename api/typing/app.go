package typing

type Device struct {
	Imei         string `json:"imei"`         // 设备识别码 IMEI
	Oaid         string `json:"oaid"`         // 设备识别码 OAID
	Idfa         string `json:"idfa"`         // 设备识别码 IDFA
	ImeiMd5      string `json:"imeiMd5"`      // 设备识别码 IMEI MD5
	OaidMd5      string `json:"oaidMd5"`      // 设备识别码 OAID MD5
	IdfaMd5      string `json:"idfaMd5"`      // 设备识别码 IDFA MD5
	MacMd5       string `json:"macMd5"`       // 设备识别码 MAC  MD5
	AndroidIdMd5 string `json:"androidIdMd5"` // 设备识别码 AndroidId MD5
}

type QueryLog struct {
	Request   RtaRequest `json:"request"`
	IsCache   bool       `json:"isCache"`
	IsTarget  bool       `json:"isTarget"`
	ItemTasks []string   `json:"itemTasks"`
}
