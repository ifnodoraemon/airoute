package controlplane

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

const notificationBodyLimit = 64 << 10

func normalizeNotificationText(title, content, level *string, required bool) error {
	*title, *content, *level = strings.TrimSpace(*title), strings.TrimSpace(*content), strings.TrimSpace(*level)
	if required && (*title == "" || *content == "") {
		return fmt.Errorf("标题和内容不能为空")
	}
	if utf8.RuneCountInString(*title) > 120 || utf8.RuneCountInString(*content) > 10000 {
		return fmt.Errorf("标题最多 120 个字符，内容最多 10000 个字符")
	}
	if *level == "" {
		*level = "info"
	}
	if *level != "info" && *level != "warning" && *level != "critical" {
		return fmt.Errorf("通知级别无效")
	}
	return nil
}

func notificationFilter(c *gin.Context) (storage.NotificationFilter, error) {
	f := storage.NotificationFilter{Kind: c.Query("kind"), Unread: c.Query("unread") == "true"}
	if f.Kind == "all" {
		f.Kind = ""
	}
	if f.Kind != "" && f.Kind != "announcement" && f.Kind != "message" {
		return f, fmt.Errorf("消息类型无效")
	}
	if value := c.Query("unread"); value != "" && value != "true" && value != "false" {
		return f, fmt.Errorf("未读筛选参数无效")
	}
	var err error
	f.Page, err = notificationPageValue(c.DefaultQuery("page", "1"), 100000)
	if err == nil {
		f.PageSize, err = notificationPageValue(c.DefaultQuery("page_size", "20"), 100)
	}
	return f, err
}

func notificationPageValue(value string, maximum int) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > maximum {
		return 0, fmt.Errorf("消息分页参数无效")
	}
	return n, nil
}

func requireNotificationUser(c *gin.Context) (int64, bool) {
	if _, ok := RequireAuthClaims(c); !ok {
		return 0, false
	}
	id := GetSessionUserID(c)
	if id <= 0 {
		c.JSON(401, gin.H{"code": 401, "error": "无法验证当前账号，请重新登录"})
		return 0, false
	}
	c.Header("Cache-Control", "no-store")
	return id, true
}
