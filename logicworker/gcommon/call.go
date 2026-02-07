package gcommon

import "github.com/gzjjyz/logger"

// CheckArgsCount 检查函数参数
func CheckArgsCount(fName string, expect, got int) bool {
	if got < expect {
		logger.LogStack("%s CheckArgsCount error!!!, expect=%d, got=%d", fName, expect, got)
		return false
	}
	return true
}
