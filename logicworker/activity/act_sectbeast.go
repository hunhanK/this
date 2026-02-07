/**
 * @Author: lzp
 * @Date: 2023/12/7
 * @Desc: 宗门灵兽
**/

package activity

import (
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/auction"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/auctionmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"

	"github.com/gzjjyz/logger"
)

var (
	sectActorInfo map[uint64]uint64 // k:玩家id	v:伤害
	totalDamage   uint64            // 宗门伤害
	sectActEnd    bool              // 活动结束
)

func GetActorDamage(actorId uint64) uint64 {
	return sectActorInfo[actorId]
}

func GetSectDamage() uint64 {
	return totalDamage
}

func GetSeatBeastData() *pb3.SectBeast {
	if gshare.GetStaticVar().SectBeast == nil {
		gshare.GetStaticVar().SectBeast = &pb3.SectBeast{}
	}
	sectBeast := gshare.GetStaticVar().SectBeast
	if sectBeast.ExpLv == nil {
		sectBeast.ExpLv = &pb3.ExpLvSt{Lv: 1}
	}

	return sectBeast
}

func GetSectBeastExpLv() *pb3.ExpLvSt {
	data := GetSeatBeastData()
	return data.ExpLv
}

func AddBeastExp(exp uint64) {
	gData := GetSeatBeastData()
	oldLv := gData.ExpLv.Lv

	gData.ExpLv.Exp += exp
	lvConf := jsondata.GetSectBeastBossLvConf(gData.ExpLv.Lv)

	for lvConf != nil && gData.ExpLv.Exp >= lvConf.NeedExp {
		if jsondata.GetSectBeastBossLvConf(gData.ExpLv.Lv+1) == nil {
			break
		}
		gData.ExpLv.Exp -= lvConf.NeedExp
		gData.ExpLv.Lv += 1
		lvConf = jsondata.GetSectBeastBossLvConf(gData.ExpLv.Lv)
	}

	if gData.ExpLv.Lv > oldLv {
		engine.BroadcastTipMsgById(tipmsgid.SectBeastMonsterUpgrade, gData.ExpLv.Lv)
		syncBeastExpLvToFight(gData)
	}
}

// GetCrossRank 获取跨服排行榜
func GetCrossRank(playerId uint64) {
	if sectActEnd {
		return
	}
	if engine.FightClientExistPredicate(base.SmallCrossServer) {
		err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSectBeastRankReq, &pb3.G2CSectBeastRankReq{
			ActorId: playerId,
			PfId:    engine.GetPfId(),
			SrvId:   engine.GetServerId(),
		})
		if err != nil {
			logger.LogError("G2CSectBeastRank call err: %v", err)
		}
	}
}

func syncBeastExpLvToFight(sectBeast *pb3.SectBeast) {
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FSectBeastInfo, &pb3.CommonSt{
		U64Param: sectBeast.ExpLv.Exp,
		U32Param: sectBeast.ExpLv.Lv,
	})
	if err != nil {
		logger.LogError("G2FSectBeastInfo err: %v", err)
	}
}

func onSyncSectDamage(buf []byte) {
	var req pb3.CommonSt
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("onSyncSectDamage Unmarshal failed err: %s", err)
		return
	}

	actorId := req.U64Param
	damage := req.U64Param2

	player := manager.GetPlayerPtrById(actorId)
	if player == nil {
		logger.LogError("onSyncSectDamage player not found playerId=%d", actorId)
		return
	}

	sectActorInfo[actorId] += damage
	totalDamage += damage

	msg := &pb3.G2CSectBeastDamage{
		ActorId:        player.GetId(),
		Name:           player.GetName(),
		Career:         player.GetJob(),
		SmallCrossCamp: uint32(player.GetSmallCrossCamp()),
		Damage:         damage,
		PfId:           engine.GetPfId(),
		SrvId:          engine.GetServerId(),
	}

	// 连接上跨服，需要同步到跨服服务器上
	if engine.FightClientExistPredicate(base.SmallCrossServer) {
		if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSectBeastDamage, msg); err != nil {
			logger.LogError("G2CSectBeastDamage call err: %v", err)
		}
	}
}

func onSectBeastBossKill(buf []byte) {
	var req pb3.CommonSt
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("onSyncSectDamage Unmarshal failed err: %s", err)
		return
	}

	monId := req.U32Param
	mConf := jsondata.GetSectBeastMonsterConf(monId)
	if mConf == nil {
		return
	}

	var dConf *jsondata.SectBeastDropAward
	level := gshare.GetTopLevel()
	for _, v := range mConf.DropAward {
		tmpConf := v
		if level <= v.Level {
			dConf = tmpConf
			break
		}
	}

	if dConf == nil {
		dConf = mConf.DropAward[len(mConf.DropAward)-1]
	}

	var rewards jsondata.StdRewardVec
	for _, v := range dConf.Awards {
		if v.Weight > 0 && !random.Hit(v.Weight, 10000) {
			continue
		}
		rewards = append(rewards, v)
	}

	// 加入拍卖
	for _, reward := range rewards {
		goods, err := auctionmgr.AuctionMgrInstance.GenerateGoods(&auction.SoldInfo{
			ItemId:         reward.Id,
			Count:          uint32(reward.Count),
			RelationBossId: monId,
			SoldType:       auction.SoldTypeSys,
		})
		if err != nil {
			logger.LogError("err: %v", err)
			continue
		}
		err = auctionmgr.AuctionMgrInstance.PutIntoAuction(goods, 0)
		if err != nil {
			logger.LogError("err: %v", err)
			continue
		}
	}
}

func onSectBeastCrossRank(buf []byte) {
	var msg pb3.S2C_31_150
	if err := pb3.Unmarshal(buf, &msg); err != nil {
		logger.LogError("onSectBeastCrossRank Unmarshal failed err: %s", err)
		return
	}

	actorData := msg.GetSelfData()
	player := manager.GetPlayerPtrById(actorData.ActorId)
	if player == nil {
		return
	}
	player.SendProto3(31, 150, &msg)
}

func actSectBeastClose(_ []byte) {
	if sectActEnd {
		return
	}
	sectActEnd = true
	trySendPerDamageMail()
	trySendSectDamageMail()

	if engine.FightClientExistPredicate(base.SmallCrossServer) {
		if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSectBeastActEnd, nil); err != nil {
			logger.LogError("G2CSectBeastActEnd call err: %v", err)
		}
	}
}

func actSectBeastStart(_ []byte) {
	sectActEnd = false
	sectActorInfo = make(map[uint64]uint64)
	totalDamage = 0

	if engine.FightClientExistPredicate(base.SmallCrossServer) {
		if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSectBeastActStart, nil); err != nil {
			logger.LogError("G2CSectBeastActStart call err: %v", err)
		}
	}
}

func trySendPerDamageMail() {
	conf := jsondata.GetSectBeastConf()
	if conf == nil {
		return
	}

	for actorId, damage := range sectActorInfo {
		confL := jsondata.GetSectBeastDamageConfLByDamage(int64(damage))
		var awards []*jsondata.StdReward

		for _, conf := range confL {
			awards = append(awards, conf.Awards...)
		}
		if len(awards) > 0 {
			mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
				ConfId:  common.Mail_SectBeastDamageReward,
				Rewards: jsondata.MergeStdReward(awards),
			})
		}
	}
}

// 2024.2.20参与活动造成伤害才会发奖励
func trySendSectDamageMail() {
	conf := jsondata.GetSectBeastConf()
	if conf == nil {
		return
	}

	confL := jsondata.GetSectBeastSectDamageConfLByDamage(int64(totalDamage))
	var awards []*jsondata.StdReward
	for _, conf := range confL {
		awards = append(awards, conf.Awards...)
	}

	if len(awards) > 0 {
		awards2 := jsondata.MergeStdReward(awards)
		for actorId := range sectActorInfo {
			mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
				ConfId:  common.Mail_SectBeastDamageReward2,
				Rewards: awards2,
			})
		}
	}
}

func clearSectData() {
	sectActorInfo = make(map[uint64]uint64)
	totalDamage = 0
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		if gshare.GetStaticVar().SectBeast == nil {
			gshare.GetStaticVar().SectBeast = &pb3.SectBeast{
				ExpLv: &pb3.ExpLvSt{Lv: 1, Exp: 0},
			}
		}
	})

	event.RegSysEvent(custom_id.SeFightSrvConnSucc, func(args ...interface{}) {
		sectBeast := gshare.GetStaticVar().SectBeast
		if sectBeast == nil {
			return
		}
		syncBeastExpLvToFight(sectBeast)
	})

	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		clearSectData()
	})

	engine.RegisterSysCall(sysfuncid.F2GSectBeastActStart, actSectBeastStart)
	engine.RegisterSysCall(sysfuncid.F2GSectBeastActClose, actSectBeastClose)
	engine.RegisterSysCall(sysfuncid.F2GSectBeastDamageRet, onSyncSectDamage)
	engine.RegisterSysCall(sysfuncid.F2GSectBeastBossKill, onSectBeastBossKill)

	engine.RegisterSysCall(sysfuncid.C2GSectBeastRankRet, onSectBeastCrossRank)
}
