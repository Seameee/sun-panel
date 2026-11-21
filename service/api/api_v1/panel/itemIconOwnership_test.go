package panel

import (
	"fmt"
	"testing"

	"sun-panel/global"
	"sun-panel/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateItemIconsInOwnedGroup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.ItemIconGroup{}, &models.ItemIcon{}); err != nil {
		t.Fatal(err)
	}
	old := global.Db
	global.Db = db
	t.Cleanup(func() { global.Db = old })

	group := models.ItemIconGroup{UserId: 7, Title: "Demo"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	icons := []models.ItemIcon{
		{UserId: 7, ItemIconGroupId: int(group.ID), Title: "First"},
		{UserId: 7, ItemIconGroupId: int(group.ID), Title: "Second"},
	}
	if err := createOwnedItemIcons(7, []int{int(group.ID), int(group.ID)}, &icons); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.ItemIcon{}).Where("user_id = ? AND item_icon_group_id = ?", 7, group.ID).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("owned icons = %d, %v", count, err)
	}
}
