/**
 * @Author: LvYuMeng
 * @Date: 2025/5/6
 * @Desc: 开服预告
**/

package yy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/net"
)

type YYOpenPreview struct {
	YYBase
}

func (yy *YYOpenPreview) OnInit() {
}

func (yy *YYOpenPreview) OnOpen() {
	yy.Broadcast(75, 70, &pb3.S2C_75_70{
		ActiveId: yy.Id,
	})
}

func (yy *YYOpenPreview) PlayerLogin(player iface.IPlayer) {
	yy.sendPlayerData(player)
}

func (yy *YYOpenPreview) PlayerReconnect(player iface.IPlayer) {
	yy.sendPlayerData(player)
}

func (yy *YYOpenPreview) GetConf() *jsondata.YYOpenPreviewConfig {
	return jsondata.GetYYOpenPreviewConfig(yy.ConfName, yy.ConfIdx)
}

func (yy *YYOpenPreview) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.OpenPreviewInfo {
		return
	}
	delete(globalVar.YyDatas.OpenPreviewInfo, yy.GetId())
}

func (yy *YYOpenPreview) getThisData() *pb3.YYOpenPreviewInfo {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}

	if globalVar.YyDatas.OpenPreviewInfo == nil {
		globalVar.YyDatas.OpenPreviewInfo = make(map[uint32]*pb3.YYOpenPreviewInfo)
	}

	tmp := globalVar.YyDatas.OpenPreviewInfo[yy.Id]
	if nil == tmp {
		tmp = &pb3.YYOpenPreviewInfo{
			PlayerData: make(map[uint64]*pb3.YYOpenPreviewPlayerData),
		}
		globalVar.YyDatas.OpenPreviewInfo[yy.Id] = tmp
	}

	return tmp
}

func (yy *YYOpenPreview) getPlayerData(player iface.IPlayer) *pb3.YYOpenPreviewPlayerData {
	actData := yy.getThisData()
	playerId := player.GetId()

	playerData, ok := actData.PlayerData[playerId]
	if !ok {
		playerData = &pb3.YYOpenPreviewPlayerData{
			RevIds: make(map[uint32]uint32),
		}
		actData.PlayerData[playerId] = playerData
	}

	if playerData.RevIds == nil {
		playerData.RevIds = make(map[uint32]uint32)
	}

	return playerData
}

// sendPlayerData 下发玩家数据
func (yy *YYOpenPreview) sendPlayerData(player iface.IPlayer) {
	playerData := yy.getPlayerData(player)
	msg := &pb3.S2C_75_70{
		ActiveId:   yy.Id,
		PlayerData: playerData,
	}
	player.SendProto3(75, 70, msg)
}

// receiveReward 领取奖励
func (yy *YYOpenPreview) receiveReward(player iface.IPlayer, id uint32) error {
	conf := yy.GetConf()
	if conf == nil {
		return neterror.ParamsInvalidError("config not exist")
	}

	benefit, ok := conf.Benefit[id]
	if !ok {
		return neterror.ParamsInvalidError("benefit not exist")
	}

	playerData := yy.getPlayerData(player)

	receiveTimes := playerData.RevIds[id]
	if receiveTimes >= benefit.ReceiveLimit {
		return neterror.ParamsInvalidError("receive limit")
	}

	nowSec := time_util.NowSec()
	if playerData.LastReceiveTime != 0 && time_util.IsSameDay(playerData.LastReceiveTime, nowSec) {
		return neterror.ParamsInvalidError("receive is receive today")
	}

	// 添加已领取ID
	playerData.RevIds[id]++
	playerData.LastReceiveTime = nowSec

	engine.GiveRewards(player, benefit.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogYYOpenPreviewReward})

	msg := &pb3.S2C_75_71{
		ActiveId:         yy.Id,
		Id:               id,
		Count:            playerData.RevIds[id],
		LastReceiveTimes: nowSec,
	}
	player.SendProto3(75, 71, msg)

	return nil
}

func (yy *YYOpenPreview) c2sReceiveReward(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_75_71
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	return yy.receiveReward(player, req.Id)
}

func init() {
	yymgr.RegisterYYType(yydefine.YYOpenPreview, func() iface.IYunYing {
		return &YYOpenPreview{}
	})

	net.RegisterGlobalYYSysProto(75, 71, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYOpenPreview).c2sReceiveReward
	})
}
