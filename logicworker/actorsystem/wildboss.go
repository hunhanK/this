package actorsystem

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type WildBossSys struct {
	Base
	*miscitem.BossTipContainer
}

func (sys *WildBossSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.RemindRareBossId {
		binary.RemindRareBossId = make([]uint32, 0)
	}
	sys.BossTipContainer = miscitem.NewBossTipContainer(&binary.RemindRareBossId)
}

func (sys *WildBossSys) SendRareInfo() {
	if localReq, err := base.MakeMessage(17, 151, &pb3.C2S_17_151{}); nil == err {
		sys.owner.DoNetMsg(17, 151, localReq)
	} else {
		sys.LogError("本服野外boss信息发送出错:%v", err)
	}
	if crossReq, err := base.MakeMessage(17, 152, &pb3.C2S_17_152{}); nil == err {
		sys.owner.DoNetMsg(17, 152, crossReq)
	} else {
		sys.LogError("跨服野外boss信息发送出错:%v", err)
	}
}

func (sys *WildBossSys) OnOpen() {
	sys.SendRareInfo()
	sys.SendRareBossRemind()
}

func (sys *WildBossSys) OnAfterLogin() {
	sys.SendRareInfo()
	sys.SendRareBossRemind()
}

func (sys *WildBossSys) OnReconnect() {
	sys.SendRareInfo()
	sys.SendRareBossRemind()
}

func (sys *WildBossSys) SendRareBossRemind() {
	binary := sys.GetBinaryData()
	sys.owner.SendProto3(17, 150, &pb3.S2C_17_150{BossIds: binary.GetRemindRareBossId()})
}

// 请求进入副本
func (sys *WildBossSys) c2sEnterFb(msg *base.Message) error {
	var req pb3.C2S_17_41
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}
	lv := sys.owner.GetLevel()
	circle := sys.owner.GetCircle()
	conf := jsondata.GetWildBossBarrierConf(req.GetId())
	if nil == conf {
		return neterror.ConfNotFoundError("wild boss conf is nil")
	}
	confList := jsondata.GetWildBossConf()
	for sceneId, v := range confList {
		if conf.SecondaryType == v.SecondaryType && v.SecondaryId == req.SecondaryId {
			if (v.SecondaryLevel > 0 && v.SecondaryLevel > lv) || (v.SecondaryCircle > 0 && v.SecondaryCircle > circle) {
				sys.owner.SendTipMsg(tipmsgid.TpLevelNotReach)
				return nil
			}
			if v.IsCross > 0 {
				sys.owner.EnterFightSrv(base.SmallCrossServer, fubendef.EnterCrossRareBoss, &pb3.EnterWildBoss{SenceId: sceneId})
			} else {
				sys.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterWildBoss, &pb3.EnterWildBoss{SenceId: sceneId})
			}
			return nil
		}
	}
	return neterror.ConfNotFoundError("wild boss conf is nil")
}

func c2sRecord(player iface.IPlayer, msg *base.Message) error {
	rsp := &pb3.S2C_17_40{
		BossLootRecord:     gshare.GetStaticVar().GetBossLootRecord(),
		BossRareLootRecord: gshare.GetStaticVar().GetBossRareLootRecord(),
	}
	player.SendProto3(17, 40, rsp)
	return nil
}

func (sys *WildBossSys) c2sRemindRareBoss(msg *base.Message) error {
	req := &pb3.C2S_17_150{}
	if err := msg.UnPackPb3Msg(req); nil != err {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}
	if !jsondata.IsRemindBoss(req.GetBossId()) {
		return neterror.ParamsInvalidError("boss(%d) not rare boss type", req.GetBossId())
	}
	sys.ChangeTip(req.GetBossId(), req.GetNeed())
	sys.SendRareBossRemind()
	return nil
}

func onLootRecord(actor iface.IPlayer, buf []byte) {
	var drop pb3.FActorLootItemSt
	if err := pb3.Unmarshal(buf, &drop); nil != err {
		actor.LogError("self boss loot record error:%v", err)
		return
	}

	itemConf := jsondata.GetItemConfig(drop.ItemId)
	if itemConf == nil {
		return
	}

	if itemConf.IsRare >= itemdef.ItemIsRare_Record {
		SetBossLootRecord(&pb3.CommonRecord{
			Id: actor.GetId(),
			Params: []string{
				actor.GetName(),
				utils.I32toa(drop.GetSceneId()),
				utils.I32toa(drop.GetMonId()),
				utils.I32toa(drop.GetItemId()),
				utils.I32toa(drop.GetTime()),
			},
		}, itemConf.IsRare)
	}
}

func SetBossLootRecord(record *pb3.CommonRecord, isRareType uint32) {
	globalVar := gshare.GetStaticVar()

	mxRecord := jsondata.GlobalUint("bossjilumax")
	if isRareType == itemdef.ItemIsRare_Record {
		globalVar.BossLootRecord = append(globalVar.BossLootRecord, record)
		if uint32(len(globalVar.BossLootRecord)) > mxRecord {
			globalVar.BossLootRecord = globalVar.BossLootRecord[1:]
		}
		return
	}

	maxRareRecord := jsondata.GlobalUint("bossTopjilumax")
	if isRareType == itemdef.ItemIsRare_Rare {
		globalVar.BossRareLootRecord = append(globalVar.BossRareLootRecord, record)
		if uint32(len(globalVar.BossRareLootRecord)) > maxRareRecord {
			globalVar.BossRareLootRecord = globalVar.BossRareLootRecord[1:]
		}
		return
	}

	logger.LogError("SetBossLootRecord error isRareType:%d", isRareType)
}

func killOneRareBoss(actor iface.IPlayer, buf []byte) {
	actor.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionKillRareBoss)
}

func killOneWildBoss(actor iface.IPlayer, buf []byte) {
	actor.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionKillWildBoss)
}

func init() {
	RegisterSysClass(sysdef.SiWildBoss, func() iface.ISystem {
		return &WildBossSys{}
	})

	engine.RegisterActorCallFunc(playerfuncid.SyncCommonBossLootRecord, onLootRecord)

	net.RegisterSysProto(17, 41, sysdef.SiWildBoss, (*WildBossSys).c2sEnterFb)
	net.RegisterProto(17, 40, c2sRecord)

	net.RegisterSysProto(17, 150, sysdef.SiWildBoss, (*WildBossSys).c2sRemindRareBoss)

	engine.RegisterActorCallFunc(playerfuncid.KillOneRareBoss, killOneRareBoss)
	engine.RegisterActorCallFunc(playerfuncid.KillOneWildBoss, killOneWildBoss)
}
