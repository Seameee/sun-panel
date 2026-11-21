package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sun-panel/global"
	"sun-panel/lib/language"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoginBodyLimits(t *testing.T) {
	if global.Lang == nil {
		global.Lang = language.NewLang("../../../assets/lang/zh-cn.ini")
		t.Cleanup(func() { global.Lang = nil })
	}
	l := LoginApi{}
	for _, tc := range []struct {
		name    string
		json    string
		limit   int64
		handler gin.HandlerFunc
	}{
		{"password", `{"username":"demo","password":"example"}`, passwordLoginBodyLimit, l.Login},
		{"passkey", `{"flowId":"example","credential":{}}`, passkeyLoginBodyLimit, l.PasskeyLoginFinish},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.json + strings.Repeat(" ", int(tc.limit)-len(tc.json))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			c.Request.ContentLength = -1
			var dst map[string]json.RawMessage
			if err := bindLoginJSON(c, &dst, tc.limit); err != nil || len(dst) != 2 {
				t.Fatalf("valid JSON at the limit: fields=%d, err=%v", len(dst), err)
			}

			for _, n := range []int64{-1, tc.limit + 1} {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body+" "))
				c.Request.ContentLength = n
				tc.handler(c)
				var envelope struct {
					Code int    `json:"code"`
					Msg  string `json:"msg"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || w.Code != http.StatusOK ||
					envelope.Code != -1 || !strings.Contains(envelope.Msg, "request body too large") {
					t.Fatalf("Content-Length %d: status=%d, body=%s, err=%v", n, w.Code, w.Body.String(), err)
				}
			}
		})
	}
}
