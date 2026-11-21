package system

import (
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type ModuleConfigApi struct{}

func (a *ModuleConfigApi) GetByName(c *gin.Context) {
	req := models.ModuleConfig{}

	if err := c.ShouldBindWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	visitMode, _ := c.Get(base.GIN_GET_VISIT_MODE)
	public := visitMode == base.VISIT_MODE_PUBLIC
	if public && req.Name != "systemMonitor" && req.Name != "deskModuleSearchBox" {
		apiReturn.ErrorNoAccess(c)
		return
	}
	userInfo, _ := base.GetCurrentUserInfo(c)

	mCfg := models.ModuleConfig{}
	if cfg, err := mCfg.GetConfigByUserIdAndName(global.Db, userInfo.ID, req.Name); err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	} else {
		if public && req.Name == "systemMonitor" {
			cfg = publicMonitorConfig(cfg)
		}
		apiReturn.SuccessData(c, cfg)
	}
}

func publicMonitorConfig(config map[string]any) map[string]any {
	list, _ := config["list"].([]any)
	cards := make([]map[string]any, 0, len(list))
	for _, entry := range list {
		card, _ := entry.(map[string]any)
		monitorType, _ := card["monitorType"].(string)
		if monitorType != "cpu" && monitorType != "memory" {
			continue
		}
		style := make(map[string]any)
		if fields, ok := card["extendParam"].(map[string]any); ok {
			for _, name := range []string{"progressColor", "progressRailColor", "color", "backgroundColor"} {
				if value, ok := fields[name].(string); ok {
					style[name] = value
				}
			}
		}
		public := map[string]any{"monitorType": monitorType, "extendParam": style}
		if description, ok := card["description"].(string); ok {
			public["description"] = description
		}
		cards = append(cards, public)
	}
	return map[string]any{"list": cards}
}

func (a *ModuleConfigApi) Save(c *gin.Context) {
	req := models.ModuleConfig{}
	if err := c.ShouldBindWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	userInfo, _ := base.GetCurrentUserInfo(c)
	mCfg := models.ModuleConfig{}
	mCfg.UserId = userInfo.ID
	mCfg.Value = req.Value
	mCfg.Name = req.Name

	if err := mCfg.Save(global.Db); err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	apiReturn.Success(c)
}
