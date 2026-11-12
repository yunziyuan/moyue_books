package handlers

import (
	"net/http"
	"os"

	"my-books/backend/config"
	"my-books/backend/middleware"
	"my-books/backend/models"
	"my-books/backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CleanupReport 一键清理的明细报告：各类型清理/影响的记录数。
type CleanupReport struct {
	DryRun                bool  `json:"dry_run"`                  // 是否为预览（仅统计未删除）
	DuplicateBooks        int64 `json:"duplicate_books"`          // 删除的重复图书数（保留最完整的一份）
	MissingSourceBooks    int64 `json:"missing_source_books"`     // 删除的「源文件缺失」图书数
	EmptyCategories       int64 `json:"empty_categories"`         // 删除的空分类数（无图书且无子分类）
	OrphanAnnotations     int64 `json:"orphan_annotations"`       // 删除的孤儿批注数
	OrphanBookmarks       int64 `json:"orphan_bookmarks"`         // 删除的孤儿书签数
	OrphanReviews         int64 `json:"orphan_reviews"`           // 删除的孤儿读后感数
	OrphanReadingHistory  int64 `json:"orphan_reading_history"`   // 删除的孤儿阅读历史数
	OrphanCollectionBooks int64 `json:"orphan_collection_books"`  // 删除的孤儿收藏关联数
}

// CleanupData 一键去重复书架及无效数据。
//   - 重复图书：同一 user 下 file_path 完全相同的多份入库，保留阅读进度最高/最近更新的一份，
//     其余的批注、书签、读后感、阅读历史、收藏关联先迁移到保留本，再删除重复记录。
//   - 无效数据：源文件已消失（source_missing）的图书、没有任何图书归属且无子分类的空分类，
//     以及指向已不存在图书的孤儿子记录（批注/书签/读后感/阅读历史/收藏关联）。
//
// 支持 ?dry_run=1 仅统计并返回将清理的数量，不执行任何删除，供前端预览确认。
func CleanupData(c *gin.Context) {
	uid := middleware.CurrentUserID(c)
	dryRun := c.Query("dry_run") == "1" || c.Query("dry_run") == "true"

	report := CleanupReport{DryRun: dryRun}

	// 只读统计复用主库连接；写操作统一走 exec（dry_run 时空操作）。
	ro := config.DB

	// 预览阶段：所有计数已在各步骤外层完成，exec 直接空操作，绝不触碰数据库。
	// 执行阶段：用事务包裹全部写操作，保证原子性。
	exec := func(fn func(tx *gorm.DB)) {
		if dryRun {
			return
		}
		tx := config.DB.Begin()
		if tx.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败"})
			return
		}
		fn(tx)
		if c.IsAborted() {
			tx.Rollback()
			return
		}
		if err := tx.Commit().Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "清理失败：" + err.Error()})
		}
	}

	// ---- 1. 重复图书（按 file_path 分组） ----
	type dupRow struct {
		FilePath string
		Cnt      int64
	}
	var dups []dupRow
	ro.Model(&models.Book{}).
		Select("file_path, COUNT(*) as cnt").
		Where("user_id = ? AND file_path <> ''", uid).
		Group("file_path").
		Having("COUNT(*) > 1").
		Scan(&dups)

	for _, d := range dups {
		var books []models.Book
		ro.Where("user_id = ? AND file_path = ?", uid, d.FilePath).
			Order("progress DESC, updated_at DESC").
			Find(&books)
		if len(books) < 2 {
			continue
		}
		keepID := books[0].ID
		dupIDs := make([]uint, 0, len(books)-1)
		for _, b := range books[1:] {
			dupIDs = append(dupIDs, b.ID)
		}
		report.DuplicateBooks += int64(len(dupIDs))
		exec(func(tx *gorm.DB) {
			// 子记录迁移到保留本（收藏关联需规避复合主键冲突）
			tx.Model(&models.Annotation{}).Where("user_id = ? AND book_id IN ?", uid, dupIDs).Update("book_id", keepID)
			tx.Model(&models.Bookmark{}).Where("user_id = ? AND book_id IN ?", uid, dupIDs).Update("book_id", keepID)
			tx.Model(&models.BookReview{}).Where("user_id = ? AND book_id IN ?", uid, dupIDs).Update("book_id", keepID)
			tx.Model(&models.ReadingHistory{}).Where("user_id = ? AND book_id IN ?", uid, dupIDs).Update("book_id", keepID)
			tx.Exec(`UPDATE collection_books SET book_id = ?
				WHERE user_id = ? AND book_id IN ?
				AND NOT EXISTS (SELECT 1 FROM collection_books cb WHERE cb.collection_id = collection_books.collection_id AND cb.book_id = ?)`,
				keepID, uid, dupIDs, keepID)
			tx.Where("user_id = ? AND book_id IN ?", uid, dupIDs).Delete(&models.CollectionBook{})
			tx.Where("book_id IN ?", dupIDs).Delete(&models.BookShare{})
			// 删除重复图书（封面为派生缩略图，一并清理）
			for _, id := range dupIDs {
				var b models.Book
				if tx.First(&b, id).Error == nil && b.CoverPath != "" {
					os.Remove(utils.ResolvePath(b.CoverPath))
				}
			}
			tx.Where("user_id = ? AND id IN ?", uid, dupIDs).Delete(&models.Book{})
		})
		if c.IsAborted() {
			return
		}
	}

	// ---- 2. 源文件缺失的图书 ----
	var missing []models.Book
	ro.Where("user_id = ? AND source_missing = ?", uid, true).Find(&missing)
	report.MissingSourceBooks = int64(len(missing))
	exec(func(tx *gorm.DB) {
		ids := make([]uint, 0, len(missing))
		for _, b := range missing {
			ids = append(ids, b.ID)
			if b.CoverPath != "" {
				os.Remove(utils.ResolvePath(b.CoverPath))
			}
		}
		if len(ids) > 0 {
			cleanupDeleteBookChildren(tx, uid, ids)
			tx.Where("book_id IN ?", ids).Delete(&models.BookShare{})
			tx.Where("user_id = ? AND id IN ?", uid, ids).Delete(&models.Book{})
		}
	})
	if c.IsAborted() {
		return
	}

	// ---- 3. 空分类（无图书且无子分类） ----
	var cats []models.Category
	ro.Where("user_id = ?", uid).Find(&cats)
	for _, cat := range cats {
		var bookCnt, childCnt int64
		ro.Model(&models.Book{}).Where("user_id = ? AND category = ?", uid, cat.Name).Count(&bookCnt)
		if bookCnt > 0 {
			continue
		}
		ro.Model(&models.Category{}).Where("user_id = ? AND parent = ?", uid, cat.Name).Count(&childCnt)
		if childCnt > 0 {
			continue
		}
		report.EmptyCategories++
		exec(func(tx *gorm.DB) {
			tx.Delete(&cat)
		})
		if c.IsAborted() {
			return
		}
	}

	// ---- 4. 孤儿子记录（指向已不存在的图书） ----
	validIDs := ro.Model(&models.Book{}).Where("user_id = ?", uid).Select("id")
	countOrphans := func(model interface{}, field *int64) {
		var cnt int64
		ro.Model(model).Where("user_id = ? AND book_id NOT IN (?)", uid, validIDs).Count(&cnt)
		*field = cnt
		exec(func(tx *gorm.DB) {
			tx.Where("user_id = ? AND book_id NOT IN (?)", uid, validIDs).Delete(model)
		})
	}
	countOrphans(&models.Annotation{}, &report.OrphanAnnotations)
	if c.IsAborted() {
		return
	}
	countOrphans(&models.Bookmark{}, &report.OrphanBookmarks)
	if c.IsAborted() {
		return
	}
	countOrphans(&models.BookReview{}, &report.OrphanReviews)
	if c.IsAborted() {
		return
	}
	countOrphans(&models.ReadingHistory{}, &report.OrphanReadingHistory)
	if c.IsAborted() {
		return
	}
	countOrphans(&models.CollectionBook{}, &report.OrphanCollectionBooks)
	if c.IsAborted() {
		return
	}

	if c.IsAborted() {
		return
	}
	c.JSON(http.StatusOK, report)
}

// cleanupDeleteBookChildren 删除指定图书的子记录（批注/书签/读后感/阅读历史/收藏关联）。
func cleanupDeleteBookChildren(tx *gorm.DB, uid uint, bookIDs []uint) {
	tx.Where("user_id = ? AND book_id IN ?", uid, bookIDs).Delete(&models.Annotation{})
	tx.Where("user_id = ? AND book_id IN ?", uid, bookIDs).Delete(&models.Bookmark{})
	tx.Where("user_id = ? AND book_id IN ?", uid, bookIDs).Delete(&models.BookReview{})
	tx.Where("user_id = ? AND book_id IN ?", uid, bookIDs).Delete(&models.ReadingHistory{})
	tx.Where("user_id = ? AND book_id IN ?", uid, bookIDs).Delete(&models.CollectionBook{})
}
