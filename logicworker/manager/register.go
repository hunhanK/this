/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/9/13 21:13
 */

package manager

import (
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
)

type (
	InitFnType func() pb3.Message
	DataFnType func(actor iface.IPlayer) pb3.Message
)

var (
	mInitFn = make(map[uint32]InitFnType) // 数据对应的proto结构
	mDataFn = make(map[uint32]DataFnType)
)

func Register(dataId uint32, initFn InitFnType, dataFn DataFnType) {
	if _, ok := mInitFn[dataId]; ok {
		logger.LogStack("注册玩家数据初始化方法重复. 数据id：%d", dataId)
		return
	}

	if _, ok := mDataFn[dataId]; ok {
		logger.LogStack("注册玩家数据获取方法重复. 数据id：%d", dataId)
		return
	}

	mInitFn[dataId] = initFn
	mDataFn[dataId] = dataFn
}

func GetInitFn(dataId uint32) InitFnType {
	return mInitFn[dataId]
}

func GetDataFn(dataId uint32) DataFnType {
	return mDataFn[dataId]
}
