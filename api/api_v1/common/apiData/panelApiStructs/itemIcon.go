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
