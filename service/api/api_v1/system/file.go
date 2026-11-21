package system

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"sun-panel/api/api_v1/common/apiData/commonApiStructs"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/lib/safefile"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

type FileApi struct{}

func (a *FileApi) UploadImg(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)
	configUpload := global.Config.GetValueString("base", "source_path")

	// 限制请求体大小（单张图片 + 表单冗余）
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, safefile.MaxImageBytes+(1<<20))

	f, err := c.FormFile("imgfile")
	if err != nil {
		apiReturn.ErrorByCode(c, 1300)
		return
	}

	filepath, fileExt, err := saveImageUpload(f, configUpload)
	if err != nil {
		switch err {
		case errUploadExtNotAllowed, errUploadContentMismatch, errUploadTooLarge:
			apiReturn.ErrorByCode(c, 1301)
		default:
			global.Logger.Errorln("upload img:", err)
			apiReturn.ErrorByCode(c, 1300)
		}
		return
	}

	// 向数据库添加记录
	mFile := models.File{}
	if _, err := mFile.AddFile(userInfo.ID, f.Filename, fileExt, filepath); err != nil {
		logUploadCleanup(configUpload, filepath)
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}

	apiReturn.SuccessData(c, gin.H{
		"imageUrl": safefile.UploadURLPath(filepath, configUpload),
	})
}

func (a *FileApi) UploadFiles(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)
	configUpload := global.Config.GetValueString("base", "source_path")

	// 限制请求体大小
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, safefile.MaxMultipartBytes)

	form, err := c.MultipartForm()
	if err != nil {
		apiReturn.ErrorByCode(c, 1300)
		return
	}
	files := form.File["files[]"]
	if len(files) > maxUploadFiles {
		apiReturn.ErrorByCode(c, 1301)
		return
	}

	errFiles := []string{}
	succMap := map[string]string{}
	for _, f := range files {
		filepath, fileExt, err := saveImageUpload(f, configUpload)
		if err != nil {
			errFiles = append(errFiles, f.Filename)
			continue
		}

		// 向数据库添加记录
		mFile := models.File{}
		if _, err := mFile.AddFile(userInfo.ID, f.Filename, fileExt, filepath); err != nil {
			logUploadCleanup(configUpload, filepath)
			errFiles = append(errFiles, f.Filename)
			continue
		}
		succMap[f.Filename] = safefile.UploadURLPath(filepath, configUpload)
	}

	apiReturn.SuccessData(c, gin.H{
		"succMap":  succMap,
		"errFiles": errFiles,
	})
}

func (a *FileApi) GetList(c *gin.Context) {
	list := []models.File{}
	userInfo, _ := base.GetCurrentUserInfo(c)
	var count int64
	if err := global.Db.Order("created_at desc").Find(&list, "user_id=?", userInfo.ID).Count(&count).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}

	data := []map[string]interface{}{}
	configUpload := global.Config.GetValueString("base", "source_path")
	for _, v := range list {
		src := safefile.UploadURLPath(v.Src, configUpload)
		if src == "" {
			continue
		}
		data = append(data, map[string]interface{}{
			"src":        src,
			"fileName":   v.FileName,
			"id":         v.ID,
			"createTime": v.CreatedAt,
			"updateTime": v.UpdatedAt,
			"path":       v.Src,
		})
	}
	apiReturn.SuccessListData(c, data, count)
}

func (a *FileApi) Deletes(c *gin.Context) {
	req := commonApiStructs.RequestDeleteIds[uint]{}
	userInfo, _ := base.GetCurrentUserInfo(c)
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	files := []models.File{}
	if err := global.Db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Find(&files, "user_id=? AND id in ?", userInfo.ID, req.Ids).Error; err != nil {
			return err
		}
		return tx.Delete(&models.File{}, "user_id=? AND id in ?", userInfo.ID, req.Ids).Error
	}); err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	configUpload := global.Config.GetValueString("base", "source_path")
	for _, v := range files {
		logUploadCleanup(configUpload, v.Src)
	}
	apiReturn.Success(c)
}

// ---- 上传校验辅助 ----

var (
	errUploadExtNotAllowed   = errors.New("upload: extension not allowed")
	errUploadContentMismatch = errors.New("upload: content does not match image type")
	errUploadTooLarge        = errors.New("upload: file too large")
)

const maxUploadFiles = 20

// saveImageUpload 校验并保存单个图片，返回磁盘路径与规范化的文件扩展名。
func saveImageUpload(f *multipart.FileHeader, configUpload string) (string, string, error) {
	if f == nil {
		return "", "", errUploadExtNotAllowed
	}

	clientExt := safefile.ExtOf(f.Filename)
	if !safefile.IsAllowedUploadExt(clientExt) {
		return "", "", errUploadExtNotAllowed
	}
	if f.Size <= 0 || f.Size > safefile.MaxImageBytes {
		return "", "", errUploadTooLarge
	}

	src, err := f.Open()
	if err != nil {
		return "", "", err
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, safefile.MaxImageBytes+1))
	if err != nil {
		return "", "", err
	}
	if len(data) > safefile.MaxImageBytes {
		return "", "", errUploadTooLarge
	}
	detectedExt, ok := safefile.DetectImageExt(data[:min(len(data), safefile.SniffLen)])
	if !ok || !safefile.SameImageType(clientExt, detectedExt) {
		return "", "", errUploadContentMismatch
	}
	filepath, err := safefile.SaveImage(configUpload, clientExt, data)
	return filepath, clientExt, err
}

func logUploadCleanup(configUpload, filepath string) {
	if err := safefile.RemoveUpload(configUpload, filepath); err != nil {
		global.Logger.Warn("upload cleanup failed: ", err)
	}
}
