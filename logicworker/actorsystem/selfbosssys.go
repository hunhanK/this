package actorsystem

/**
 * @Author: YangQibin
 * @Desc: 材料副本
 * @Date: 2023/3/21
 */

import (
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

type SelfBossFubenSys struct {
	Base
	data    *pb3.SelfBossData
	unFirst bool
}

func (sys *SelfBossFubenSys) init() bool {
	data := sys.GetBinaryData().SelfBossData
	if data == nil {
		data = &pb3.SelfBossData{
			AttackInfo: make(map[uint32]*pb3.SelfBossEntry),
		}

		sys.GetBinaryData().SelfBossData = data
	}

	sys.data = data

	if data.AttackInfo == nil {
		data.AttackInfo = make(map[uint32]*pb3.SelfBossEntry)
	}

	for id := range data.AttackInfo {
		conf := jsondata.GetSelfBossConf(id)
		if conf == nil {
			continue
		}

		if conf.MaxLevel < sys.owner.GetLevel() {
			delete(data.AttackInfo, id)
		}
	}

	return true
}

func (sys *SelfBossFubenSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *SelfBossFubenSys) OnOpen() {
	if !sys.init() {
		return
	}

	sys.c2sInfo(nil)
}

func (sys *SelfBossFubenSys) OnLogin() {
	sys.c2sInfo(nil)
}

func (sys *SelfBossFubenSys) OnReconnect() {
	sys.c2sInfo(nil)
}

func (sys *SelfBossFubenSys) onNewDay() {
	for _, mfbs := range sys.data.AttackInfo {
		mfbs.ChallengedTimes = 0
		mfbs.BuyedTimes = 0
	}
	sys.SendProto3(17, 120, &pb3.S2C_17_120{
		Data: sys.data,
	})
}

func (sys *SelfBossFubenSys) c2sInfo(msg *base.Message) error {
	sys.SendProto3(17, 120, &pb3.S2C_17_120{
		Data: sys.data,
	})
	return nil
}

func (sys *SelfBossFubenSys) c2sBuyChallengeTimes(msg *base.Message) error {
	var req pb3.C2S_17_123
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}
	conf := jsondata.GetSelfBossConf(req.Id)
	if conf == nil {
		return neterror.ParamsInvalidError("invalid id %d", req.Id)
	}

	if err := sys.checkLevelLimit(req.Id); err != nil {
		return err
	}

	attackInfo, ok := sys.data.AttackInfo[req.Id]
	if !ok {
		attackInfo = &pb3.SelfBossEntry{}
		sys.data.AttackInfo[req.Id] = attackInfo
	}

	vipPrivilegeTimes, err := sys.GetOwner().GetPrivilege(privilegedef.EnumSelfBossBuyTimes)
	if err != nil {
		return neterror.InternalError("get privilege times for SelfBossBuyTimes failed %s", err)
	}

	if attackInfo.BuyedTimes >= uint32(vipPrivilegeTimes)+conf.MaxNum {
		return neterror.ParamsInvalidError("vip privilege times limit")
	}

	if !sys.GetOwner().ConsumeByConf(conf.BuyConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogSelfBossVipPrivilegeTimesBuy}) {
		return neterror.ParamsInvalidError("consume failed")
	}

	attackInfo.BuyedTimes++

	sys.SendProto3(17, 123, &pb3.S2C_17_123{Id: req.Id})
	return nil
}

func (sys *SelfBossFubenSys) c2sAttack(msg *base.Message) error {
	var req pb3.C2S_17_121
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	conf := jsondata.GetSelfBossConf(req.Id)
	if nil == conf {
		return neterror.ParamsInvalidError("invalid id %d", req.Id)
	}

	if err := sys.checkLevelLimit(req.Id); err != nil {
		return err
	}

	attackInfo, ok := sys.data.AttackInfo[req.Id]
	if !ok {
		attackInfo = &pb3.SelfBossEntry{}
		sys.data.AttackInfo[req.Id] = attackInfo
	}

	if attackInfo.ChallengedTimes >= conf.Num+attackInfo.BuyedTimes {
		return neterror.ParamsInvalidError("challenge times execeded")
	}

	var consumes []*jsondata.Consume

	consumes = append(consumes, &jsondata.Consume{
		Type:       custom_id.ConsumeTypeItem,
		Id:         uint32(jsondata.GetSelfBossCommonConf().Ticket),
		Count:      conf.TicketNum,
		CanAutoBuy: false,
	})

	//TODO 如果组队状态下的话需要拦截

	err := sys.GetOwner().DoFirstExperience(pb3.ExperienceType_ExperienceTypeSelfBoss, func() error {
		sys.unFirst = true
		return nil
	})
	if err != nil {
		return neterror.InternalError("enter fight srv failed err %s", err)
	}

	err = sys.GetOwner().EnterFightSrv(base.LocalFightServer, fubendef.EnterSelfBoss,
		&pb3.EnterSelfBossReq{
			Id: req.Id,
		},
		&argsdef.ConsumesSt{
			Consumes: consumes,
			LogId:    pb3.LogId_LogSelfBossAttack,
		})

	if err != nil {
		return neterror.InternalError("enter fight srv failed err %s", err)
	}

	sys.onAttack()

	return nil
}

func (sys *SelfBossFubenSys) onAttack() {
	sys.GetOwner().TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionAttackSelfBoss)
}

func (sys *SelfBossFubenSys) checkLevelLimit(id uint32) error {
	conf := jsondata.GetSelfBossConf(id)
	if conf == nil {
		return neterror.ParamsInvalidError("invalid id %d", id)
	}

	if conf.Level > sys.owner.GetLevel() {
		return neterror.ParamsInvalidError("level not enough")
	}

	if conf.MaxLevel < sys.owner.GetLevel() && conf.MaxLevel != 0 {
		return neterror.ParamsInvalidError("level too high")
	}

	return nil
}

func (sys *SelfBossFubenSys) c2sQuickAttack(msg *base.Message) error {
	var req pb3.C2S_17_122
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	conf := jsondata.GetSelfBossConf(req.Id)
	if conf == nil {
		return neterror.ParamsInvalidError("invalid id %d", req.Id)
	}

	if err := sys.checkLevelLimit(req.Id); err != nil {
		return err
	}

	attackInfo, ok := sys.data.AttackInfo[req.Id]
	if !ok {
		attackInfo = &pb3.SelfBossEntry{}
		sys.data.AttackInfo[req.Id] = attackInfo
	}

	if !attackInfo.Passed {
		return neterror.ParamsInvalidError("not passed")
	}

	if attackInfo.ChallengedTimes >= conf.Num+attackInfo.BuyedTimes {
		return neterror.ParamsInvalidError("challenge times execeded")
	}

	consumes := []*jsondata.Consume{}
	consumes = append(consumes, &jsondata.Consume{
		Type:  custom_id.ConsumeTypeItem,
		Id:    uint32(jsondata.GetSelfBossCommonConf().Ticket),
		Count: conf.TicketNum,
	})

	if !sys.GetOwner().ConsumeByConf(consumes, true, common.ConsumeParams{LogId: pb3.LogId_LogSelfBossAttack}) {
		return neterror.ParamsInvalidError("ticket not enough")
	}

	monsterConf := jsondata.GetMonsterConf(conf.BossId)
	if monsterConf == nil {
		return neterror.ParamsInvalidError("invalid monster id %d", conf.BossId)
	}

	attackInfo.ChallengedTimes++
	sys.onAttack()

	sys.GetOwner().CallActorFunc(actorfuncid.ReqSelfBossQuickAttackReward, &pb3.ReqSelfBossQuickAttackReward{
		ConfId: req.Id,
	})

	commonConf := jsondata.GetSelfBossCommonConf()
	sys.GetOwner().TriggerQuestEvent(custom_id.QttPassFbTimes, commonConf.FubenId, 1)
	sys.GetOwner().TriggerQuestEvent(custom_id.QttAchievementsPassFbTimes, commonConf.FubenId, 1)
	sys.GetOwner().TriggerQuestEvent(custom_id.QttAchievementsEnterFbTimes, commonConf.FubenId, 1)
	sys.GetOwner().TriggerQuestEvent(custom_id.QttMonsterSupType, monsterConf.SubType, 1)
	sys.GetOwner().TriggerEvent(custom_id.AeQuickAttackKillMon, monsterConf.Id, conf.SceneId, uint32(1), commonConf.FubenId)
	sys.GetOwner().FinishBossQuickAttack(monsterConf.Id, conf.SceneId, commonConf.FubenId)

	return nil
}

func (sys *SelfBossFubenSys) syncQuickAttackLootItem(reward jsondata.StdRewardVec, sceneId uint32, monsterId uint32) {
	for _, foo := range reward {
		itemConf := jsondata.GetItemConfig(foo.Id)
		if itemConf == nil {
			continue
		}

		if itemConf.IsRare >= itemdef.ItemIsRare_Record {
			SetBossLootRecord(&pb3.CommonRecord{
				Id: sys.GetOwner().GetId(),
				Params: []string{
					sys.GetOwner().GetName(),
					utils.I32toa(sceneId),
					utils.I32toa(monsterId),
					utils.I32toa(foo.Id),
					utils.I32toa(uint32(time.Now().Unix())),
				},
			}, itemConf.IsRare)
		}
	}
}

func (sys *SelfBossFubenSys) checkOut(settle *pb3.FbSettlement) {
	if len(settle.ExData) < 1 {
		sys.LogError("len of ExData is not correct fubenId %d", settle.FbId)
	}

	confId := settle.ExData[0]
	conf := jsondata.GetSelfBossConf(confId)
	if conf == nil {
		sys.LogError("invalid conf id %d", confId)
		return
	}

	res := &pb3.S2C_17_254{
		Settle: settle,
	}

	attackInfo, ok := sys.data.AttackInfo[confId]

	if !ok {
		attackInfo = &pb3.SelfBossEntry{}
		sys.data.AttackInfo[confId] = attackInfo
	}

	if settle.Ret == 2 {
		if sys.unFirst {
			attackInfo.ChallengedTimes++
		}
		attackInfo.Passed = true

		logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogPassSelfBoss, &pb3.LogPlayerCounter{
			NumArgs: uint64(attackInfo.ChallengedTimes),
		})
	}

	sys.SendProto3(17, 254, res)

	if mConf := jsondata.GetMonsterConf(conf.BossId); nil != mConf {
		sys.owner.TriggerQuestEventByRange(custom_id.QttSelfBossQuality, mConf.Quality, 0, custom_id.QTYPE_ADD)
	}
}

func checkOutSelfBossFubenSys(player iface.IPlayer, buf []byte) {
	var req pb3.FbSettlement
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("unmarshal failed %s", err)
		return
	}

	selfBossFubenSys, ok := player.GetSysObj(sysdef.SiSelfBoss).(*SelfBossFubenSys)
	if !ok || !selfBossFubenSys.IsOpen() {
		return
	}

	selfBossFubenSys.checkOut(&req)
}

func gmEnterSelfBoss(player iface.IPlayer, args ...string) bool {
	if len(args) != 1 {
		return false
	}

	id := utils.AtoUint32(args[0])

	selfBossFubenSys, ok := player.GetSysObj(sysdef.SiSelfBoss).(*SelfBossFubenSys)
	if !ok || !selfBossFubenSys.IsOpen() {
		return ok
	}

	msg := base.NewMessage()
	msg.PackPb3Msg(&pb3.C2S_17_121{
		Id: id,
	})

	if err := selfBossFubenSys.c2sAttack(msg); err != nil {
		player.LogError("gmEnterSelfBoss failed %s", err)
		return false
	}

	return true
}

func onLootSelfBossRecord(actor iface.IPlayer, buf []byte) {
	var drop pb3.FActorLootItemSt
	if err := pb3.Unmarshal(buf, &drop); nil != err {
		actor.LogError("wild boss loot record error:%v", err)
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

func onResSelfBossQuickAttackReward(player iface.IPlayer, buf []byte) {
	var res pb3.ResSelfBossQuickAttackReward
	if err := pb3.Unmarshal(buf, &res); err != nil {
		player.LogError("unmarshal failed %s", err)
		return
	}

	rewards := jsondata.Pb3RewardVecToStdRewardVec(res.Rewards)

	state := engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogSelfBossAttack,
		NoTips: true,
	})

	if !state {
		player.LogError("give rewards failed")
		return
	}

	conf := jsondata.GetSelfBossConf(res.ConfId)
	if conf == nil {
		player.LogError("invalid conf id %d", res.ConfId)
		return
	}

	selfBossFubenSys, ok := player.GetSysObj(sysdef.SiSelfBoss).(*SelfBossFubenSys)
	if !ok || !selfBossFubenSys.IsOpen() {
		player.LogError("selfBossFubenSys is not open")
		return
	}

	selfBossFubenSys.SendProto3(17, 122, &pb3.S2C_17_122{
		Id: res.ConfId,
	})

	selfBossFubenSys.SendProto3(17, 254, &pb3.S2C_17_254{
		Settle: &pb3.FbSettlement{
			FbId:      jsondata.GetSelfBossCommonConf().FubenId,
			Ret:       2,
			ExData:    []uint32{conf.Id},
			ShowAward: res.Rewards,
			IsQuick:   true,
		},
	})

	if mConf := jsondata.GetMonsterConf(conf.BossId); nil != mConf {
		selfBossFubenSys.owner.TriggerQuestEventByRange(custom_id.QttSelfBossQuality, mConf.Quality, 0, custom_id.QTYPE_ADD)
	}

	selfBossFubenSys.syncQuickAttackLootItem(rewards, conf.SceneId, conf.BossId)
}

func init() {
	RegisterSysClass(sysdef.SiSelfBoss, func() iface.ISystem {
		return &SelfBossFubenSys{}
	})

	net.RegisterSysProto(17, 120, sysdef.SiSelfBoss, (*SelfBossFubenSys).c2sInfo)
	net.RegisterSysProto(17, 121, sysdef.SiSelfBoss, (*SelfBossFubenSys).c2sAttack)
	net.RegisterSysProto(17, 123, sysdef.SiSelfBoss, (*SelfBossFubenSys).c2sBuyChallengeTimes)
	net.RegisterSysProto(17, 122, sysdef.SiSelfBoss, (*SelfBossFubenSys).c2sQuickAttack)

	gmevent.Register("enterSelfBoss", gmEnterSelfBoss, 0)
	engine.RegisterActorCallFunc(playerfuncid.CheckOutSelfBossFuben, checkOutSelfBossFubenSys)
	engine.RegisterActorCallFunc(playerfuncid.SyncSelfBossLootRecord, onLootSelfBossRecord)

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		selfBossFubenSys, ok := actor.GetSysObj(sysdef.SiSelfBoss).(*SelfBossFubenSys)
		if !ok || !selfBossFubenSys.IsOpen() {
			return
		}

		selfBossFubenSys.onNewDay()
	})

	engine.RegisterActorCallFunc(playerfuncid.ResSelfBossQuickAttackReward, onResSelfBossQuickAttackReward)
}
