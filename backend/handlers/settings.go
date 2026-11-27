package handlers

import (
	"net/http"
	"strings"

	"my-books/backend/config"
	"my-books/backend/middleware"
	"my-books/backend/models"
	"my-books/backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

const (
	// SettingKeyShelfMode 书架使用习惯：按账号持久化，保证各端一致
	SettingKeyShelfMode = "shelf_mode"
	ShelfModeAll        = "all"      // 显示全部书籍（默认）
	ShelfModeSelected   = "selected" // 仅显示指定到书架的书籍
)

// ttsCredentialFields 设置页可编辑的 TTS 凭据白名单。
// Secret=true 的字段只在 GET 时回显「是否已配置」，绝不返回明文。
var ttsCredentialFields = []struct {
	JSON   string
	Key    string
	Secret bool
}{
	{"xf_app_id", services.TTSSettingXfAppID, false},
	{"xf_api_key", services.TTSSettingXfAPIKey, true},
	{"xf_api_secret", services.TTSSettingXfAPISecret, true},
	{"xf_api_password", services.TTSSettingXfAPIPassword, true},
	{"deepseek_api_key", services.TTSSettingDeepSeekKey, true},
	{"deepseek_base_url", services.TTSSettingDeepSeekURL, false},
}

func normalizeShelfMode(v string) string {
	if v == ShelfModeSelected {
		return ShelfModeSelected
	}
	return ShelfModeAll
}

// readUserSetting 读取用户级设置项，不存在时返回默认值（Find 不产生 record not found 日志）
func readUserSetting(uid uint, key, def string) string {
	var s models.Setting
	if err := config.DB.Where("user_id = ? AND key = ?", uid, key).Limit(1).Find(&s).Error; err != nil || s.Value == "" {
		return def
	}
	return s.Value
}

// writeUserSetting 原子 upsert：idx_user_key 保证唯一，多端同时保存也不会冲突
func writeUserSetting(uid uint, key, value string) error {
	return config.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&models.Setting{UserID: uid, Key: key, Value: value}).Error
}

// GetUserSettings 读取账号级偏好（GET /api/settings）。密钥只回显配置状态。
func GetUserSettings(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	creds := gin.H{}
	for _, f := range ttsCredentialFields {
		value := readUserSetting(uid, f.Key, "")
		if f.Secret {
			creds[f.JSON+"_set"] = value != ""
		} else {
			creds[f.JSON] = value
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"shelf_mode":      normalizeShelfMode(readUserSetting(uid, SettingKeyShelfMode, ShelfModeAll)),
		"tts_credentials": creds,
	})
}

// UpdateUserSettings 更新账号级偏好（PUT /api/settings）。
// 仅更新请求中出现的字段；TTS 密钥传空字符串表示清除该项。
func UpdateUserSettings(c *gin.Context) {
	var input map[string]*string
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUserID(c)

	if v, ok := input["shelf_mode"]; ok {
		if v == nil || (*v != ShelfModeAll && *v != ShelfModeSelected) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "shelf_mode 取值无效"})
			return
		}
		if err := writeUserSetting(uid, SettingKeyShelfMode, *v); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	for _, f := range ttsCredentialFields {
		v, ok := input[f.JSON]
		if !ok || v == nil {
			continue
		}
		value := strings.TrimSpace(*v)
		if len(value) > 2048 {
			c.JSON(http.StatusBadRequest, gin.H{"error": f.JSON + " 长度超出限制"})
			return
		}
		var err error
		if value == "" {
			err = config.DB.Where("user_id = ? AND key = ?", uid, f.Key).Delete(&models.Setting{}).Error
		} else {
			err = writeUserSetting(uid, f.Key, value)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	GetUserSettings(c)
}
