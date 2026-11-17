package panelApiStructs

import (
	"sun-panel/api/api_v1/common/apiData/commonApiStructs"
	"sun-panel/models"
)

type ItemIconEditRequest struct {
	models.ItemIcon
	IconJson string
}

type ItemIconSaveSortRequest struct {
	SortItems       []commonApiStructs.SortRequestItem `json:"sortItems"`
	ItemIconGroupId uint                               `json:"itemIconGroupId"`
}

type ItemIconGetSiteFaviconReq struct {
	Url string `json:"url"`
}

type ItemIconGetSiteFaviconResp struct {
	IconUrl   string `json:"iconUrl"`
	PageTitle string `json:"pageTitle"`
}

// 获取站点图标候选项（供用户挑选）
type ItemIconGetSiteFaviconOptionsReq struct {
	Url string `json:"url"`
}

type ItemIconGetSiteFaviconOption struct {
	SourceUrl string `json:"sourceUrl"` // 图标原始地址
	Preview   string `json:"preview"`   // base64 缩略图
	Width     int    `json:"width"`     // 原始宽度，SVG 为 0
	Height    int    `json:"height"`    // 原始高度，SVG 为 0
}

type ItemIconGetSiteFaviconOptionsResp struct {
	PageTitle string                         `json:"pageTitle"`
	Icons     []ItemIconGetSiteFaviconOption `json:"icons"`
}

// 保存用户选中的站点图标
type ItemIconSaveSiteFaviconReq struct {
	Url       string `json:"url"`
	SourceUrl string `json:"sourceUrl"` // 为空时自动选择优先级最高的图标
}

type ItemIconSaveSiteFaviconResp struct {
	IconUrl   string `json:"iconUrl"`
	PageTitle string `json:"pageTitle"`
}

// 内网可达性探测：只接受书签 ID，不接受任意 URL
type ItemIconProbeNetworkReq struct {
	Ids []uint `json:"ids"`
}

type ItemIconProbeNetworkResult struct {
	Id    uint `json:"id"`
	LanOk bool `json:"lanOk"` // 内网地址是否可直连
	LanMs int  `json:"lanMs"` // 内网拨号耗时（毫秒），失败为 0
}

type ItemIconProbeNetworkResp struct {
	ClientIsLan bool                         `json:"clientIsLan"` // 本次请求来源是否为私网地址
	Throttled   bool                         `json:"throttled"`   // 是否被限流（前端沿用上次结果）
	Results     []ItemIconProbeNetworkResult `json:"results"`
}
