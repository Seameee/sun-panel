package biz

import (
	"errors"
	"fmt"
	"testing"

	"sun-panel/global"
	"sun-panel/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	sortTestUserA uint = 1
	sortTestUserB uint = 2
)

func setupSortTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&models.ItemIcon{}, &models.ItemIconGroup{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	oldDb := global.Db
	global.Db = db
	t.Cleanup(func() { global.Db = oldDb })
	return db
}

func seedSortGroup(t *testing.T, db *gorm.DB, userId uint, title string, sort int) models.ItemIconGroup {
	t.Helper()
	group := models.ItemIconGroup{Title: title, Sort: sort, UserId: userId}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group %s: %v", title, err)
	}
	return group
}

func seedSortItem(t *testing.T, db *gorm.DB, userId uint, groupId uint, title string, sort int) models.ItemIcon {
	t.Helper()
	item := models.ItemIcon{Title: title, Sort: sort, UserId: userId, ItemIconGroupId: int(groupId)}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create item %s: %v", title, err)
	}
	return item
}

func groupOrderIds(t *testing.T, db *gorm.DB, groupId uint) []uint {
	t.Helper()
	items := []models.ItemIcon{}
	if err := db.Where("item_icon_group_id = ?", groupId).Order("sort, created_at").Find(&items).Error; err != nil {
		t.Fatalf("load group order: %v", err)
	}
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func itemGroupIdOf(t *testing.T, db *gorm.DB, itemId uint) uint {
	t.Helper()
	item := models.ItemIcon{}
	if err := db.First(&item, itemId).Error; err != nil {
		t.Fatalf("load item %d: %v", itemId, err)
	}
	return uint(item.ItemIconGroupId)
}

func assertIds(t *testing.T, got, want []uint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ids length = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}

func TestSaveSortBatchSameGroupReorders(t *testing.T) {
	db := setupSortTestDB(t)
	groupA := seedSortGroup(t, db, sortTestUserA, "A", 1)
	item1 := seedSortItem(t, db, sortTestUserA, groupA.ID, "1", 1)
	item2 := seedSortItem(t, db, sortTestUserA, groupA.ID, "2", 2)
	item3 := seedSortItem(t, db, sortTestUserA, groupA.ID, "3", 3)

	err := Item.SaveSortBatch(sortTestUserA, []ItemSortGroup{
		{ItemIconGroupId: groupA.ID, ItemIds: []uint{item3.ID, item1.ID, item2.ID}},
	})
	if err != nil {
		t.Fatalf("SaveSortBatch: %v", err)
	}

	assertIds(t, groupOrderIds(t, db, groupA.ID), []uint{item3.ID, item1.ID, item2.ID})
}

func TestSaveSortBatchCrossGroupMove(t *testing.T) {
	db := setupSortTestDB(t)
	groupA := seedSortGroup(t, db, sortTestUserA, "A", 1)
	groupB := seedSortGroup(t, db, sortTestUserA, "B", 2)
	a1 := seedSortItem(t, db, sortTestUserA, groupA.ID, "a1", 1)
	a2 := seedSortItem(t, db, sortTestUserA, groupA.ID, "a2", 2)
	b1 := seedSortItem(t, db, sortTestUserA, groupB.ID, "b1", 1)

	err := Item.SaveSortBatch(sortTestUserA, []ItemSortGroup{
		{ItemIconGroupId: groupA.ID, ItemIds: []uint{a2.ID}},
		{ItemIconGroupId: groupB.ID, ItemIds: []uint{b1.ID, a1.ID}},
	})
	if err != nil {
		t.Fatalf("SaveSortBatch: %v", err)
	}

	assertIds(t, groupOrderIds(t, db, groupA.ID), []uint{a2.ID})
	assertIds(t, groupOrderIds(t, db, groupB.ID), []uint{b1.ID, a1.ID})
	if got := itemGroupIdOf(t, db, a1.ID); got != groupB.ID {
		t.Fatalf("moved item group = %d, want %d", got, groupB.ID)
	}
	if got := itemGroupIdOf(t, db, a2.ID); got != groupA.ID {
		t.Fatalf("kept item group = %d, want %d", got, groupA.ID)
	}

	var total int64
	if err := db.Model(&models.ItemIcon{}).Where("user_id = ?", sortTestUserA).Count(&total).Error; err != nil {
		t.Fatalf("count items: %v", err)
	}
	if total != 3 {
		t.Fatalf("total items = %d, want 3 (no card lost)", total)
	}
}

func TestSaveSortBatchAllowsEmptyGroup(t *testing.T) {
	db := setupSortTestDB(t)
	groupA := seedSortGroup(t, db, sortTestUserA, "A", 1)
	groupC := seedSortGroup(t, db, sortTestUserA, "C", 3)
	a1 := seedSortItem(t, db, sortTestUserA, groupA.ID, "a1", 1)

	err := Item.SaveSortBatch(sortTestUserA, []ItemSortGroup{
		{ItemIconGroupId: groupA.ID, ItemIds: []uint{a1.ID}},
		{ItemIconGroupId: groupC.ID, ItemIds: []uint{}},
	})
	if err != nil {
		t.Fatalf("SaveSortBatch: %v", err)
	}
	assertIds(t, groupOrderIds(t, db, groupA.ID), []uint{a1.ID})
	assertIds(t, groupOrderIds(t, db, groupC.ID), []uint{})
}

func TestSaveSortBatchRejectsMissingItem(t *testing.T) {
	db := setupSortTestDB(t)
	groupA := seedSortGroup(t, db, sortTestUserA, "A", 1)
	a1 := seedSortItem(t, db, sortTestUserA, groupA.ID, "a1", 1)
	a2 := seedSortItem(t, db, sortTestUserA, groupA.ID, "a2", 2)

	err := Item.SaveSortBatch(sortTestUserA, []ItemSortGroup{
		{ItemIconGroupId: groupA.ID, ItemIds: []uint{a1.ID}},
	})
	if !errors.Is(err, ErrItemSortDataStale) {
		t.Fatalf("err = %v, want ErrItemSortDataStale", err)
	}
	// 失败时不得修改数据
	assertIds(t, groupOrderIds(t, db, groupA.ID), []uint{a1.ID, a2.ID})
}

func TestSaveSortBatchRejectsForeignGroup(t *testing.T) {
	db := setupSortTestDB(t)
	groupB := seedSortGroup(t, db, sortTestUserB, "B", 1)

	err := Item.SaveSortBatch(sortTestUserA, []ItemSortGroup{
		{ItemIconGroupId: groupB.ID, ItemIds: []uint{}},
	})
	if !errors.Is(err, ErrItemSortGroupNotOwned) {
		t.Fatalf("err = %v, want ErrItemSortGroupNotOwned", err)
	}
}

func TestSaveSortBatchRejectsForeignItem(t *testing.T) {
	db := setupSortTestDB(t)
	groupA := seedSortGroup(t, db, sortTestUserA, "A", 1)
	a1 := seedSortItem(t, db, sortTestUserA, groupA.ID, "a1", 1)
	groupB := seedSortGroup(t, db, sortTestUserB, "B", 1)
	b1 := seedSortItem(t, db, sortTestUserB, groupB.ID, "b1", 1)

	err := Item.SaveSortBatch(sortTestUserA, []ItemSortGroup{
		{ItemIconGroupId: groupA.ID, ItemIds: []uint{a1.ID, b1.ID}},
	})
	if !errors.Is(err, ErrItemSortItemNotOwned) {
		t.Fatalf("err = %v, want ErrItemSortItemNotOwned", err)
	}
	if got := itemGroupIdOf(t, db, b1.ID); got != groupB.ID {
		t.Fatalf("foreign item group = %d, want %d", got, groupB.ID)
	}
}

func TestSaveSortBatchRejectsItemOfUnlistedGroup(t *testing.T) {
	db := setupSortTestDB(t)
	groupA := seedSortGroup(t, db, sortTestUserA, "A", 1)
	groupB := seedSortGroup(t, db, sortTestUserA, "B", 2)
	a1 := seedSortItem(t, db, sortTestUserA, groupA.ID, "a1", 1)
	b1 := seedSortItem(t, db, sortTestUserA, groupB.ID, "b1", 1)

	err := Item.SaveSortBatch(sortTestUserA, []ItemSortGroup{
		{ItemIconGroupId: groupA.ID, ItemIds: []uint{a1.ID, b1.ID}},
	})
	if !errors.Is(err, ErrItemSortItemNotOwned) {
		t.Fatalf("err = %v, want ErrItemSortItemNotOwned", err)
	}
	if got := itemGroupIdOf(t, db, b1.ID); got != groupB.ID {
		t.Fatalf("unlisted group item moved: group = %d, want %d", got, groupB.ID)
	}
}

func TestSaveSortBatchRejectsDuplicateItem(t *testing.T) {
	db := setupSortTestDB(t)
	groupA := seedSortGroup(t, db, sortTestUserA, "A", 1)
	groupB := seedSortGroup(t, db, sortTestUserA, "B", 2)
	a1 := seedSortItem(t, db, sortTestUserA, groupA.ID, "a1", 1)

	err := Item.SaveSortBatch(sortTestUserA, []ItemSortGroup{
		{ItemIconGroupId: groupA.ID, ItemIds: []uint{a1.ID}},
		{ItemIconGroupId: groupB.ID, ItemIds: []uint{a1.ID}},
	})
	if !errors.Is(err, ErrItemSortDuplicateItem) {
		t.Fatalf("err = %v, want ErrItemSortDuplicateItem", err)
	}
}

func TestSaveSortBatchRejectsDuplicateGroup(t *testing.T) {
	db := setupSortTestDB(t)
	groupA := seedSortGroup(t, db, sortTestUserA, "A", 1)
	a1 := seedSortItem(t, db, sortTestUserA, groupA.ID, "a1", 1)

	err := Item.SaveSortBatch(sortTestUserA, []ItemSortGroup{
		{ItemIconGroupId: groupA.ID, ItemIds: []uint{a1.ID}},
		{ItemIconGroupId: groupA.ID, ItemIds: []uint{a1.ID}},
	})
	if !errors.Is(err, ErrItemSortDuplicateGroup) {
		t.Fatalf("err = %v, want ErrItemSortDuplicateGroup", err)
	}
}

func TestSaveSortBatchRejectsEmptyPayload(t *testing.T) {
	setupSortTestDB(t)

	if err := Item.SaveSortBatch(sortTestUserA, nil); !errors.Is(err, ErrItemSortEmptyGroups) {
		t.Fatalf("err = %v, want ErrItemSortEmptyGroups", err)
	}
	if err := Item.SaveSortBatch(sortTestUserA, []ItemSortGroup{{ItemIconGroupId: 0}}); !errors.Is(err, ErrItemSortInvalidGroup) {
		t.Fatalf("err = %v, want ErrItemSortInvalidGroup", err)
	}
}
