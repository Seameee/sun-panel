package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sun-panel/global"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	sortApiTestUserA uint = 11
	sortApiTestUserB uint = 12
)

func setupPanelTestRouter(t *testing.T, userId uint) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

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

	itemIcon := ItemIcon{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userInfo", models.User{BaseModel: models.BaseModel{ID: userId}})
		c.Next()
	})
	router.POST("/panel/itemIcon/saveSort", itemIcon.SaveSort)
	router.POST("/panel/itemIcon/saveSortBatch", itemIcon.SaveSortBatch)

	return router, db
}

func panelTestGroup(t *testing.T, db *gorm.DB, userId uint, title string, sort int) models.ItemIconGroup {
	t.Helper()
	group := models.ItemIconGroup{Title: title, Sort: sort, UserId: userId}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	return group
}

func panelTestItem(t *testing.T, db *gorm.DB, userId uint, groupId uint, title string, sort int) models.ItemIcon {
	t.Helper()
	item := models.ItemIcon{Title: title, Sort: sort, UserId: userId, ItemIconGroupId: int(groupId)}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	return item
}

type panelTestResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func doPanelPost(t *testing.T, router *gin.Engine, path string, body any) panelTestResp {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := panelTestResp{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", w.Body.String(), err)
	}
	return resp
}

func panelGroupOrder(t *testing.T, db *gorm.DB, groupId uint) []uint {
	t.Helper()
	items := []models.ItemIcon{}
	if err := db.Where("item_icon_group_id = ?", groupId).Order("sort, created_at").Find(&items).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

// 旧的单分组排序接口必须保持兼容
func TestItemIconSaveSortLegacyStillWorks(t *testing.T) {
	router, db := setupPanelTestRouter(t, sortApiTestUserA)
	group := panelTestGroup(t, db, sortApiTestUserA, "A", 1)
	item1 := panelTestItem(t, db, sortApiTestUserA, group.ID, "1", 1)
	item2 := panelTestItem(t, db, sortApiTestUserA, group.ID, "2", 2)

	resp := doPanelPost(t, router, "/panel/itemIcon/saveSort", map[string]any{
		"itemIconGroupId": group.ID,
		"sortItems": []map[string]any{
			{"id": item2.ID, "sort": 1},
			{"id": item1.ID, "sort": 2},
		},
	})
	if resp.Code != 0 {
		t.Fatalf("legacy saveSort code = %d, msg = %s", resp.Code, resp.Msg)
	}
	got := panelGroupOrder(t, db, group.ID)
	if len(got) != 2 || got[0] != item2.ID || got[1] != item1.ID {
		t.Fatalf("legacy order = %v, want [%d %d]", got, item2.ID, item1.ID)
	}
}

func TestItemIconSaveSortBatchCrossGroup(t *testing.T) {
	router, db := setupPanelTestRouter(t, sortApiTestUserA)
	groupA := panelTestGroup(t, db, sortApiTestUserA, "A", 1)
	groupB := panelTestGroup(t, db, sortApiTestUserA, "B", 2)
	a1 := panelTestItem(t, db, sortApiTestUserA, groupA.ID, "a1", 1)
	b1 := panelTestItem(t, db, sortApiTestUserA, groupB.ID, "b1", 1)

	resp := doPanelPost(t, router, "/panel/itemIcon/saveSortBatch", map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": groupA.ID, "itemIds": []uint{}},
			{"itemIconGroupId": groupB.ID, "itemIds": []uint{a1.ID, b1.ID}},
		},
	})
	if resp.Code != 0 {
		t.Fatalf("saveSortBatch code = %d, msg = %s", resp.Code, resp.Msg)
	}
	got := panelGroupOrder(t, db, groupB.ID)
	if len(got) != 2 || got[0] != a1.ID || got[1] != b1.ID {
		t.Fatalf("group B order = %v, want [%d %d]", got, a1.ID, b1.ID)
	}
	if order := panelGroupOrder(t, db, groupA.ID); len(order) != 0 {
		t.Fatalf("group A should be empty, got %v", order)
	}
}

func TestItemIconSaveSortBatchRejectsForeignGroup(t *testing.T) {
	router, db := setupPanelTestRouter(t, sortApiTestUserA)
	foreignGroup := panelTestGroup(t, db, sortApiTestUserB, "B", 1)

	resp := doPanelPost(t, router, "/panel/itemIcon/saveSortBatch", map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": foreignGroup.ID, "itemIds": []uint{}},
		},
	})
	if resp.Code != 1005 {
		t.Fatalf("code = %d, want 1005 (no permission), msg = %s", resp.Code, resp.Msg)
	}
}

func TestItemIconSaveSortBatchRejectsStaleData(t *testing.T) {
	router, db := setupPanelTestRouter(t, sortApiTestUserA)
	group := panelTestGroup(t, db, sortApiTestUserA, "A", 1)
	a1 := panelTestItem(t, db, sortApiTestUserA, group.ID, "a1", 1)
	a2 := panelTestItem(t, db, sortApiTestUserA, group.ID, "a2", 2)

	resp := doPanelPost(t, router, "/panel/itemIcon/saveSortBatch", map[string]any{
		"groups": []map[string]any{
			{"itemIconGroupId": group.ID, "itemIds": []uint{a1.ID}},
		},
	})
	if resp.Code != 1400 {
		t.Fatalf("code = %d, want 1400 (stale data), msg = %s", resp.Code, resp.Msg)
	}
	got := panelGroupOrder(t, db, group.ID)
	if len(got) != 2 || got[0] != a1.ID || got[1] != a2.ID {
		t.Fatalf("order changed on failure: %v", got)
	}
}
