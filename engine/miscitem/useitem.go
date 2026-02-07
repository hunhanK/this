/**
 * @Author: DaiGuanYu
 * @Desc:   道具使用
 * @Date: 2022/10/10 14:45
 */

package miscitem

import (
	"jjyz/base/jsondata"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
)

// ResultHandleFunc 需要目标的技能结果
type commonUseItemHandleFunc func(player iface.IPlayer, param *UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64)

var (
	resultTargetReqHandleMap = map[uint32]commonUseItemHandleFunc{}
)

func RegCommonUseItemHandle(ut uint32, cb commonUseItemHandleFunc) {
	if _, exists := resultTargetReqHandleMap[ut]; exists {
		logger.LogStack("RegCommonUseItemHandler repeat!!! type=%d", ut)
		return
	}
	resultTargetReqHandleMap[ut] = cb
}

func commonUseItem(player iface.IPlayer, param *UseItemParamSt) (success, del bool, cnt int64) {
	conf := jsondata.GetUseItemConfById(param.ItemId)
	if nil == conf {
		return false, false, 0
	}

	fn, ok := resultTargetReqHandleMap[conf.Type]
	if !ok {
		return false, false, 0
	}

	return fn(player, param, conf)
}

func init() {
	RegisterLoadFunc(func() {
		conf := jsondata.GetUseItemConf()
		for key := range conf {
			RegItemUseHandle(key, commonUseItem)
		}
	})
}
