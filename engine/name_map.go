/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 14:02
 */

package engine

import (
	emoji "github.com/go-xman/go.emoji"
	"github.com/gzjjyz/logger"
	"jjyz/base/db"
	"jjyz/base/jsondata"
	"regexp"
	"sync"
	"unicode"
)

var (
	emptyObj           = struct{}{}
	mPaddingPlayerName sync.Map
	mAllPlayerName     = make(map[string]struct{})
)

func LoadName() error {
	var names []string
	if err := db.OrmEngine.Table("actors").Cols("actor_name").Find(&names); nil != err {
		logger.LogError("********** loadallactorname error! server stop!**********")
		return err
	}

	for _, name := range names {
		mAllPlayerName[name] = struct{}{}
	}
	return nil
}

func CheckNameRepeat(name string) bool {
	if _, ok := mAllPlayerName[name]; ok {
		return false
	}

	if IsPendingName(name) {
		return false
	}

	if jsondata.CheckBattleArenaRobotNameRepeat(name) {
		return false
	}

	if jsondata.CheckMainCityRobotNameRepeat(name) {
		return false
	}
	return true
}

func isValidString(s string) bool {
	// 正则表达式：只允许中文、数字、大小写字母
	// Go的正则表达式中，表示Unicode字符范围通常使用 \p{Script=Han} 来匹配中文字符，或者使用 \p{L} 来匹配任何字母字符（包括中文），\p{N} 来匹配任何数字字符。
	pattern := "^[\\p{L}\\p{N}·]+$"
	// 编译正则表达式
	re, err := regexp.Compile(pattern)
	if err != nil {
		logger.LogDebug("正则表达式编译错误:%v", err)
		return false
	}
	// 匹配字符串
	matched := re.MatchString(s)
	return matched
}

// CheckNameSpecialCharacter 检查名字的特殊字符
func CheckNameSpecialCharacter(name string) bool {
	// 空白符号
	for _, c := range name {
		if unicode.IsSpace(c) {
			return false
		}
	}
	// emoji 表情
	if emoji.HasEmoji(name) {
		return false
	}

	// 只允许中文、数字、大小写字母 ·
	if !isValidString(name) {
		return false
	}
	return true
}

func RemovePlayerName(name string) {
	delete(mAllPlayerName, name)
}

func AddPlayerName(name string) {
	mAllPlayerName[name] = struct{}{}
}

func AddPendingName(name string) {
	mPaddingPlayerName.Store(name, emptyObj)
}

func IsPendingName(name string) bool {
	_, ok := mPaddingPlayerName.Load(name)
	return ok
}

func DelPendingName(name string) {
	mPaddingPlayerName.Delete(name)
}
