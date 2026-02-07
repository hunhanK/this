/**
 * @Author: LvYuMeng
 * @Date: 2025/5/22
 * @Desc:
**/

package yy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

type YYZongziFeast struct {
	YYBase
}

func (yy *YYZongziFeast) ResetData() {
	event.TriggerSysEvent(custom_id.SeClearAskForHelpData, custom_id.AskTypeYYZongziFeast)

	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.ZongziFeastData {
		return
	}
	delete(globalVar.YyDatas.ZongziFeastData, yy.GetId())
}

func (yy *YYZongziFeast) OnOpen() {
	yy.Broadcast(75, 75, &pb3.S2C_75_75{
		ActiveId:   yy.Id,
		PlayerData: &pb3.YYZongziFeastPlayerData{},
	})
}

func (yy *YYZongziFeast) NewDay() {
	data := yy.getThisData()
	data.PlayerData = nil
	yy.Broadcast(75, 75, &pb3.S2C_75_75{
		ActiveId:   yy.Id,
		PlayerData: &pb3.YYZongziFeastPlayerData{},
	})
}

func (yy *YYZongziFeast) PlayerLogin(player iface.IPlayer) {
	yy.s2cPlayerInfo(player)
}

func (yy *YYZongziFeast) PlayerReconnect(player iface.IPlayer) {
	yy.s2cPlayerInfo(player)
}

func (yy *YYZongziFeast) s2cPlayerInfo(player iface.IPlayer) {
	if nil == player {
		return
	}
	pData := yy.getPlayerData(player.GetId())
	player.SendProto3(75, 75, &pb3.S2C_75_75{
		ActiveId:   yy.Id,
		PlayerData: pData,
	})
}

func (yy *YYZongziFeast) SendItem(player iface.IPlayer, targetId uint64, itemId uint32) bool {
	conf := yy.GetConf()
	if nil == conf {
		return false
	}

	if _, ok := conf.AskItems[itemId]; !ok {
		return false
	}

	pData := yy.getPlayerData(player.GetId())

	if pData.DailyGivingCount >= conf.DailyGiving {
		return false
	}

	targetData := yy.getPlayerData(targetId)
	if targetData.DailyReceiveCount >= conf.DailyAsk {
		return false
	}

	if !player.ConsumeByConf(jsondata.ConsumeVec{
		&jsondata.Consume{
			Id:    itemId,
			Count: 1,
		},
	}, false, common.ConsumeParams{LogId: pb3.LogId_LogZongziFeatstGiving}) {
		return false
	}

	pData.DailyGivingCount++
	yy.s2cPlayerInfo(player)

	targetData.DailyReceiveCount++

	sendReward := jsondata.StdRewardVec{
		&jsondata.StdReward{
			Id:    itemId,
			Count: 1,
		},
	}

	target := manager.GetPlayerPtrById(targetId)

	if nil != target {
		yy.s2cPlayerInfo(target)
		engine.GiveRewards(target, sendReward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogZongziFeatstGiving})
	} else {
		mailmgr.SendMailToActor(targetId, &mailargs.SendMailSt{
			ConfId:  common.Mail_AskHelpGiftAwards,
			Rewards: sendReward,
			Content: &mailargs.CommonMailArgs{
				Str1: player.GetName(),
				Str2: jsondata.GetItemName(itemId),
			},
		})
	}

	return true
}

func (yy *YYZongziFeast) CanAskItem(playerId uint64, itemId uint32) bool {
	conf := yy.GetConf()
	if nil == conf {
		return false
	}

	if _, ok := conf.AskItems[itemId]; !ok {
		return false
	}

	pData := yy.getPlayerData(playerId)

	if pData.DailyReceiveCount >= conf.DailyAsk {
		return false
	}

	return true
}

func (yy *YYZongziFeast) getThisData() *pb3.YYZongziFeastData {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}

	if globalVar.YyDatas.ZongziFeastData == nil {
		globalVar.YyDatas.ZongziFeastData = make(map[uint32]*pb3.YYZongziFeastData)
	}

	tmp := globalVar.YyDatas.ZongziFeastData[yy.Id]
	if nil == tmp {
		tmp = new(pb3.YYZongziFeastData)
	}

	if nil == tmp.PlayerData {
		tmp.PlayerData = make(map[uint64]*pb3.YYZongziFeastPlayerData)
	}
	globalVar.YyDatas.ZongziFeastData[yy.Id] = tmp

	return tmp
}

func (yy *YYZongziFeast) getPlayerData(playerId uint64) *pb3.YYZongziFeastPlayerData {
	actData := yy.getThisData()
	if _, ok := actData.PlayerData[playerId]; !ok {
		actData.PlayerData[playerId] = new(pb3.YYZongziFeastPlayerData)
	}
	return actData.PlayerData[playerId]
}

func (yy *YYZongziFeast) GetConf() *jsondata.ZongziFeastConfig {
	return jsondata.GetYYZongziFeastConf(yy.ConfName, yy.ConfIdx)
}

func (yy *YYZongziFeast) c2sMake(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_75_76
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := yy.GetConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	if req.Count == 0 {
		return neterror.ParamsInvalidError("count is zero")
	}

	if !player.ConsumeRate(conf.Consume, int64(req.Count), false, common.ConsumeParams{LogId: pb3.LogId_LogZongziFeatstMakeConsume}) {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	rewards := jsondata.StdRewardMulti(conf.MakeItem, int64(req.Count))
	engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogZongziFeatstMakeAwards})

	player.SendShowRewardsPopByYY(rewards, yy.Id)

	player.SendProto3(75, 76, &pb3.S2C_75_76{
		ActiveId: yy.Id,
		Count:    req.Count,
	})
	return nil
}

func init() {
	yymgr.RegisterYYType(yydefine.YYZongziFeast, func() iface.IYunYing {
		return &YYZongziFeast{}
	})

	net.RegisterGlobalYYSysProto(75, 76, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYZongziFeast).c2sMake
	})
}
