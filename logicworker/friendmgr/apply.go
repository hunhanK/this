/**
 * @Author: LvYuMeng
 * @Date: 2023/12/5
 * @Desc:
**/

package friendmgr

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type MarryApplyMgr struct {
	marryApply map[uint64]*pb3.MarryApply
}

var marryApplyMgr = &MarryApplyMgr{
	marryApply: make(map[uint64]*pb3.MarryApply),
}

func GetMarryApplyMgr() *MarryApplyMgr {
	return marryApplyMgr
}

func DeleteDelPlayerData(ids map[uint64]struct{}) {
	delCommonIds := make([]uint64, 0)
	allData := GetFriendShip()
	for commonId, v := range allData.FriendCommonData {
		leftIds := make([]uint64, 0, 2)
		if _, exist := ids[v.ActorId1]; !exist {
			leftIds = append(leftIds, v.ActorId1)
		}
		if _, exist := ids[v.ActorId2]; !exist {
			leftIds = append(leftIds, v.ActorId2)
		}

		// 组合解散，更新排行榜
		if len(leftIds) < 2 {
			delCommonIds = append(delCommonIds, commonId)

			// 剩下一个人
			if len(leftIds) == 1 {
				leftId := leftIds[0]
				GetMarryApplyMgr().BackCashGift(leftId) //退还礼金
				BackMarryDepot(v.ActorId1, v.ActorId2, leftId)
			}
		}
	}

	for _, delId := range delCommonIds {
		delete(allData.FriendCommonData, delId)
	}

	gshare.GetStaticVar().FriendRecommed = pie.Uint64s(gshare.GetStaticVar().FriendRecommed).FilterNot(func(u uint64) bool {
		_, ok := ids[u]
		return ok
	})
}

func MarryDepotGlobalVarKey(actorId1, actorId2 uint64) string {
	if actorId1 < actorId2 {
		return fmt.Sprintf("%d_%d", actorId1, actorId2)
	}

	return fmt.Sprintf("%d_%d", actorId2, actorId1)
}

func BackMarryDepot(actorId1, actorId2, backActorId uint64) {
	mds := gshare.GetStaticVar().MarryDepots
	key := MarryDepotGlobalVarKey(actorId1, actorId2)

	depot, has := mds[key]
	if !has {
		return
	}

	var rewards []*jsondata.StdReward
	for _, item := range depot.Items {
		if item.Ext.OwnerId != backActorId {
			continue
		}
		rewards = append(rewards, &jsondata.StdReward{
			Id:    item.GetItemId(),
			Count: item.GetCount(),
			Bind:  item.GetBind(),
		})
	}
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(backActorId, &mailargs.SendMailSt{
			ConfId:  common.Mail_MarryDepotReturn,
			Rewards: rewards,
		})
	}
	delete(gshare.GetStaticVar().MarryDepots, key)
}

func (mgr *MarryApplyMgr) HasMarryInvite(tarId uint64) bool {
	if _, ok := mgr.marryApply[tarId]; ok {
		return true
	}

	return false
}

func (mgr *MarryApplyMgr) SendMarryInvite(apply *pb3.MarryApply, tarId uint64) {
	_, ok := mgr.marryApply[tarId]
	if ok {
		logger.LogError("target(%d) has a marry invite", tarId)
		return
	}
	mgr.marryApply[tarId] = apply
	endTime := apply.EndTime

	timer.SetTimeout(time.Duration(endTime-time_util.NowSec())*time.Second, func() {
		req, has := mgr.marryApply[tarId]
		if !has || req.EndTime > time_util.NowSec() {
			return
		}
		mgr.BackCashGift(tarId)
	})
	return
}

func (mgr *MarryApplyMgr) GiveMarryReward(friendshipId uint64, gradeConf *jsondata.MarryGradeConf) bool {
	fData, ok := GetFriendCommonDataById(friendshipId)
	if !ok {
		logger.LogError("get friend common data err")
		return false
	}
	if nil == fData.MarryInfo.WeddingBanquetTimes {
		fData.MarryInfo.WeddingBanquetTimes = make(map[uint32]uint32)
	}
	//婚宴次数增加
	fData.MarryInfo.WeddingBanquetTimes[gradeConf.BanquetSceneId] += gradeConf.BanquetTimes
	BroadProto3ToCouple(friendshipId, 53, 20, &pb3.S2C_53_20{Times: fData.MarryInfo.WeddingBanquetTimes})

	event.TriggerSysEvent(custom_id.SeGiveMarryReward, &pb3.CommonSt{U64Param: fData.ActorId1, U64Param2: fData.ActorId2, U32Param: gradeConf.ID})

	mgr.giveMarryRewardToActor(fData.ActorId1, fData.ActorId2, gradeConf)
	mgr.giveMarryRewardToActor(fData.ActorId2, fData.ActorId1, gradeConf)
	return true
}

func (mgr *MarryApplyMgr) giveMarryRewardToActor(actorId, targetId uint64, gradeConf *jsondata.MarryGradeConf) {
	actor := manager.GetPlayerPtrById(actorId)
	var canGiveAwards = true
	if actor != nil {
		canGiveAwards = actor.CheckMarryRewardTimes(gradeConf)
	}
	var targetName string
	if baseData, ok := manager.GetData(targetId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		targetName = baseData.GetName()
	}
	if canGiveAwards {
		mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
			ConfId:  common.Mail_MarrySuccess,
			Rewards: gradeConf.Rewards,
			Content: &mailargs.PlayerNameArgs{
				Name: targetName,
			},
		})
		actor.AddMarryRewardTimes(gradeConf)
	} else {
		mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
			ConfId: common.Mail_MarryRewardTimesUpLimit,
			Content: &mailargs.PlayerNameArgs{
				Name: targetName,
			},
		})
	}

	if nil != actor {
		actor.SendProto3(53, 4, &pb3.S2C_53_4{
			ActorId: targetId,
			Agree:   true,
			Grade:   gradeConf.ID,
		})
	}
	return
}

func (mgr *MarryApplyMgr) AcceptMarry(actor iface.IPlayer, reqId, friendshipId uint64) (uint32, bool) {
	apply, ok := mgr.marryApply[actor.GetId()]
	if !ok { //没有申请或者申请过期
		return 0, false
	}

	conf := jsondata.GetMarryConf()

	gradeConf := conf.Grade[apply.ConfId]
	if nil == gradeConf {
		logger.LogError("marry grade conf(%d) is nil", apply.ConfId)
		return 0, false
	}

	if apply.IsAA {
		consume := gradeConf.AAConsume

		if !actor.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogMarryApply}) {
			actor.SendTipMsg(tipmsgid.TpItemNotEnough)
			return 0, false
		}
	}

	fData, ok := GetFriendCommonDataById(friendshipId)
	if !ok {
		logger.LogError("get friend common data err")
		return 0, false
	}

	if !utils.IsSetBit(fData.Status, custom_id.FsMarry) { //结婚
		fData.Status = utils.SetBit(fData.Status, custom_id.FsMarry)
		fData.MarryInfo = &pb3.MarryCommonData{MarryTime: time_util.NowSec()}
		if target := manager.GetSimplyData(reqId); nil != target {
			broadcastId := utils.Ternary(gradeConf.IsSameSex, tipmsgid.MarrySuccSameSex, tipmsgid.MarrySuccOppoSex).(int)
			engine.BroadcastTipMsgById(uint32(broadcastId), actor.GetName(), target.GetName())
		}
		logworker.LogPlayerBehavior(actor, pb3.LogId_LogMarryApply, &pb3.LogPlayerCounter{
			NumArgs: reqId,
		})
	}

	delete(mgr.marryApply, actor.GetId())

	mgr.GiveMarryReward(friendshipId, gradeConf)

	return apply.ConfId, true
}

func (mgr *MarryApplyMgr) BackCashGift(id uint64) {
	apply, ok := mgr.marryApply[id]
	if !ok {
		return
	}

	backReward := jsondata.Pb3RewardVecToStdRewardVec(apply.Back)
	var tarName string
	if baseData, ok := manager.GetData(id, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		tarName = baseData.GetName()
	}
	delete(mgr.marryApply, id)
	mailmgr.SendMailToActor(apply.ReqId, &mailargs.SendMailSt{
		ConfId:  common.Mail_CashGiftRefuse,
		Rewards: backReward,
		Content: &mailargs.PlayerNameArgs{
			Name: tarName,
		},
	})
	if applier := manager.GetPlayerPtrById(apply.ReqId); nil != applier {
		applier.SendTipMsg(tipmsgid.MarryRefuseReq, tarName)
	}
}

func (mgr *MarryApplyMgr) SendMarryInfo(actor iface.IPlayer) {
	apply, ok := mgr.marryApply[actor.GetId()]
	if !ok {
		return
	}

	rsp := &pb3.S2C_53_3{
		Apply:  apply,
		Player: manager.GetSimplyData(apply.ReqId),
	}
	actor.SendProto3(53, 3, rsp)
}

func refreshMarryApply(args ...interface{}) {
	var (
		data = GetFriendShip()
		mgr  = GetMarryApplyMgr()
	)

	mgr.marryApply = data.MarryApply

	for actorId, apply := range mgr.marryApply {
		endTime := apply.EndTime
		timer.SetTimeout(time.Duration(endTime-time_util.NowSec())*time.Second, func() {
			req, has := data.MarryApply[actorId]
			if !has || req.EndTime > time_util.NowSec() {
				return
			}
			mgr.BackCashGift(actorId)
		})
	}
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, refreshMarryApply)

	gmevent.Register("mergeDivorce", func(player iface.IPlayer, args ...string) bool {
		playerId := utils.AtoUint64(args[0])
		DeleteDelPlayerData(map[uint64]struct{}{playerId: {}})
		return true
	}, 1)
}
