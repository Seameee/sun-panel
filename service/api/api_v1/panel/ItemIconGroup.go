package panel

import (
	"errors"
	"sun-panel/api/api_v1/common/apiData/commonApiStructs"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ItemIconGroup struct {
}

func (a *ItemIconGroup) Edit(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)
	req := models.ItemIconGroup{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	req.UserId = userInfo.ID
	req.User = models.User{}

	if req.ID != 0 {
		updateFields := []string{"Icon", "Title", "Description"}
		if req.Sort != 0 {
			updateFields = append(updateFields, "Sort")
		}
		result := global.Db.Model(&models.ItemIconGroup{}).Select(updateFields).
			Where("id = ? AND user_id = ?", req.ID, userInfo.ID).Updates(&req)
		if result.Error != nil {
			apiReturn.ErrorDatabase(c, result.Error.Error())
			return
		}
		if result.RowsAffected == 0 {
			apiReturn.ErrorNoAccess(c)
			return
		}
	} else if err := global.Db.Omit(clause.Associations).Create(&req).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}

	apiReturn.SuccessData(c, req)
}

func (a *ItemIconGroup) GetList(c *gin.Context) {

	userInfo, _ := base.GetCurrentUserInfo(c)
	groups := []models.ItemIconGroup{}

	visitMode, _ := c.Get(base.GIN_GET_VISIT_MODE)
	err := global.Db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Order("sort ,created_at").Where("user_id=?", userInfo.ID).Find(&groups).Error; err != nil {
			return err
		}

		// 只为登录用户初始化默认分组。
		if len(groups) == 0 && visitMode != base.VISIT_MODE_PUBLIC {
			defaultGroup := models.ItemIconGroup{
				Title:  "APP",
				UserId: userInfo.ID,
				Icon:   "material-symbols:ad-group-outline",
			}
			if err := tx.Create(&defaultGroup).Error; err != nil {
				return err
			}

			// 并将当前账号下所有无分组的图标更新到当前组
			if err := tx.Model(&models.ItemIcon{}).Where("user_id=?", userInfo.ID).Update("item_icon_group_id", defaultGroup.ID).Error; err != nil {
				return err
			}

			groups = append(groups, defaultGroup)
		}

		// 返回 nil 提交事务
		return nil
	})

	if err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	} else {
		apiReturn.SuccessListData(c, groups, 0)
	}
}

func (a *ItemIconGroup) Deletes(c *gin.Context) {
	req := commonApiStructs.RequestDeleteIds[uint]{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	userInfo, _ := base.GetCurrentUserInfo(c)

	errLastGroup := errors.New("at least one group must be retained")
	txErr := global.Db.Transaction(func(tx *gorm.DB) error {
		var groupIDs []uint
		if err := tx.Model(&models.ItemIconGroup{}).Where("user_id = ? AND id IN ?", userInfo.ID, req.Ids).Pluck("id", &groupIDs).Error; err != nil {
			return err
		}
		if len(groupIDs) == 0 {
			return nil
		}
		var count int64
		if err := tx.Model(&models.ItemIconGroup{}).Where("user_id = ?", userInfo.ID).Count(&count).Error; err != nil {
			return err
		}
		if int64(len(groupIDs)) >= count {
			return errLastGroup
		}
		if err := tx.Delete(&models.ItemIconGroup{}, "id IN ? AND user_id = ?", groupIDs, userInfo.ID).Error; err != nil {
			return err
		}
		return (&models.ItemIcon{}).DeleteByItemIconGroupIds(tx, userInfo.ID, groupIDs)
	})

	if errors.Is(txErr, errLastGroup) {
		apiReturn.ErrorCode(c, 1201, "At least one must be retained", nil)
		return
	}
	if txErr != nil {
		apiReturn.ErrorDatabase(c, txErr.Error())
		return
	}

	apiReturn.Success(c)
}

// 保存排序
func (a *ItemIconGroup) SaveSort(c *gin.Context) {
	req := commonApiStructs.SortRequest{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	userInfo, _ := base.GetCurrentUserInfo(c)

	transactionErr := global.Db.Transaction(func(tx *gorm.DB) error {
		// 在事务中执行一些 db 操作（从这里开始，您应该使用 'tx' 而不是 'db'）
		for _, v := range req.SortItems {
			if err := tx.Model(&models.ItemIconGroup{}).Where("user_id=? AND id=?", userInfo.ID, v.Id).Update("sort", v.Sort).Error; err != nil {
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
