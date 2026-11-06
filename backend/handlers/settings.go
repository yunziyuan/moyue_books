package handlers

import (
	"net/http"

	"my-books/backend/config"
	"my-books/backend/middleware"
	"my-books/backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

const (
	// SettingKeyShelfMode 书架使用习惯：按账号持久化，保证各端一致
	SettingKeyShelfMode = "shelf_mode"
	ShelfModeAll        = "all"      // 显示全部书籍（默认）
	ShelfModeSelected   = "selected" // 仅显示指定到书架的书籍
)

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

// GetUserSettings 读取账号级偏好（GET /api/settings）
func GetUserSettings(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	c.JSON(http.StatusOK, gin.H{
		"shelf_mode": normalizeShelfMode(readUserSetting(uid, SettingKeyShelfMode, ShelfModeAll)),
	})
}

// UpdateUserSettings 更新账号级偏好（PUT /api/settings）
func UpdateUserSettings(c *gin.Context) {
	var input struct {
		ShelfMode *string `json:"shelf_mode"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.ShelfMode == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "shelf_mode 不能为空"})
		return
	}
	mode := *input.ShelfMode
	if mode != ShelfModeAll && mode != ShelfModeSelected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "shelf_mode 取值无效"})
		return
	}
	if err := writeUserSetting(middleware.CurrentUserID(c), SettingKeyShelfMode, mode); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"shelf_mode": mode})
}
