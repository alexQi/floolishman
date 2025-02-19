package source

import (
	"floolishman/types"
	"fmt"
	"testing"
)

var authHeader = map[string]string{
	"cookie":    "bnc-uuid=e40f2571-0b6c-4b43-abab-a7ab36234432; se_gd=Q0PGxWRkWEIURASgNCBIgZZUQDxVQBVUlACNcW0FlhcWwD1NWUNR1; se_gsd=VSwgPzNkISs0Mwk0JRw0M1M1VRJUBwtXWFtFW1BWU1haJ1NT1; OptanonAlertBoxClosed=2024-03-08T07:54:59.022Z; BNC-Location=BINANCE; source=referral; campaign=accounts.binance.com; pl-id=36843340; changeBasisTimeZone=; g_state={\"i_l\":0}; _gcl_au=1.1.1988211238.1718447414; _ga_MEG0BSW76K=GS1.1.1719472131.1.1.1719472148.0.0.0; se_sd=wsFABRx5RQLCA5bcCBQ4gZZXAUAATEXWlsAZbW0FFFWVgW1NWUcV1; profile-setting=setted; futures-layout=pro; theme=dark; userPreferredCurrency=VND_USD; BNC_FV_KEY=33b71ee3b69ba28bbfbc8116ccf6a3d923efef6c; language=zh-CN; _gid=GA1.2.689652698.1721278112; BNC_FV_KEY_T=101-ODyR7H3vIq7Vcj64x3UoVHrq91NuHUgKv4CqznEXLHUmDpFQ63VJ5FO54sAJSeYtDMLHd64oUwd39ZJ%2Fuuttdw%3D%3D-Dk4S3aJgOwGeFCh0e8t0AA%3D%3D-31; BNC_FV_KEY_EXPIRE=1721299712055; sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%22900885031%22%2C%22first_id%22%3A%2218e1d0ed8ec1f02-0953ffb814c9fd8-1d525637-1296000-18e1d0ed8ed2344%22%2C%22props%22%3A%7B%22%24latest_traffic_source_type%22%3A%22%E7%9B%B4%E6%8E%A5%E6%B5%81%E9%87%8F%22%2C%22%24latest_search_keyword%22%3A%22%E6%9C%AA%E5%8F%96%E5%88%B0%E5%80%BC_%E7%9B%B4%E6%8E%A5%E6%89%93%E5%BC%80%22%2C%22%24latest_referrer%22%3A%22%22%2C%22%24latest_utm_source%22%3A%22referral%22%2C%22%24latest_utm_campaign%22%3A%22accounts.binance.com%22%7D%2C%22identities%22%3A%22eyIkaWRlbnRpdHlfY29va2llX2lkIjoiMThlMWQwZWQ4ZWMxZjAyLTA5NTNmZmI4MTRjOWZkOC0xZDUyNTYzNy0xMjk2MDAwLTE4ZTFkMGVkOGVkMjM0NCIsIiRpZGVudGl0eV9sb2dpbl9pZCI6IjkwMDg4NTAzMSJ9%22%2C%22history_login_id%22%3A%7B%22name%22%3A%22%24identity_login_id%22%2C%22value%22%3A%22900885031%22%7D%2C%22%24device_id%22%3A%2218e1d0ed8ec1f02-0953ffb814c9fd8-1d525637-1296000-18e1d0ed8ed2344%22%7D; cr00=919A97EDCBB7A404E4C757DD32E54E05; d1og=web.900885031.9BEAD1A06C3C317A625C0B0D66BB30E4; r2o1=web.900885031.DC719519C0169C9B00C403A7BB5D1A97; f30l=web.900885031.41C3ABB486FFC03461FB0DEF32FBD773; logined=y; isAccountsLoggedIn=y; lang=zh-CN; __BNC_USER_DEVICE_ID__={\"ae553461b390e813854113dadaa65f55\":{\"date\":1721280889248,\"value\":\"1721280889203N4JYXwPycXkr5nZqmJr\"},\"d41d8cd98f00b204e9800998ecf8427e\":{\"date\":1720010122787,\"value\":\"\"}}; p20t=web.900885031.D079D3CD8A890C254E2498AB1BBBD5FB; _uetsid=74d610f044c711ef9e92ff0588eeabde; _uetvid=40cdfbe0dd2311ee8d80f30035e84d31; _gat_UA-162512367-1=1; _ga_3WP50LGEEC=GS1.1.1721280875.162.1.1721280948.58.0.0; _ga=GA1.1.1724567546.1709884496; OptanonConsent=isGpcEnabled=0&datestamp=Thu+Jul+18+2024+13%3A35%3A48+GMT%2B0800+(%E4%B8%AD%E5%9B%BD%E6%A0%87%E5%87%86%E6%97%B6%E9%97%B4)&version=202406.1.0&browserGpcFlag=0&isIABGlobal=false&hosts=&consentId=eb9fd072-a9f1-4fae-9c47-7a1de1f1da53&interactionCount=1&landingPath=NotLandingPage&groups=C0001%3A1%2CC0003%3A1%2CC0004%3A1%2CC0002%3A1&geolocation=US%3BWA&AwaitingReconsent=false&isAnonUser=1",
	"csrftoken": "51fa9fa603fa9490e3d9fb3c182ede86",
}

func TestGetUserPortfolioList(t *testing.T) {
	binanceSource := &BinanceSource{}
	binanceSource.InitHttpClient(types.ProxyOption{
		Status: true,
		Url:    "http://127.0.0.1:7890",
	})
	list, err := binanceSource.GetUserPortfolioList(authHeader)
	if err != nil {
		return
	}
	fmt.Print(list)
}
