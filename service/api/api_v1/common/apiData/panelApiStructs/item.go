package panelApiStructs

import (
	"sun-panel/api/api_v1/common/apiData/commonApiStructs"
	"sun-panel/models"
	"sun-panel/models/datatype"
)

type ItemInfo struct {
	models.BaseModel
	CardType        int                       `json:"cardType"` // 卡片类型：普通图标，docker图标，系统统计卡片（暂时不适配）...
	IconJson        string                    `json:"-"`        // 历史遗留问题，为适配1.4.0及以下版本暂时适配
	Icon            datatype.ItemIconIconInfo `json:"icon"`
	Title           string                    `json:"title"`
	OnlyName        string                    `json:"onlyName"`
	Url             string                    `json:"url"`
	LanUrl          string                    `json:"lanUrl"`
	Description     string                    `json:"description"`
	OpenMethod      int                       `json:"openMethod"`
	Sort            int                       `json:"sort"`
	BackgroundColor string                    `json:"backgroundColor"` // 背景色 1.5.0
	ItemIconGroupId int                       `json:"itemIconGroupId"`
	ExpandParam     map[string]any            `json:"expandParam"`
	UserId          uint                      `json:"userId"`
}

type ItemIconEditRequest struct {
	models.ItemIcon
	IconJson string
}

type ItemIconSaveSortRequest struct {
	SortItems       []commonApiStructs.SortRequestItem `json:"sortItems"`
	ItemIconGroupId uint                               `json:"itemIconGroupId"`
}

// ItemIconSortBatchGroup 跨分组批量排序中的单个分组
// 未出现在请求中的分组不会被修改；出现过的分组必须提交该组全部卡片，避免丢失卡片
type ItemIconSortBatchGroup struct {
	ItemIconGroupId uint   `json:"itemIconGroupId"`
	ItemIds         []uint `json:"itemIds"`
}

// ItemIconSaveSortBatchRequest 跨分组原子批量排序请求
type ItemIconSaveSortBatchRequest struct {
	Groups []ItemIconSortBatchGroup `json:"groups"`
}

type ItemIconGetSiteFaviconReq struct {
	Url string `json:"url"`
}

type ItemIconGetSiteFaviconResp struct {
	IconUrl string `json:"iconUrl"`
}
