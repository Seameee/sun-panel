package panel

// 跨分组批量排序的接口层与核心层测试
//
// 覆盖：
//  1. 跨组移动、排空分组、未提交分组不受影响；
//  2. 越权（他人分组、未列出分组的卡片）被拒绝且数据不变；
//  3. 重复分组/重复卡片/非法ID/过期数据被拒绝；
//  4. 失败时整体回滚，不产生半写状态。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sun-panel/global"
	"sun-panel/lib/language"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	sortBatchTestUserA uint = 101
	sortBatchTestUserB uint = 102
)

func setupSortBatchTestDB(t *testing.T) *gorm.DB {
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
	// 内存库需要单连接，否则不同连接看不到同一份数据
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&models.ItemIcon{}, &models.ItemIconGroup{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func setupSortBatchRouter(t *testing.T, db *gorm.DB, userId uint, withUser bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// apiReturn 的错误文案来自 global.Lang，生产环境在启动时加载，测试里手动补上
	if global.Lang == nil {
		oldLang := global.Lang
		global.Lang = language.NewLang("../../../assets/lang/zh-cn.ini")
		t.Cleanup(func() { global.Lang = oldLang })
	}

	oldDb := global.Db
	global.Db = db
	t.Cleanup(func() { global.Db = oldDb })

	itemIcon := ItemIcon{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if withUser {
			c.Set("userInfo", models.User{BaseModel: models.BaseModel{ID: userId}})
		}
		c.Next()
	})
	router.POST("/panel/itemIcon/saveSortBatch", itemIcon.SaveSortBatch)
	return router
}

func sortBatchGroup(t *testing.T, db *gorm.DB, userId uint, title string) models.ItemIconGroup {
	t.Helper()
	group := models.ItemIconGroup{Title: title, UserId: userId}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	return group
}

func sortBatchItem(t *testing.T, db *gorm.DB, userId uint, groupId uint, title string, sort int) models.ItemIcon {
	t.Helper()
	item := models.ItemIcon{Title: title, Sort: sort, UserId: userId, ItemIconGroupId: int(groupId)}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	return item
}

type sortBatchResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func doSortBatchPost(t *testing.T, router *gin.Engine, body any) sortBatchResp {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/panel/itemIcon/saveSortBatch", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := sortBatchResp{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", w.Body.String(), err)
	}
	return resp
}

// groupItems 按 sort 返回某分组下的卡片ID列表
func groupItems(t *testing.T, db *gorm.DB, groupId uint) []uint {
	t.Helper()
	items := []models.ItemIcon{}
	if err := db.Where("item_icon_group_id = ?", groupId).Order("sort, id").Find(&items).Error; err != nil {
		t.Fatalf("load group items: %v", err)
	}
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func assertIds(t *testing.T, label string, got []uint, want ...uint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}
}

// 跨组移动：A 组保留一个、另一个移到 B 组，B 组原卡片跟随排序
func TestSaveSortBatchCrossGroupMove(t *testing.T) {
	db := setupSortBatchTestDB(t)
	router := setupSortBatchRouter(t, db, sortBatchTestUserA, true)

	groupA := sortBatchGroup(t, db, sortBatchTestUserA, "A")
	groupB := sortBatchGroup(t, db, sortBatchTestUserA, "B")
	a1 := sortBatchItem(t, db, sortBatchTestUserA, groupA.ID, "a1", 1)
	a2 := sortBatchItem(t, db, sortBatchTestUserA, groupA.ID, "a2", 2)
	b1 := sortBatchItem(t, db, sortBatchTestUserA, groupB.ID, "b1", 1)

	resp := doSortBatchPost(t, router, map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": groupA.ID, "itemIds": []uint{a2.ID}},
			{"itemIconGroupId": groupB.ID, "itemIds": []uint{a1.ID, b1.ID}},
		},
	})
	if resp.Code != 0 {
		t.Fatalf("code = %d, msg = %s", resp.Code, resp.Msg)
	}

	assertIds(t, "groupA", groupItems(t, db, groupA.ID), a2.ID)
	assertIds(t, "groupB", groupItems(t, db, groupB.ID), a1.ID, b1.ID)

	// 组内 sort 必须重排为 1..n
	moved := models.ItemIcon{}
	if err := db.First(&moved, a1.ID).Error; err != nil {
		t.Fatalf("reload moved item: %v", err)
	}
	if moved.Sort != 1 || moved.ItemIconGroupId != int(groupB.ID) {
		t.Fatalf("moved item sort=%d group=%d, want sort=1 group=%d", moved.Sort, moved.ItemIconGroupId, groupB.ID)
	}
}

// 排空分组：提交空 itemIds 是合法的，只清空该组
func TestSaveSortBatchEmptiesGroup(t *testing.T) {
	db := setupSortBatchTestDB(t)
	router := setupSortBatchRouter(t, db, sortBatchTestUserA, true)

	groupA := sortBatchGroup(t, db, sortBatchTestUserA, "A")
	groupB := sortBatchGroup(t, db, sortBatchTestUserA, "B")
	a1 := sortBatchItem(t, db, sortBatchTestUserA, groupA.ID, "a1", 1)
	b1 := sortBatchItem(t, db, sortBatchTestUserA, groupB.ID, "b1", 1)

	resp := doSortBatchPost(t, router, map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": groupA.ID, "itemIds": []uint{}},
			{"itemIconGroupId": groupB.ID, "itemIds": []uint{b1.ID, a1.ID}},
		},
	})
	if resp.Code != 0 {
		t.Fatalf("code = %d, msg = %s", resp.Code, resp.Msg)
	}
	assertIds(t, "groupA", groupItems(t, db, groupA.ID))
	assertIds(t, "groupB", groupItems(t, db, groupB.ID), b1.ID, a1.ID)
}

// 未出现在请求中的分组不受影响
func TestSaveSortBatchLeavesUnlistedGroupsUntouched(t *testing.T) {
	db := setupSortBatchTestDB(t)
	router := setupSortBatchRouter(t, db, sortBatchTestUserA, true)

	groupA := sortBatchGroup(t, db, sortBatchTestUserA, "A")
	groupC := sortBatchGroup(t, db, sortBatchTestUserA, "C")
	a1 := sortBatchItem(t, db, sortBatchTestUserA, groupA.ID, "a1", 1)
	c1 := sortBatchItem(t, db, sortBatchTestUserA, groupC.ID, "c1", 1)

	resp := doSortBatchPost(t, router, map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": groupA.ID, "itemIds": []uint{a1.ID}},
		},
	})
	if resp.Code != 0 {
		t.Fatalf("code = %d, msg = %s", resp.Code, resp.Msg)
	}
	assertIds(t, "groupC", groupItems(t, db, groupC.ID), c1.ID)
}

// 他人分组：拒绝并保持数据不变
func TestSaveSortBatchRejectsForeignGroup(t *testing.T) {
	db := setupSortBatchTestDB(t)
	router := setupSortBatchRouter(t, db, sortBatchTestUserA, true)

	foreignGroup := sortBatchGroup(t, db, sortBatchTestUserB, "B")
	foreignItem := sortBatchItem(t, db, sortBatchTestUserB, foreignGroup.ID, "b1", 1)

	resp := doSortBatchPost(t, router, map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": foreignGroup.ID, "itemIds": []uint{foreignItem.ID}},
		},
	})
	if resp.Code != 1005 {
		t.Fatalf("code = %d, want 1005, msg = %s", resp.Code, resp.Msg)
	}
	assertIds(t, "foreignGroup", groupItems(t, db, foreignGroup.ID), foreignItem.ID)
}

// 提交未列出分组下的卡片：拒绝
func TestSaveSortBatchRejectsItemFromUnlistedGroup(t *testing.T) {
	db := setupSortBatchTestDB(t)
	router := setupSortBatchRouter(t, db, sortBatchTestUserA, true)

	groupA := sortBatchGroup(t, db, sortBatchTestUserA, "A")
	groupB := sortBatchGroup(t, db, sortBatchTestUserA, "B")
	b1 := sortBatchItem(t, db, sortBatchTestUserA, groupB.ID, "b1", 1)

	resp := doSortBatchPost(t, router, map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": groupA.ID, "itemIds": []uint{b1.ID}},
		},
	})
	if resp.Code != 1005 {
		t.Fatalf("code = %d, want 1005, msg = %s", resp.Code, resp.Msg)
	}
	assertIds(t, "groupB", groupItems(t, db, groupB.ID), b1.ID)
}

// 过期数据：分组下有卡片未提交，整体回滚
func TestSaveSortBatchRejectsStaleDataAndRollsBack(t *testing.T) {
	db := setupSortBatchTestDB(t)
	router := setupSortBatchRouter(t, db, sortBatchTestUserA, true)

	groupA := sortBatchGroup(t, db, sortBatchTestUserA, "A")
	groupB := sortBatchGroup(t, db, sortBatchTestUserA, "B")
	a1 := sortBatchItem(t, db, sortBatchTestUserA, groupA.ID, "a1", 1)
	a2 := sortBatchItem(t, db, sortBatchTestUserA, groupA.ID, "a2", 2)
	b1 := sortBatchItem(t, db, sortBatchTestUserA, groupB.ID, "b1", 1)

	// B 组合法，A 组漏提交 a2 → 整体失败，B 组也不能被写入
	resp := doSortBatchPost(t, router, map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": groupA.ID, "itemIds": []uint{a1.ID}},
			{"itemIconGroupId": groupB.ID, "itemIds": []uint{a1.ID, b1.ID}},
		},
	})
	if resp.Code == 0 {
		t.Fatalf("expected failure, got success")
	}
	assertIds(t, "groupA", groupItems(t, db, groupA.ID), a1.ID, a2.ID)
	assertIds(t, "groupB", groupItems(t, db, groupB.ID), b1.ID)
}

// 重复卡片
func TestSaveSortBatchRejectsDuplicateItem(t *testing.T) {
	db := setupSortBatchTestDB(t)
	router := setupSortBatchRouter(t, db, sortBatchTestUserA, true)

	groupA := sortBatchGroup(t, db, sortBatchTestUserA, "A")
	groupB := sortBatchGroup(t, db, sortBatchTestUserA, "B")
	a1 := sortBatchItem(t, db, sortBatchTestUserA, groupA.ID, "a1", 1)

	resp := doSortBatchPost(t, router, map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": groupA.ID, "itemIds": []uint{a1.ID}},
			{"itemIconGroupId": groupB.ID, "itemIds": []uint{a1.ID}},
		},
	})
	if resp.Code == 0 {
		t.Fatalf("expected failure, got success")
	}
	assertIds(t, "groupA", groupItems(t, db, groupA.ID), a1.ID)
}

// 重复分组
func TestSaveSortBatchRejectsDuplicateGroup(t *testing.T) {
	db := setupSortBatchTestDB(t)
	router := setupSortBatchRouter(t, db, sortBatchTestUserA, true)

	groupA := sortBatchGroup(t, db, sortBatchTestUserA, "A")

	resp := doSortBatchPost(t, router, map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": groupA.ID, "itemIds": []uint{}},
			{"itemIconGroupId": groupA.ID, "itemIds": []uint{}},
		},
	})
	if resp.Code == 0 {
		t.Fatalf("expected failure, got success")
	}
}

// 未登录：必须拒绝
func TestSaveSortBatchRequiresLogin(t *testing.T) {
	db := setupSortBatchTestDB(t)
	router := setupSortBatchRouter(t, db, 0, false)

	groupA := sortBatchGroup(t, db, sortBatchTestUserA, "A")

	resp := doSortBatchPost(t, router, map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": groupA.ID, "itemIds": []uint{}},
		},
	})
	if resp.Code != 1005 {
		t.Fatalf("code = %d, want 1005, msg = %s", resp.Code, resp.Msg)
	}
}

// 核心层参数校验（不经过 HTTP）
func TestSaveItemIconSortBatchValidation(t *testing.T) {
	db := setupSortBatchTestDB(t)
	groupA := sortBatchGroup(t, db, sortBatchTestUserA, "A")

	cases := []struct {
		name   string
		groups []ItemIconSortBatchGroup
	}{
		{name: "empty groups", groups: nil},
		{name: "zero group id", groups: []ItemIconSortBatchGroup{{ItemIconGroupId: 0}}},
		{name: "duplicate group", groups: []ItemIconSortBatchGroup{{ItemIconGroupId: groupA.ID}, {ItemIconGroupId: groupA.ID}}},
		{name: "zero item id", groups: []ItemIconSortBatchGroup{{ItemIconGroupId: groupA.ID, ItemIds: []uint{0}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := SaveItemIconSortBatchWithDB(db, sortBatchTestUserA, tc.groups); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}
