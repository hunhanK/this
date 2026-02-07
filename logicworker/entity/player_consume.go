package entity

import (
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/functional"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
)

func (player *Player) ConsumeByConf(consumes jsondata.ConsumeVec, autoBuy bool, params common.ConsumeParams) bool {

	consumer := player.Consumer

	normalizedConsume, valid := consumer.NormalizeConsumeConf(consumes, autoBuy)
	if !valid {
		return valid
	}

	params.DrawAwardsSubDiamondAddRate = player.GetFightAttr(attrdef.DrawAwardsSubDiamondAddRate)
	success, _ := consumer.Consume(normalizedConsume, params)
	return success
}

func (player *Player) ConsumeByConfWithRet(consumes jsondata.ConsumeVec, autoBuy bool, params common.ConsumeParams) (bool, argsdef.RemoveMaps) {

	consumer := player.Consumer

	normalizedConsume, valid := consumer.NormalizeConsumeConf(consumes, autoBuy)
	if !valid {
		return valid, argsdef.RemoveMaps{}
	}

	params.DrawAwardsSubDiamondAddRate = player.GetFightAttr(attrdef.DrawAwardsSubDiamondAddRate)
	success, remove := consumer.Consume(normalizedConsume, params)
	return success, remove
}

func (player *Player) CheckConsumeByConf(consumes jsondata.ConsumeVec, autoBuy bool, logId pb3.LogId) bool {
	consumer := player.Consumer

	normalizedConsume, valid := consumer.NormalizeConsumeConf(consumes, autoBuy)
	if !valid {
		return valid
	}

	removeMap, _, valid := consumer.CalcRemoveAndBuyItem(normalizedConsume, common.ConsumeParams{LogId: logId, DrawAwardsSubDiamondAddRate: player.GetFightAttr(attrdef.DrawAwardsSubDiamondAddRate)})
	if !valid {
		return valid
	}

	return consumer.CheckEnoughByRemoveMaps(removeMap)
}

func (player *Player) ConsumeRate(consumes jsondata.ConsumeVec, count int64, autoBuy bool, params common.ConsumeParams) bool {
	consumes = jsondata.ConsumeMulti(consumes, uint32(count))
	return player.ConsumeByConf(consumes, autoBuy, params)
}

func onFightCallConsume(player iface.IPlayer, buf []byte) {
	var req pb3.ConsumeReq
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("OnFightCallConsume Unmarshal err:%v", err)
		return
	}

	consumeVec := functional.Map(req.ConsumeVec, func(foo *pb3.Consume) *jsondata.Consume {
		return &jsondata.Consume{
			Id:         foo.Id,
			Count:      foo.Count,
			CanAutoBuy: foo.CanAutoBuy,
			Type:       uint8(foo.Type),
			Job:        foo.Job,
		}
	})

	state := player.ConsumeByConf(consumeVec, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId(req.LogId)})
	player.CallActorFunc(actorfuncid.ConsumeCallBack, &pb3.ConsumeRes{
		State:     state,
		CbId:      req.CbId,
		ExtendArg: req.ExtendArg,
	})

	logger.LogDebug("received onFightCallConsume")
}

func init() {
	engine.RegisterActorCallFunc(playerfuncid.Consume, onFightCallConsume)
}
