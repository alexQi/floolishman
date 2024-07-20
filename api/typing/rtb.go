package typing

type Geo struct {
	Province string `json:"province"`
	City     string `json:"city"`
}

type RtbDevice struct {
	Ua      string `json:"ua"`
	Geo     Geo    `json:"geo"`
	Didmd5  string `json:"didmd5"`
	Idfamd5 string `json:"idfamd5"`
	Oaid    string `json:"oaid"`
	Oaidmd5 string `json:"oaidmd5"`
}

type Imp struct {
	Id       string `json:"id"`
	TagId    string `json:"tagId"`
	SlotId   string `json:"slotId"`
	Pos      int    `json:"pos"`
	Bidfloor int    `json:"bidfloor"`
}

type RtbRequest struct {
	Id     string      `json:"id"`
	Device RtbDevice   `json:"device"`
	Imp    []Imp       `json:"imp"`
	Site   interface{} `json:"site"`
	App    interface{} `json:"app"`
}

type RtbTemplate struct {
	Aid              int      `json:"aid"`
	TemplateId       int64    `json:"template_id"`
	Title            string   `json:"title"`
	Source           string   `json:"source"`
	ImageUrl         []string `json:"image_url"`
	ClickUrl         string   `json:"click_url"`
	ClickmonitorUrls []string `json:"clickmonitor_urls"`
	ViewmonitorUrls  []string `json:"viewmonitor_urls"`
	DeeplinkUrl      string   `json:"deeplink_url"`
}

type Seatbid struct {
	Bid []Bid `json:"bid"`
}

type Bid struct {
	Id    string  `json:"id"`
	Impid string  `json:"impid"`
	Price float64 `json:"price"`
	Adm   string  `json:"adm"`
}

type RtbResponse struct {
	Id      string    `json:"id"`
	Bidid   string    `json:"bidid"`
	Seatbid []Seatbid `json:"seatbid"`
}
