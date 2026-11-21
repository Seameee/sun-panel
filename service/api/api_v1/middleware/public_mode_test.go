package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"sun-panel/global"
	"sun-panel/initialize/database"
	"sun-panel/lib/cache"
	"sun-panel/lib/cmn/systemSetting"
	"sun-panel/lib/iniConfig"
	"sun-panel/lib/language"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"gopkg.in/ini.v1"
)

func TestPublicModeRequiresActiveAccount(t *testing.T) {
	db, err := database.DbInit(&database.SQLiteConfig{Filename: filepath.Join(t.TempDir(), "panel.db")})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "public@example.test", Status: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	languageConfig, err := ini.Load([]byte("[login]\nerr_token_expire=Login required\n"))
	if err != nil {
		t.Fatal(err)
	}
	oldDB, oldSettings, oldLang := global.Db, global.SystemSetting, global.Lang
	t.Cleanup(func() { global.Db, global.SystemSetting, global.Lang = oldDB, oldSettings, oldLang })
	global.Db = db
	global.SystemSetting = &systemSetting.SystemSettingCache{Cache: cache.NewGoCache[interface{}](time.Minute, 0)}
	global.SystemSetting.Cache.SetDefault(systemSetting.PANEL_PUBLIC_USER_ID, strconv.FormatUint(uint64(user.ID), 10))
	global.Lang = &language.LangStructObj{LangContet: &iniConfig.IniConfig{Config: languageConfig}}

	router := gin.New()
	router.GET("/panel", PublicModeInterceptor, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0})
	})
	for _, tc := range []struct{ status, code int }{{1, 0}, {2, 1001}, {3, 1001}, {1, 0}} {
		if err := db.Model(&user).Update("status", tc.status).Error; err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panel", nil))
		var response struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Code != tc.code {
			t.Fatalf("account status %d: response code %d, want %d", tc.status, response.Code, tc.code)
		}
	}
}
