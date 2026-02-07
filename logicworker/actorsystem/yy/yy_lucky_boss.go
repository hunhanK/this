/**
 * @Author: zjj
 * @Date: 2024/7/18
 * @Desc: 福运BOSS
**/

package yy

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"time"
)

type YYLuckyBoss struct {
	YYBase
	startTimer   *time_util.Timer // 同一个活动同一时刻只会有一批boss
	endTimer     *time_util.Timer //
	MonsterLvMap map[uint32]uint32
}

func (b *YYLuckyBoss) ResetData() {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if globalVar.YyDatas.LuckyBossData == nil {
		globalVar.YyDatas.LuckyBossData = make(map[uint32]*pb3.YYLuckyBossData)
	}
	delete(globalVar.YyDatas.LuckyBossData, b.Id)
}

func (b *YYLuckyBoss) getData() *pb3.YYLuckyBossData {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if globalVar.YyDatas.LuckyBossData == nil {
		globalVar.YyDatas.LuckyBossData = make(map[uint32]*pb3.YYLuckyBossData)
	}
	if globalVar.YyDatas.LuckyBossData[b.Id] == nil {
		globalVar.YyDatas.LuckyBossData[b.Id] = &pb3.YYLuckyBossData{}
	}
	return globalVar.YyDatas.LuckyBossData[b.Id]
}

func (b *YYLuckyBoss) fbStartTimer(hour, minutes uint32) {
	if b.startTimer != nil {
		b.startTimer.Stop()
	}
	b.startTimer = timer.SetTimeout(time.Duration(minutes)*time.Minute, func() {
		err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FCreateYYLuckyBoss, &pb3.CreateYYLuckyBossSt{
			ConfIdx:  b.ConfIdx,
			ConfName: b.ConfName,
		})
		if err != nil {
			b.LogError("err:%v", err)
		}
		b.getData().CurHour = hour
		engine.Broadcast(chatdef.CIWorld, 0, 69, 121, &pb3.S2C_69_121{
			ActiveId: b.Id,
			Hour:     hour,
		}, 0)
		engine.BroadcastTipMsgById(tipmsgid.YYGoodLuckBossTips1)
	})
}

func (b *YYLuckyBoss) ServerStopSaveData() {
	b.getData().CurHour = 0
}

func (b *YYLuckyBoss) fbEndTimer(hour, duration uint32) {
	if b.endTimer != nil {
		b.endTimer.Stop()
	}
	b.endTimer = timer.SetTimeout(time.Duration(duration)*time.Minute, func() {
		err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FDestroyYYLuckyBoss, &pb3.DestroyYYLuckyBossSt{
			ConfIdx:  b.ConfIdx,
			ConfName: b.ConfName,
		})
		if err != nil {
			b.LogError("err:%v", err)
		}
		b.getData().CurHour = 0
		engine.Broadcast(chatdef.CIWorld, 0, 69, 122, &pb3.S2C_69_122{
			ActiveId: b.Id,
			Hour:     hour,
		}, 0)
	})
}

func (b *YYLuckyBoss) handleHourArrive(hour int32) {
	conf, ok := jsondata.GetYYLuckyBossConf(b.ConfName, b.ConfIdx)
	if !ok {
		b.LogError("conf not found")
		return
	}
	for _, timeConf := range conf.TimeConf {
		if uint32(hour) != timeConf.Hour {
			continue
		}
		b.fbStartTimer(uint32(hour), timeConf.Minutes)
		b.fbEndTimer(uint32(hour), timeConf.Minutes+timeConf.Duration)
		break
	}
}

func (b *YYLuckyBoss) sendRecords(player iface.IPlayer) {
	var rsp = &pb3.S2C_69_124{
		ActiveId:          b.Id,
		Records:           b.getData().Records,
		PlayerBaseInfoMap: make(map[uint64]*pb3.PlayerDataBase),
		CurHour:           b.getData().CurHour,
	}
	for _, record := range rsp.Records {
		_, ok := rsp.PlayerBaseInfoMap[record.ActorId]
		if ok {
			continue
		}
		rsp.PlayerBaseInfoMap[record.ActorId] = manager.GetData(record.ActorId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	}
	if player == nil {
		engine.Broadcast(chatdef.CIWorld, 0, 69, 124, rsp, 0)
		return
	}
	player.SendProto3(69, 124, rsp)
}

func (b *YYLuckyBoss) handleGiveYYLuckyBossLuckyAwards(player iface.IPlayer, buf []byte) {
	var req pb3.F2GGiveYYLuckyBossLuckyAwards
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}
	if b.ConfName != req.ConfName || b.ConfIdx != req.ConfIdx {
		return
	}

	// 下发奖励
	stdRewardVec := jsondata.Pb3RewardVecToStdRewardVec(req.Awards)
	stdRewardVec = engine.FilterRewardByPlayer(player, stdRewardVec)

	// 幸运奖
	if req.IsHitLuckyAwards {
		data := b.getData()
		var newRecord = &pb3.YYLuckyBossLuckyAwardsRecord{
			ActorId:   player.GetId(),
			CreatedAt: time_util.NowSec(),
			Awards:    jsondata.StdRewardVecToPb3RewardVec(stdRewardVec),
		}
		data.Records = append(data.Records, newRecord)
		// 如果超过100 就把开头的元素移除 只保留100条
		if len(data.Records) > 100 {
			data.Records = data.Records[1:]
		}
	}

	engine.GiveRewards(player, stdRewardVec, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLuckyBossGiveAwards})
	player.SendProto3(69, 123, &pb3.S2C_69_123{
		ActiveId:         b.Id,
		Awards:           jsondata.StdRewardVecToPb3RewardVec(stdRewardVec),
		IsHitLuckyAwards: req.IsHitLuckyAwards,
	})
}

func (b *YYLuckyBoss) getRecords(player iface.IPlayer, _ *base.Message) error {
	b.sendRecords(player)
	return nil
}

func (b *YYLuckyBoss) getMonsterLv(player iface.IPlayer, _ *base.Message) error {
	b.sendMonsterLv(player)
	return nil
}

func (b *YYLuckyBoss) sendMonsterLv(player iface.IPlayer) {
	var rsp = &pb3.S2C_69_125{
		ActiveId:     b.Id,
		MonsterLvMap: b.MonsterLvMap,
	}
	if player == nil {
		engine.Broadcast(chatdef.CIWorld, 0, 69, 125, rsp, 0)
		return
	}
	player.SendProto3(69, 125, rsp)
}

func rangeAllYYLuckBoss(doLogic func(ying iface.IYunYing)) {
	allYY := yymgr.GetAllYY(yydefine.YYLuckyBoss)
	for _, v := range allYY {
		if !v.IsOpen() {
			continue
		}
		utils.ProtectRun(func() {
			doLogic(v)
		})
	}
}

func enterYYLuckyBossFb(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_120
	if err := msg.UnpackagePbmsg(&req); nil != err {
		return neterror.ParamsInvalidError("enterFb error:%v", err)
	}
	err := player.EnterFightSrv(base.LocalFightServer, fubendef.EnterYYLuckyBoss, &pb3.EnterFubenHdl{
		SceneId: req.SceneId,
		PosX:    int32(req.X),
		PosY:    int32(req.Y),
	})
	if err != nil {
		player.LogError("err:%v", err)
		return err
	}
	return nil
}

func handleF2GSyncYYLuckyBossMonsterLv(buf []byte) {
	rangeAllYYLuckBoss(func(v iface.IYunYing) {
		var req pb3.F2GYYLuckyBossMonsterLv
		if err := pb3.Unmarshal(buf, &req); nil != err {
			return
		}
		yyLuckyBossSys := v.(*YYLuckyBoss)
		if yyLuckyBossSys.ConfName != req.ConfName || yyLuckyBossSys.ConfIdx != req.ConfIdx {
			return
		}
		yyLuckyBossSys.MonsterLvMap = req.MonsterLvMap
		yyLuckyBossSys.sendMonsterLv(nil)
	})
}

func init() {
	yymgr.RegisterYYType(yydefine.YYLuckyBoss, func() iface.IYunYing {
		return &YYLuckyBoss{}
	})

	net.RegisterProto(69, 120, enterYYLuckyBossFb)
	net.RegisterGlobalYYSysProto(69, 124, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYLuckyBoss).getRecords
	})
	net.RegisterGlobalYYSysProto(69, 125, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYLuckyBoss).getMonsterLv
	})

	event.RegSysEvent(custom_id.SeHourArrive, func(args ...interface{}) {
		hour, ok := args[0].(int)
		if !ok {
			logger.LogStack("hour convert failed")
			return
		}
		rangeAllYYLuckBoss(func(v iface.IYunYing) {
			v.(*YYLuckyBoss).handleHourArrive(int32(hour))
		})
	})

	engine.RegisterActorCallFunc(playerfuncid.F2GGiveYYLuckyBossLuckyAwards, func(player iface.IPlayer, buf []byte) {
		rangeAllYYLuckBoss(func(v iface.IYunYing) {
			v.(*YYLuckyBoss).handleGiveYYLuckyBossLuckyAwards(player, buf)
		})
	})

	engine.RegisterSysCall(sysfuncid.F2GSyncYYLuckyBossMonsterLv, handleF2GSyncYYLuckyBossMonsterLv)

	gmevent.Register("YYLuckyBoss.CreateMonster", func(player iface.IPlayer, args ...string) bool {
		rangeAllYYLuckBoss(func(v iface.IYunYing) {
			v.(*YYLuckyBoss).handleHourArrive(int32(utils.AtoUint32(args[0])))
		})
		return true
	}, 1)
	gmevent.Register("YYLuckyBoss.CreateMonsterV2", func(player iface.IPlayer, args ...string) bool {
		rangeAllYYLuckBoss(func(v iface.IYunYing) {
			b := v.(*YYLuckyBoss)
			conf, ok := jsondata.GetYYLuckyBossConf(b.ConfName, b.ConfIdx)
			hour := utils.AtoUint32(args[0])
			if !ok {
				b.LogError("conf not found")
				return
			}
			for _, timeConf := range conf.TimeConf {
				if hour != timeConf.Hour {
					continue
				}
				err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FCreateYYLuckyBoss, &pb3.CreateYYLuckyBossSt{
					ConfIdx:  b.ConfIdx,
					ConfName: b.ConfName,
				})
				if err != nil {
					b.LogError("err:%v", err)
				}
				b.getData().CurHour = hour
				engine.Broadcast(chatdef.CIWorld, 0, 69, 121, &pb3.S2C_69_121{
					ActiveId: b.Id,
					Hour:     hour,
				}, 0)
				engine.BroadcastTipMsgById(tipmsgid.YYGoodLuckBossTips1)
				break
			}
		})
		return true
	}, 1)

	gmevent.Register("YYLuckyBoss.clearMonster", func(player iface.IPlayer, args ...string) bool {
		rangeAllYYLuckBoss(func(v iface.IYunYing) {
			v.(*YYLuckyBoss).fbEndTimer(utils.AtoUint32(args[0]), 0)
		})
		return true
	}, 1)
}
