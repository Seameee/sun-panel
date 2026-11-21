package panel

// 跨分组批量排序（独立文件，避免与 itemIcon.go 的其它改动冲突）
//
// 与既有的 SaveSort 不同，本接口一次提交多个分组，并允许卡片在分组之间移动。
// 设计要点：
//  1. 只修改请求中出现过的分组，未出现的分组保持原样；
//  2. 请求中出现过的分组，其下已存在的卡片必须全部提交，否则视为前端数据过期；
//  3. 所有写入在同一个事务内完成，任一步失败整体回滚；
//  4. 分组与卡片都必须属于当前登录用户（防止越权修改他人数据）。

import (
	"errors"

	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 跨分组批量排序的校验错误
var (
	ErrItemSortEmptyGroups    = errors.New("sort groups is empty")
	ErrItemSortInvalidGroup   = errors.New("invalid sort group")
	ErrItemSortDuplicateGroup = errors.New("duplicate sort group")
	ErrItemSortGroupNotOwned  = errors.New("sort group does not belong to current user")
	ErrItemSortInvalidItem    = errors.New("invalid sort item")
	ErrItemSortDuplicateItem  = errors.New("duplicate sort item")
	ErrItemSortItemNotOwned   = errors.New("sort item does not belong to current user or these groups")
	ErrItemSortDataStale      = errors.New("sort data is out of date, some cards are missing, please refresh")
)

// ItemIconSortBatchGroup 跨分组批量排序中的单个分组
// ItemIds 为该组按最终顺序排列的卡片ID；空数组表示该分组排空
type ItemIconSortBatchGroup struct {
	ItemIconGroupId uint   `json:"itemIconGroupId"`
	ItemIds         []uint `json:"itemIds"`
}

// ItemIconSaveSortBatchRequest 跨分组原子批量排序请求
type ItemIconSaveSortBatchRequest struct {
	Groups []ItemIconSortBatchGroup `json:"groups"`
}

// SaveSortBatch 跨分组原子批量保存排序（含卡片跨组移动）
func (a *ItemIcon) SaveSortBatch(c *gin.Context) {
	req := ItemIconSaveSortBatchRequest{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	userInfo, ok := base.GetCurrentUserInfo(c)
	if !ok || userInfo.ID == 0 {
		apiReturn.ErrorNoAccess(c)
		return
	}

	if err := SaveItemIconSortBatch(userInfo.ID, req.Groups); err != nil {
		switch {
		case errors.Is(err, ErrItemSortGroupNotOwned), errors.Is(err, ErrItemSortItemNotOwned):
			// 越权访问他人分组/卡片，不返回细节
			apiReturn.ErrorNoAccess(c)
		case errors.Is(err, ErrItemSortEmptyGroups),
			errors.Is(err, ErrItemSortInvalidGroup),
			errors.Is(err, ErrItemSortDuplicateGroup),
			errors.Is(err, ErrItemSortInvalidItem),
			errors.Is(err, ErrItemSortDuplicateItem),
			errors.Is(err, ErrItemSortDataStale):
			apiReturn.ErrorParamFomat(c, err.Error())
		default:
			apiReturn.ErrorDatabase(c, err.Error())
		}
		return
	}

	apiReturn.Success(c)
}

// SaveItemIconSortBatch 使用全局数据库连接保存排序
func SaveItemIconSortBatch(userId uint, groups []ItemIconSortBatchGroup) error {
	return SaveItemIconSortBatchWithDB(global.Db, userId, groups)
}

// SaveItemIconSortBatchWithDB 保存排序（可注入数据库，便于测试）
//
// 规则：
//  1. 请求中的分组必须属于当前用户，且不能重复；
//  2. 卡片不能重复提交，且必须属于当前用户、属于请求中列出的分组；
//  3. 请求中列出的分组，其下已有卡片必须全部提交，否则视为数据过期（防止丢卡片）；
//  4. 组内 sort 按提交顺序重新写为 1..n，跨组卡片同步更新 item_icon_group_id；
//  5. 全部更新在同一个事务内完成，任一步失败整体回滚。
func SaveItemIconSortBatchWithDB(db *gorm.DB, userId uint, groups []ItemIconSortBatchGroup) error {
	if len(groups) == 0 {
		return ErrItemSortEmptyGroups
	}

	groupIds := make([]uint, 0, len(groups))
	seenGroup := make(map[uint]bool, len(groups))
	for _, g := range groups {
		if g.ItemIconGroupId == 0 {
			return ErrItemSortInvalidGroup
		}
		if seenGroup[g.ItemIconGroupId] {
			return ErrItemSortDuplicateGroup
		}
		seenGroup[g.ItemIconGroupId] = true
		groupIds = append(groupIds, g.ItemIconGroupId)
	}

	seenItem := make(map[uint]bool, len(groupIds))
	for _, g := range groups {
		for _, itemId := range g.ItemIds {
			if itemId == 0 {
				return ErrItemSortInvalidItem
			}
			if seenItem[itemId] {
				return ErrItemSortDuplicateItem
			}
			seenItem[itemId] = true
		}
	}

	type sortAssignment struct {
		id      uint
		groupId uint
		sort    int
	}
	assignments := make([]sortAssignment, 0, len(seenItem))
	for _, g := range groups {
		for index, itemId := range g.ItemIds {
			assignments = append(assignments, sortAssignment{id: itemId, groupId: g.ItemIconGroupId, sort: index + 1})
		}
	}

	return db.Transaction(func(tx *gorm.DB) error {
		ownedGroups := []models.ItemIconGroup{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id").
			Where("user_id = ? AND id IN ?", userId, groupIds).Find(&ownedGroups).Error; err != nil {
			return err
		}
		if len(ownedGroups) != len(groupIds) {
			return ErrItemSortGroupNotOwned
		}

		existingItems := []models.ItemIcon{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id").Select("id").
			Where("user_id = ? AND item_icon_group_id IN ?", userId, groupIds).Find(&existingItems).Error; err != nil {
			return err
		}

		existingIds := make(map[uint]bool, len(existingItems))
		for _, item := range existingItems {
			existingIds[item.ID] = true
			// 已存在的卡片必须全部提交，否则说明前端数据已过期
			if !seenItem[item.ID] {
				return ErrItemSortDataStale
			}
		}
		for itemId := range seenItem {
			// 卡片不属于当前用户，或属于未列出的分组
			if !existingIds[itemId] {
				return ErrItemSortItemNotOwned
			}
		}

		for _, assignment := range assignments {
			if err := tx.Model(&models.ItemIcon{}).
				Where("id = ? AND user_id = ?", assignment.id, userId).
				Updates(map[string]any{
					"item_icon_group_id": int(assignment.groupId),
					"sort":               assignment.sort,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
