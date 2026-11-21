package system

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"sun-panel/api/api_v1/middleware"
	"sun-panel/global"
	"sun-panel/initialize/database"
	"sun-panel/lib/cache"
	"sun-panel/lib/cmn"
	"sun-panel/lib/security"
	"sun-panel/models"
)

type preUpgradeUser struct {
	models.BaseModel
	Username     string `gorm:"index:;index:idx_username_password,priority:1;type:varchar(50)"`
	Password     string `gorm:"index:idx_username_password;type:varchar(32)"`
	Name         string `gorm:"type:varchar(20)"`
	HeadImage    string `gorm:"type:varchar(200)"`
	Status       int    `gorm:"type:tinyint(1)"`
	Role         int    `gorm:"type:int(11)"`
	Mail         string `gorm:"type:varchar(50)"`
	ReferralCode string `gorm:"type:varchar(10)"`
	Token        string `gorm:"type:varchar(32)"`
}

func (preUpgradeUser) TableName() string { return "user" }

func TestAuthenticationUpgrade(t *testing.T) {
	db, err := database.DbInit(&database.SQLiteConfig{Filename: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	oldDB, oldModels := global.Db, models.Db
	oldClientTokens, oldUserTokens := global.CUserToken, global.UserToken
	global.Db, models.Db = db, db
	global.CUserToken, global.UserToken = cache.NewGoCache[string](time.Hour, 0), cache.NewGoCache[models.User](time.Hour, 0)
	t.Cleanup(func() {
		global.Db, models.Db = oldDB, oldModels
		global.CUserToken, global.UserToken = oldClientTokens, oldUserTokens
		if connection, err := db.DB(); err == nil {
			_ = connection.Close()
		}
	})
	// The two auth models are the original preview table definitions. Seed their
	// tables beside the original users schema before running the app's migration.
	if err := db.AutoMigrate(&preUpgradeUser{}, &models.SecurityTotp{}, &models.SecurityPasskey{}); err != nil {
		t.Fatal(err)
	}
	const legacyPassword = "legacy fixture password"
	user := preUpgradeUser{Username: "fixture@example.test", Password: cmn.PasswordEncryption(legacyPassword), Name: "Fixture", Status: 1, Role: 1}
	admin := preUpgradeUser{Username: "admin@sun.cc", Password: cmn.PasswordEncryption(security.LegacyDefaultPassword), Name: "Admin", Status: 1, Role: 1}
	for _, fixture := range []any{&user, &admin} {
		if err := db.Create(fixture).Error; err != nil {
			t.Fatal(err)
		}
	}
	const totpFixture = "JBSWY3DPEHPK3PXP"
	totp := models.SecurityTotp{UserId: user.ID, Secret: totpFixture, Enabled: true}
	passkey := models.SecurityPasskey{UserId: user.ID, CredentialID: "test-id", PublicKey: []byte{1, 2, 3}, SignCount: 7, BackupEligible: true, Name: "Existing key"}
	for _, fixture := range []any{&totp, &passkey} {
		if err := db.Create(fixture).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.First(&totp, totp.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&passkey, passkey.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&user, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.CreateDatabase(database.SQLITE, db); err != nil {
		t.Fatal(err)
	}
	assertRowEqual(t, db, totp.ID, totp)
	assertRowEqual(t, db, passkey.ID, passkey)
	assertRowEqual(t, db, user.ID, user)

	r := gin.New()
	if err := r.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	login, userAPI := LoginApi{}, UserApi{}
	r.POST("/api/login", login.Login)
	r.POST("/api/user/updatePassword", middleware.LoginInterceptor, userAPI.UpdatePasssword)
	r.POST("/api/user/getInfo", middleware.LoginInterceptor, userAPI.GetInfo)
	r.POST("/api/logout", middleware.LoginInterceptor, login.Logout)
	request := func(path, clientToken string, body any, expected int) map[string]any {
		t.Helper()
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if clientToken != "" {
			req.Header.Set("token", clientToken)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var envelope struct {
			Code int
			Data map[string]any
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatal("invalid response envelope", err)
		}
		if envelope.Code != expected {
			t.Fatalf("%s: got code %d, expected %d", path, envelope.Code, expected)
		}
		return envelope.Data
	}
	credentials := gin.H{"username": user.Username, "password": legacyPassword}
	request("/api/login", "", credentials, 1101)
	code, err := security.TotpCodeAt(totpFixture, time.Now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	credentials["totpCode"] = code
	loggedIn := request("/api/login", "", credentials, 0)
	if len(loggedIn) != 8 || loggedIn["username"] != user.Username {
		t.Fatal("unexpected login response fields")
	}
	request("/api/login", "", credentials, 1102)
	var current models.User
	if err := db.First(&current, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(current.Password, "$argon2id$") || current.Token == loggedIn["token"] {
		t.Fatal("legacy migration or session isolation failed")
	}
	request("/api/logout", loggedIn["token"].(string), nil, 0)
	request("/api/user/getInfo", loggedIn["token"].(string), nil, 1001)

	restricted := request("/api/login", "", gin.H{"username": admin.Username, "password": security.LegacyDefaultPassword}, 0)
	if restricted["mustChangePassword"] != true {
		t.Fatal("original password did not require a change")
	}
	restrictedToken := restricted["token"].(string)
	request("/api/user/getInfo", restrictedToken, nil, 1105)
	const replacement = "replacement fixture password"
	request("/api/user/updatePassword", restrictedToken, gin.H{"oldPassword": security.LegacyDefaultPassword, "newPassword": replacement}, 0)
	request("/api/user/getInfo", restrictedToken, nil, 1001)
	request("/api/login", "", gin.H{"username": admin.Username, "password": security.LegacyDefaultPassword}, 1003)
	fresh := request("/api/login", "", gin.H{"username": admin.Username, "password": replacement}, 0)
	request("/api/user/getInfo", fresh["token"].(string), nil, 0)
}

func assertRowEqual[T any](t *testing.T, db *gorm.DB, id uint, expected T) {
	t.Helper()
	var got T
	if err := db.First(&got, id).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("migration changed %T (id %d)", expected, id)
	}
}
