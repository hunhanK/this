package iface

import (
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/jsondata"
)

type IConsumer interface {
	CheckEnoughByRemoveMaps(remove argsdef.RemoveMaps) bool
	Consume(normalizedConsumes argsdef.NormalizeConsumesSt, params common.ConsumeParams) (bool, argsdef.RemoveMaps)
	RemoveResource(remove argsdef.RemoveMaps, params common.ConsumeParams) bool
	CalcRemoveAndBuyItem(normalizedConsumes argsdef.NormalizeConsumesSt, params common.ConsumeParams) (remove argsdef.RemoveMaps, buyItemMap map[uint32]int64, valid bool)
	NormalizeConsumeConf(consumes jsondata.ConsumeVec, autoBuy bool) (ret argsdef.NormalizeConsumesSt, valid bool)
	TriggerConsumeEvent(remove argsdef.RemoveMaps, autoBuyItemMap map[uint32]int64, buyItemMap map[uint32]int64)
}
