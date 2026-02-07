/**
 * @Author: LvYuMeng
 * @Date: 2024/4/28
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

type WholeCityLoveSys struct {
	PlayerYYBase
}

func (s *WholeCityLoveSys) formatGlobalData() *pb3.WholeCityLoveRecord {
	if !s.IsOpen() {
		return nil
	}
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.WholeCityLoveRecord {
		globalVar.WholeCityLoveRecord = make(map[uint32]*pb3.WholeCityLoveRecord)
	}
	idx := s.Id
	if nil == globalVar.WholeCityLoveRecord[idx] {
		globalVar.WholeCityLoveRecord[idx] = &pb3.WholeCityLoveRecord{}
	}
	g := globalVar.WholeCityLoveRecord[idx]
	g.StartTime = s.OpenTime
	g.EndTime = s.EndTime
	g.ConfIdx = s.ConfIdx
	if nil == g.PData {
		g.PData = make(map[uint64]*pb3.WholeCityLovePlayer)
	}
	return g
}

func (s *WholeCityLoveSys) Login() {
	s.s2cInfo()
}

func (s *WholeCityLoveSys) OnReconnect() {
	s.s2cInfo()
}

func (s *WholeCityLoveSys) OnOpen() {
	s.reset()
	s.s2cInfo()
}

func (s *WholeCityLoveSys) reset() {
	s.formatGlobalData()
}

func (s *WholeCityLoveSys) s2cInfo() {
	g := s.formatGlobalData()
	rsp := &pb3.S2C_53_75{ActiveId: s.GetId()}
	if pData := g.PData[s.GetPlayer().GetId()]; nil != pData {
		rsp.IsRev = pData.RevStatus
		rsp.Flag = pData.BuyRecord
	}
	s.SendProto3(53, 75, rsp)
}

func (s *WholeCityLoveSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_53_76
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYWholeCityLoveAwardConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("whole city love conf is nil")
	}

	playerId := s.GetPlayer().GetId()
	g := s.formatGlobalData()

	pData := g.PData[playerId]

	if nil == pData {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	if pData.RevStatus {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	flag := pData.BuyRecord
	for _, v := range conf.MeetType {
		if !utils.IsSetBit(flag, v) {
			s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
			return nil
		}
	}

	pData.RevStatus = true

	if len(conf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogWholeCityLoveAward})
	}

	s.s2cInfo()

	return nil
}

func (s *WholeCityLoveSys) c2sRankList(_ *base.Message) error {
	g := s.formatGlobalData()
	rankList := make([]*pb3.WholeCityLoveRank, 0, len(g.Rank))
	for _, v := range g.Rank {
		actor1, ok := manager.GetData(v.Key, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			continue
		}
		actor2, ok := manager.GetData(v.Value, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			continue
		}
		rankList = append(rankList, &pb3.WholeCityLoveRank{
			ActorId1: actor1.GetId(),
			Name1:    actor1.GetName(),
			ActorId2: actor2.GetId(),
			Name2:    actor2.GetName(),
		})
	}
	s.SendProto3(53, 77, &pb3.S2C_53_77{
		ActiveId: s.GetId(),
		Rank:     rankList,
	})
	return nil
}

func onWholeCityLoveMarryBuy(args ...interface{}) {
	if len(args) < 1 {
		return
	}

	commonSt, ok := args[0].(*pb3.CommonSt)
	if !ok {
		return
	}

	var (
		actorId1 = commonSt.GetU64Param()
		actorId2 = commonSt.GetU64Param2()
		confId   = commonSt.GetU32Param()
	)

	if actorId1 > actorId2 {
		actorId1, actorId2 = actorId2, actorId1
	}

	conf, ok := jsondata.GetMarryConf().Grade[confId]
	if !ok {
		logger.LogError("marry grade conf(%d) is nil", confId)
		return
	}

	nowSec := time_util.NowSec()

	globalVar := gshare.GetStaticVar()
	for id, record := range globalVar.WholeCityLoveRecord {
		if record.StartTime <= nowSec && record.EndTime > nowSec {
			pyyConf := jsondata.GetPlayerYYConf(id)
			if nil == pyyConf {
				continue
			}
			yyConf := jsondata.GetYYWholeCityLoveAwardConf(pyyConf.ConfName, record.ConfIdx)
			if nil == yyConf {
				continue
			}
			var meetKey uint32
			for _, v := range yyConf.MeetType {
				meetKey = utils.SetBit(meetKey, v)
			}
			if nil == record.PData[actorId1] {
				record.PData[actorId1] = &pb3.WholeCityLovePlayer{}
			}
			if nil == record.PData[actorId2] {
				record.PData[actorId2] = &pb3.WholeCityLovePlayer{}
			}
			record.PData[actorId1].BuyRecord = utils.SetBit(record.PData[actorId1].BuyRecord, conf.Type)
			record.PData[actorId2].BuyRecord = utils.SetBit(record.PData[actorId2].BuyRecord, conf.Type)
			if meetKey == 0 {
				continue
			}
			if len(record.Rank) < int(yyConf.RankShow) { //确认可否上榜
				isFind := false
				var flag uint32
				for _, v := range record.CpRecord {
					if v.ActorId1 == actorId1 && v.ActorId2 == actorId2 {
						isFind = true
						v.Flag = utils.SetBit(v.Flag, conf.Type)
						flag = v.Flag
					}
				}
				if !isFind {
					st := &pb3.WholeCityLoveCp{
						ActorId1: actorId1,
						ActorId2: actorId2,
						Flag:     utils.SetBit(0, conf.Type),
					}
					record.CpRecord = append(record.CpRecord, st)
					flag = st.Flag
				}
				if flag == meetKey {
					isNewCp := true
					for _, v := range record.Rank {
						if v.Key == actorId1 && v.Value == actorId2 {
							isNewCp = false
							break
						}
					}
					if isNewCp {
						record.Rank = append(record.Rank, &pb3.Key64Val64{
							Key:   actorId1,
							Value: actorId2,
						})
					}
				}
			}
			if len(record.Rank) >= int(yyConf.RankShow) { //满了就不管了
				record.CpRecord = nil
			}
		}
	}

	onWholeCityLoveProgressChange(actorId1)
	onWholeCityLoveProgressChange(actorId2)
}

func onWholeCityLoveProgressChange(actorId uint64) {
	player := manager.GetPlayerPtrById(actorId)
	if nil == player {
		return
	}
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYWholeCityLove)
	for _, obj := range yyList {
		if s, ok := obj.(*WholeCityLoveSys); ok && s.IsOpen() {
			s.s2cInfo()
		}
	}
}

func checkWholeCityLoveAward(args ...interface{}) {
	g := gshare.GetStaticVar()
	nowSec := time_util.NowSec()
	for id, record := range g.WholeCityLoveRecord {
		if record.IsOver {
			continue
		}
		if nowSec >= record.EndTime {
			pyyConf := jsondata.GetPlayerYYConf(id)
			if nil == pyyConf {
				continue
			}
			yyConf := jsondata.GetYYWholeCityLoveAwardConf(pyyConf.ConfName, record.ConfIdx)
			if nil == yyConf {
				continue
			}
			var meetKey uint32
			for _, v := range yyConf.MeetType {
				meetKey = utils.SetBit(meetKey, v)
			}
			if meetKey == 0 {
				continue
			}
			for actorId, v := range record.PData {
				if v.RevStatus {
					continue
				}
				if v.BuyRecord&meetKey == meetKey {
					v.RevStatus = true
					mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
						ConfId:  common.Mail_WholeCityLoveAward,
						Rewards: yyConf.Rewards,
					})
				}
			}
			record.IsOver = true
		}
	}

	for k, record := range g.WholeCityLoveRecord {
		if record.IsOver {
			delete(g.WholeCityLoveRecord, k)
		}
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYWholeCityLove, func() iface.IPlayerYY {
		return &WholeCityLoveSys{}
	})

	net.RegisterYYSysProtoV2(53, 76, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*WholeCityLoveSys).c2sRev
	})

	net.RegisterYYSysProtoV2(53, 77, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*WholeCityLoveSys).c2sRankList
	})

	event.RegSysEvent(custom_id.SeGiveMarryReward, onWholeCityLoveMarryBuy)

	event.RegSysEvent(custom_id.SeServerInit, checkWholeCityLoveAward)
	event.RegSysEvent(custom_id.SeNewDayArrive, checkWholeCityLoveAward)
}
