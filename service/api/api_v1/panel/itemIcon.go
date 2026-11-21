package panel

import (
	"encoding/json"
	"net/http"
	"sun-panel/api/api_v1/common/apiData/commonApiStructs"
	"sun-panel/api/api_v1/common/apiData/panelApiStructs"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/lib/safefile"
	"sun-panel/lib/safeurl"
	"sun-panel/lib/siteFavicon"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ItemIcon struct {
}

func (a *ItemIcon) Edit(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)
	req := models.ItemIcon{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	if req.ItemIconGroupId <= 0 {
		// apiReturn.Error(c, "Group is mandatory")
		apiReturn.ErrorParamFomat(c, "Group is mandatory")
		return
	}

	req.UserId = userInfo.ID
	req.User = models.User{}

	// json转字符串
	if j, err := json.Marshal(req.Icon); err == nil {
		req.IconJson = string(j)
	}

	if req.ID != 0 {
		updateField := []string{"IconJson", "Title", "Url", "LanUrl", "Description", "OpenMethod", "ItemIconGroupId"}
		if req.Sort != 0 {
			updateField = append(updateField, "Sort")
		}
		ownedGroup := global.Db.Model(&models.ItemIconGroup{}).Select("id").Where("id = ? AND user_id = ?", req.ItemIconGroupId, userInfo.ID)
		result := global.Db.Model(&models.ItemIcon{}).Select(updateField).
			Where("id = ? AND user_id = ? AND EXISTS (?)", req.ID, userInfo.ID, ownedGroup).Updates(&req)
		if result.Error != nil {
			apiReturn.ErrorDatabase(c, result.Error.Error())
			return
		}
		if result.RowsAffected == 0 {
			apiReturn.ErrorNoAccess(c)
			return
		}
	} else {
		req.Sort = 9999
		if err := createOwnedItemIcons(userInfo.ID, []int{req.ItemIconGroupId}, &req); err != nil {
			respondItemIconCreateError(c, err)
			return
		}
	}

	apiReturn.SuccessData(c, req)
}

// 添加多个图标
func (a *ItemIcon) AddMultiple(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)
	// type Request
	req := []models.ItemIcon{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	if len(req) == 0 {
		apiReturn.SuccessData(c, req)
		return
	}
	groups := make([]int, 0, len(req))
	for i := range req {
		if req[i].ItemIconGroupId <= 0 {
			apiReturn.ErrorParamFomat(c, "Group is mandatory")
			return
		}
		groups = append(groups, req[i].ItemIconGroupId)
		req[i].ID = 0
		req[i].User = models.User{}
		req[i].UserId = userInfo.ID
		// json转字符串
		if j, err := json.Marshal(req[i].Icon); err == nil {
			req[i].IconJson = string(j)
		}
	}

	if err := createOwnedItemIcons(userInfo.ID, groups, &req); err != nil {
		respondItemIconCreateError(c, err)
		return
	}

	apiReturn.SuccessData(c, req)
}

// // 获取详情
// func (a *ItemIcon) GetInfo(c *gin.Context) {
// 	req := systemApiStructs.AiDrawGetInfoReq{}

// 	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
// 		apiReturn.ErrorParamFomat(c, err.Error())
// 		return
// 	}

// 	userInfo, _ := base.GetCurrentUserInfo(c)

// 	aiDraw := models.AiDraw{}
// 	aiDraw.ID = req.ID
// 	if err := aiDraw.GetInfo(global.Db); err != nil {
// 		if err == gorm.ErrRecordNotFound {
// 			apiReturn.Error(c, "不存在记录")
// 			return
// 		}
// 		apiReturn.ErrorDatabase(c, err.Error())
// 		return
// 	}

// 	if userInfo.ID != aiDraw.UserID {
// 		apiReturn.ErrorNoAccess(c)
// 		return
// 	}

// 	apiReturn.SuccessData(c, aiDraw)
// }

func (a *ItemIcon) GetListByGroupId(c *gin.Context) {
	req := models.ItemIcon{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	userInfo, _ := base.GetCurrentUserInfo(c)
	itemIcons := []models.ItemIcon{}

	owned := global.Db.Model(&models.ItemIconGroup{}).Select("id").Where("id = ? AND user_id = ?", req.ItemIconGroupId, userInfo.ID)
	if err := global.Db.Order("sort ,created_at").Find(&itemIcons, "item_icon_group_id IN (?) AND user_id=?", owned, userInfo.ID).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}

	visitMode, _ := c.Get(base.GIN_GET_VISIT_MODE)
	for k, v := range itemIcons {
		json.Unmarshal([]byte(v.IconJson), &itemIcons[k].Icon)
		if visitMode == base.VISIT_MODE_PUBLIC {
			itemIcons[k].LanUrl = ""
		}
	}

	apiReturn.SuccessListData(c, itemIcons, 0)
}

func (a *ItemIcon) Deletes(c *gin.Context) {
	req := commonApiStructs.RequestDeleteIds[uint]{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	userInfo, _ := base.GetCurrentUserInfo(c)
	if err := global.Db.Delete(&models.ItemIcon{}, "id in ? AND user_id=?", req.Ids, userInfo.ID).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}

	apiReturn.Success(c)
}

// 保存排序
func (a *ItemIcon) SaveSort(c *gin.Context) {
	req := panelApiStructs.ItemIconSaveSortRequest{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	userInfo, _ := base.GetCurrentUserInfo(c)

	transactionErr := global.Db.Transaction(func(tx *gorm.DB) error {
		// 在事务中执行一些 db 操作（从这里开始，您应该使用 'tx' 而不是 'db'）
		for _, v := range req.SortItems {
			if err := tx.Model(&models.ItemIcon{}).Where("user_id=? AND id=? AND item_icon_group_id=?", userInfo.ID, v.Id, req.ItemIconGroupId).Update("sort", v.Sort).Error; err != nil {
				// 返回任何错误都会回滚事务
				return err
			}
		}

		// 返回 nil 提交事务
		return nil
	})

	if transactionErr != nil {
		apiReturn.ErrorDatabase(c, transactionErr.Error())
		return
	}

	apiReturn.Success(c)
}

// 支持获取并直接下载对方网站图标到服务器
func (a *ItemIcon) GetSiteFavicon(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)
	req := panelApiStructs.ItemIconGetSiteFaviconReq{}

	// 限制请求体大小
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	pageURL, err := safeurl.Parse(req.Url)
	if err != nil {
		apiReturn.Error(c, "acquisition failed: url is not allowed")
		return
	}
	configUpload := global.Config.GetValueString("base", "source_path")
	savedPath, err := siteFavicon.DownloadSiteFavicon(c.Request.Context(), pageURL.String(), configUpload)
	if err != nil {
		apiReturn.Error(c, "acquisition failed: could not load site icon")
		return
	}
	mFile := models.File{}
	if _, err := mFile.AddFile(userInfo.ID, pageURL.Host, safefile.ExtOf(savedPath), savedPath); err != nil {
		if cleanupErr := safefile.RemoveUpload(configUpload, savedPath); cleanupErr != nil {
			global.Logger.Warn("favicon cleanup failed: ", cleanupErr)
		}
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	apiReturn.SuccessData(c, panelApiStructs.ItemIconGetSiteFaviconResp{IconUrl: safefile.UploadURLPath(savedPath, configUpload)})
}

// itemIconBelongsToUser 判断图标是否属于指定用户。
func createOwnedItemIcons(userID uint, groups []int, input any) error {
	wanted := make(map[int]struct{}, len(groups))
	for _, id := range groups {
		wanted[id] = struct{}{}
	}
	return global.Db.Transaction(func(tx *gorm.DB) error {
		var owned []int
		if err := tx.Model(&models.ItemIconGroup{}).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ? AND user_id = ?", groups, userID).Pluck("id", &owned).Error; err != nil {
			return err
		}
		if len(owned) != len(wanted) {
			return gorm.ErrRecordNotFound
		}
		return tx.Omit(clause.Associations).Create(input).Error
	})
}

func respondItemIconCreateError(c *gin.Context, err error) {
	if err == gorm.ErrRecordNotFound {
		apiReturn.ErrorParamFomat(c, "Group not found")
		return
	}
	apiReturn.ErrorDatabase(c, err.Error())
}
