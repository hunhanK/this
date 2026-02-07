/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 祈福
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/random"
)

type BfPraySys struct {
	Base
}

func (sys *BfPraySys) GetData() *pb3.BenefitPray {
	binary := sys.GetBinaryData()
	if nil == binary.Benefit {
		binary.Benefit = &pb3.BenefitData{}
	}
	if nil == binary.Benefit.Pray {
		binary.Benefit.Pray = &pb3.BenefitPray{
			NumMap: make(map[uint32]uint32),
		}
	}
	if binary.Benefit.Pray.NumMap == nil {
		binary.Benefit.Pray.NumMap = make(map[uint32]uint32)
	}
	return binary.Benefit.Pray
}

func (sys *BfPraySys) S2CInfo() {
	sys.SendProto3(41, 6, &pb3.S2C_41_6{
		Pray: sys.GetData(),
	})
}

func (sys *BfPraySys) OnOpen() {
	sys.S2CInfo()
}

func (sys *BfPraySys) OnLogin() {
	sys.S2CInfo()
}

func (sys *BfPraySys) OnReconnect() {
	sys.S2CInfo()
}

// 祈福
func (sys *BfPraySys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_41_6
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.Wrap(err)
	}

	conf, ok := jsondata.GetBenefitPrayConf(req.GetId())
	if !ok {
		return neterror.ConfNotFoundError("not found benefit pray conf ")
	}

	data := sys.GetData()
	userTotal := data.NumMap[req.GetId()]
	owner := sys.GetOwner()

	vipNum, err := owner.GetPrivilege(privilegedef.EnumBfPrayNums)
	if err != nil {
		sys.GetOwner().LogWarn("vip num is zero")
	}

	var freeNum uint32
	if conf.ResetFreeNum {
		freeNum = conf.FreeNum
	}

	confTotal := conf.Num + freeNum + uint32(vipNum)
	if userTotal > uint32(len(conf.Pray)) {
		sys.owner.SendTipMsg(tipmsgid.TpTodayIsLimit)
		return neterror.ParamsInvalidError("The config was wrong and there was no next stage of pray")
	}

	if confTotal <= userTotal {
		sys.owner.SendTipMsg(tipmsgid.TpTodayIsLimit)
		return neterror.ParamsInvalidError("The number of pray its limit today")
	}

	prayConf := conf.Pray[userTotal]
	// 先去消耗
	if len(prayConf.Consume) > 0 {
		if !owner.ConsumeByConf(prayConf.Consume, false, common.ConsumeParams{
			LogId:   pb3.LogId_LogBfPray,
			SubType: req.GetId(),
		}) {
			sys.owner.SendTipMsg(tipmsgid.TpUseItemFailed)
			return neterror.ConsumeFailedError("consume failed")
		}
	}

	// 拿到奖励
	awards := prayConf.Awards

	// 看下暴击率
	cirtConf := conf.FirstCirt
	if userTotal > 0 {
		cirtConf = conf.Cirt
	}

	var num uint32
	if cirtConf != nil {
		pool := new(random.Pool)
		for i := range cirtConf {
			pool.AddItem(cirtConf[i].Num, cirtConf[i].Weight)
		}
		num = pool.RandomOne().(uint32)
		if num > 0 {
			awards = sys.TimesAwards(awards, num)
		}
	}
	// 重新写入
	data.NumMap[req.GetId()] = userTotal + 1
	sys.GetOwner().TriggerQuestEvent(custom_id.QttAnyBfPrayTimes, 0, 1)
	sys.GetOwner().TriggerQuestEvent(custom_id.QttSpecBfPrayTimes, req.Id, 1)

	if awards != nil {
		engine.GiveRewards(owner, awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBfPray})
	}

	// 领取成功
	sys.SendProto3(41, 7, &pb3.S2C_41_7{
		Id:      req.GetId(),
		CirtNum: num,
	})

	sys.S2CInfo()

	return nil
}

func (sys *BfPraySys) TimesAwards(awards jsondata.StdRewardVec, num uint32) jsondata.StdRewardVec {
	var res jsondata.StdRewardVec

	for i := range awards {
		reward := awards[i]
		newReward := reward.Copy()
		newReward.Count = newReward.Count * int64(num)
		res = append(res, newReward)
	}

	return res
}

// 跨天 - 清空次数
func onBenefitsPrayNewDay(player iface.IPlayer, _ ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiBenefitPray).(*BfPraySys)
	if !ok || !sys.IsOpen() {
		return
	}

	numMap := sys.GetData().NumMap
	for k := range numMap {
		numMap[k] = 0
	}

	sys.S2CInfo()
}

func init() {
	RegisterSysClass(sysdef.SiBenefitPray, func() iface.ISystem {
		return &BfPraySys{}
	})
	event.RegActorEvent(custom_id.AeNewDay, onBenefitsPrayNewDay)
	net.RegisterSysProtoV2(41, 6, sysdef.SiBenefitPray, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BfPraySys).c2sAward
	})
}
