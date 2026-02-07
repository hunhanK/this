/**
 * @Author: zjj
 * @Date: 2024/8/1
 * @Desc: 屠龙BOSS
**/

package yy

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
	"time"
)

type KillDragonBossSys struct {
	YYBase
}

func (k *KillDragonBossSys) OnReconnect() {}

func (k *KillDragonBossSys) c2sEnterFb(player iface.IPlayer, _ *base.Message) error {
	conf := jsondata.GetYYKillerDragonBossConf(k.ConfName, k.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("%s not found conf", k.GetPrefix())
	}
	hour := time.Now().Hour()
	if uint32(hour) >= conf.StopChallengeHour {
		player.SendTipMsg(tipmsgid.PYYKillDragonBossNotChallenge)
		return nil
	}
	err := player.EnterFightSrv(base.LocalFightServer, fubendef.EnterPyyKillerDragonBoss, &pb3.EnterPYYKillDragonBossSt{
		ConfIdx:  k.ConfIdx,
		FbId:     conf.FuBenId,
		SceneId:  conf.SceneId,
		ConfName: k.ConfName,
	})
	if err != nil {
		k.LogError("err:%v", err)
		return err
	}
	return nil
}

func init() {
	yymgr.RegisterYYType(yydefine.YYKillerDragonBoss, func() iface.IYunYing {
		return &KillDragonBossSys{}
	})

	net.RegisterGlobalYYSysProto(61, 11, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*KillDragonBossSys).c2sEnterFb
	})
	gmevent.Register("enterKillDragonBossSys", func(player iface.IPlayer, args ...string) bool {
		yyList := yymgr.GetAllYY(yydefine.YYKillerDragonBoss)
		if nil == yyList || len(yyList) == 0 {
			return false
		}
		for _, yy := range yyList {
			err := yy.(*KillDragonBossSys).c2sEnterFb(player, nil)
			if err != nil {
				player.LogError("err:%v", err)
			}
			break
		}
		return true
	}, 1)
	engine.RegisterSysCall(sysfuncid.F2GSettleDamagePYYKillDragonBoss, handleF2GSettleDamagePYYKillDragonBoss)
}

func handleF2GSettleDamagePYYKillDragonBoss(buf []byte) {
	var req pb3.SettleDamagePYYKillDragonBoss
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}
	player := manager.GetPlayerPtrById(req.ActorId)
	if player == nil {
		return
	}
	yyList := yymgr.GetAllYY(yydefine.YYKillerDragonBoss)
	if nil == yyList || len(yyList) == 0 {
		return
	}
	manager.UpdateGameplayScoreGoalRank(ranktype.GameplayScoreGoalRankTypeByKillDragonBoss, player, req.Damage, false)
	player.TriggerQuestEvent(custom_id.QttJoinDragonBoss, 0, 1)
}
