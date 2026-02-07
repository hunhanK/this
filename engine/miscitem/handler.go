package miscitem

import (
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"time"

	"github.com/gzjjyz/logger"
)

type UseItemParamSt struct {
	Handle uint64
	ItemId uint32
	Count  int64
	Params []uint32
}

type UseItemHandleFunc func(rr iface.IPlayer, param *UseItemParamSt) (success, del bool, cnt int64)

var (
	UseItemHandle = make(map[uint32]UseItemHandleFunc)
	loadFunc      = make([]func(), 0)
)

// 注册道具使用handle
func RegItemUseHandle(itemId uint32, handle UseItemHandleFunc) {
	if nil == handle {
		logger.LogStack("注册道具使用handle为空, 道具Id:%d", itemId)
		return
	}
	if _, ok := UseItemHandle[itemId]; ok {
		logger.LogStack("重复注册道具使用, 道具Id:%d", itemId)
		return
	}
	UseItemHandle[itemId] = handle
}

// 获取道具使用handle
func GetItemUseHandle(itemId uint32) UseItemHandleFunc {
	if handle, ok := UseItemHandle[itemId]; ok {
		return handle
	}
	return nil
}

func RegisterLoadFunc(fn func()) {
	loadFunc = append(loadFunc, fn)
}

func ReloadItemFunc() {
	start := time.Now()
	logger.LogInfo("开始注册道具使用handle")
	UseItemHandle = make(map[uint32]UseItemHandleFunc)
	for _, fn := range loadFunc {
		fn()
	}
	logger.LogInfo("注册道具使用handle完成, 耗时%v", time.Since(start))
}

func init() {
	//注册道具使用handle
	event.RegSysEvent(custom_id.SeReloadJson, func(args ...interface{}) {
		ReloadItemFunc()
	})
}
