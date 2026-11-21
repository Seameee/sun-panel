package system

import (
	"net/http/httptest"
	"testing"

	"sun-panel/api/api_v1/common/base"
	"sun-panel/lib/monitor"

	"github.com/gin-gonic/gin"
)

func TestCPUResponseUsesPublicAggregate(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := monitor.CPUInfo{CoreCount: 2, CPUNum: 2, Model: "Example CPU", Usages: []float64{20, 40}}
	if got, ok := cpuResponse(c, info).(monitor.CPUInfo); !ok || got.Model != info.Model || len(got.Usages) != 2 {
		t.Fatalf("authenticated response = %#v", got)
	}
	c.Set(base.GIN_GET_VISIT_MODE, base.VISIT_MODE_PUBLIC)
	got, ok := cpuResponse(c, info).(gin.H)
	if !ok || len(got) != 1 {
		t.Fatalf("public response = %#v", got)
	}
	if usages, ok := got["usages"].([]float64); !ok || len(usages) != 1 || usages[0] != 30 {
		t.Fatalf("public usages = %#v", got["usages"])
	}
}

func TestPublicMonitorConfigKeepsPublicCards(t *testing.T) {
	config := map[string]any{"list": []any{
		map[string]any{"monitorType": "cpu", "description": "CPU dashboard", "extendParam": map[string]any{"color": "blue"}},
		map[string]any{"monitorType": "disk", "extendParam": map[string]any{"path": "/data"}},
	}}
	cards, ok := publicMonitorConfig(config)["list"].([]map[string]any)
	if !ok || len(cards) != 1 || cards[0]["monitorType"] != "cpu" || cards[0]["description"] != "CPU dashboard" {
		t.Fatalf("public cards = %#v", cards)
	}
	if style, ok := cards[0]["extendParam"].(map[string]any); !ok || style["color"] != "blue" {
		t.Fatalf("public style = %#v", cards[0]["extendParam"])
	}
}
