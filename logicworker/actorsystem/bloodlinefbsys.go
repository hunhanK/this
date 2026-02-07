/**
 * @Author: LvYuMeng
 * @Date: 2025/7/2
 * @Desc: 血脉副本
**/

package actorsystem

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"sort"
)

type BloodLineFbSys struct {
	Base
}

func (s *BloodLineFbSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *BloodLineFbSys) OnReconnect() {
	s.s2cInfo()
}

func (s *BloodLineFbSys) s2cInfo() {
	s.SendProto3(78, 1, &pb3.S2C_78_1{Data: s.getData()})
}

func (s *BloodLineFbSys) onNewDay() {
	data := s.getData()
	data.BuyTimes = 0
	s.s2cInfo()
}

func (s *BloodLineFbSys) getData() *pb3.BloodlineFbData {
	binary := s.GetBinaryData()
	if nil == binary.BloodlineFbData {
		binary.BloodlineFbData = &pb3.BloodlineFbData{}
	}
	return binary.BloodlineFbData
}

func (s *BloodLineFbSys) isInFb() bool {
	conf := jsondata.GetBloodlineFbConf()
	if nil == conf {
		return false
	}

	return s.owner.GetFbId() == conf.FbId
}

func (s *BloodLineFbSys) canEnterLayer(sceneId uint32) bool {
	conf := jsondata.GetBloodlineLayerConf(sceneId)
	if nil == conf {
		return false
	}

	if conf.MergeTimes > gshare.GetMergeTimes() {
		return false
	}

	if conf.MergeDays > gshare.GetMergeSrvDayByTimes(conf.MergeTimes) {
		return false
	}

	return true
}

func (s *BloodLineFbSys) c2sEnter(msg *base.Message) error {
	var req pb3.C2S_78_2
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if s.isInFb() {
		return neterror.ParamsInvalidError("in fb")
	}

	if !s.canEnterLayer(req.SceneId) {
		return neterror.ParamsInvalidError("layer cond err")
	}

	data := s.getData()

	enterConsumeConf := jsondata.GetBloodlineEnterConsumeByTimes(data.BuyTimes+1, s.owner.GetVipLevel())
	if nil == enterConsumeConf {
		return neterror.ParamsInvalidError("times or vip err")
	}

	err = s.owner.EnterFightSrv(base.SmallCrossServer, fubendef.EnterBloodlineFb,
		&pb3.EnterBloodlineFbReq{
			SceneId: req.SceneId,
		},
		&argsdef.ConsumesSt{
			Consumes: enterConsumeConf.Consume,
			LogId:    pb3.LogId_LogBloodlineFbEnterConsume,
		})

	if err != nil {
		return err
	}

	data.BuyTimes++
	s.s2cInfo()

	return nil
}

func (s *BloodLineFbSys) checkBossLimit(conf *jsondata.BloodlineFbBoss) bool {
	for _, v := range conf.AttrLimit {
		if s.owner.GetFightAttr(v.AttrId) < int64(v.Val) {
			return false
		}
	}
	return true
}

type bloodlineFbBoss struct {
	BossId uint32
	Energy uint32
	Sort   uint32
}

func (s *BloodLineFbSys) c2sSweep(msg *base.Message) error {
	var req pb3.C2S_78_3
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	data := s.getData()

	sceneId := req.SceneId
	conf := jsondata.GetBloodlineFbConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	enterConsumeConf := jsondata.GetBloodlineEnterConsumeByTimes(data.BuyTimes+1, s.owner.GetVipLevel())
	if nil == enterConsumeConf {
		return neterror.ParamsInvalidError("times or vip err")
	}

	for _, v := range conf.SweepAttrLimit {
		if s.owner.GetFightAttr(v.AttrId) < int64(v.Val) {
			return neterror.ParamsInvalidError("attr val not enough")
		}
	}

	if s.owner.GetLevel() < conf.SweepLevel {
		s.owner.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}

	if s.isInFb() {
		return neterror.ParamsInvalidError("in fb")
	}

	if !s.canEnterLayer(sceneId) {
		return neterror.ParamsInvalidError("layer cond err")
	}

	demonicConf := jsondata.GetDemonicConf(conf.FbId)
	if nil == demonicConf {
		return neterror.ConfNotFoundError("demonic conf not found")
	}

	var bossSort []*bloodlineFbBoss
	for _, v := range conf.Boss {
		if v.SceneId != sceneId {
			continue
		}
		if jsondata.GetMonsterType(v.BossId) != custom_id.MtBoss {
			continue
		}
		if !s.checkBossLimit(v) {
			continue
		}
		bossSort = append(bossSort, &bloodlineFbBoss{
			BossId: v.BossId,
			Sort:   v.Sort,
			Energy: v.Energy,
		})
	}

	if len(bossSort) == 0 {
		return neterror.ParamsInvalidError("boss is nil")
	}

	sort.Slice(bossSort, func(i, j int) bool {
		return bossSort[i].Sort > bossSort[j].Sort
	})

	sweepBoss := map[uint32]uint32{}

	maxEnergy := demonicConf.DemLimit
	for _, v := range bossSort {
		if maxEnergy < v.Energy {
			continue
		}

		num := maxEnergy / v.Energy
		maxEnergy -= v.Energy * num
		sweepBoss[v.BossId] = num
	}

	logger.LogDebug("sweep boss %v", sweepBoss)

	if !s.owner.CheckConsumeByConf(enterConsumeConf.Consume, false, pb3.LogId_LogBloodlineFbSweepConsume) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	err = s.owner.CallActorFunc(actorfuncid.G2FGetDropAwardsToBag, &pb3.G2FGetDropAwardsToBag{
		Monsters: sweepBoss,
		LogId:    uint32(pb3.LogId_LogBloodlineFbSweepAwards),
	})
	if err != nil {
		return err
	}

	if !s.owner.ConsumeByConf(enterConsumeConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogBloodlineFbSweepConsume}) {
		s.owner.LogError("consume not pay")
	}

	data.BuyTimes++
	s.s2cInfo()

	return nil
}

func (s *BloodLineFbSys) c2sAddEnergy(msg *base.Message) error {
	if !s.isInFb() {
		return neterror.ParamsInvalidError("not in fb")
	}

	data := s.getData()

	enterConsumeConf := jsondata.GetBloodlineEnterConsumeByTimes(data.BuyTimes+1, s.owner.GetVipLevel())
	if nil == enterConsumeConf {
		return neterror.ParamsInvalidError("times or vip err")
	}

	if !s.owner.ConsumeByConf(enterConsumeConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogBloodlineFbAddEnergy}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	err := s.owner.CallActorFunc(actorfuncid.G2FAddBloodlineEnergy, nil)

	if err != nil {
		return err
	}

	data.BuyTimes++
	s.s2cInfo()

	return nil
}

func addBloodLineFbTimes(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}

	recoverTimes := st.U32Param

	if s, ok := player.GetSysObj(sysdef.SiBloodLineFb).(*BloodLineFbSys); ok && s.IsOpen() {
		data := s.getData()
		if data.BuyTimes > recoverTimes {
			data.BuyTimes -= recoverTimes
		} else {
			data.BuyTimes = 0
		}
		s.s2cInfo()
	}
}

func init() {
	RegisterSysClass(sysdef.SiBloodLineFb, func() iface.ISystem {
		return &BloodLineFbSys{}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if s, ok := player.GetSysObj(sysdef.SiBloodLineFb).(*BloodLineFbSys); ok && s.IsOpen() {
			s.onNewDay()
		}
	})

	engine.RegisterMessage(gshare.OfflineAddBloodlineFBTimes, func() pb3.Message {
		return &pb3.CommonSt{}
	}, addBloodLineFbTimes)

	net.RegisterSysProtoV2(78, 2, sysdef.SiBloodLineFb, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BloodLineFbSys).c2sEnter
	})
	net.RegisterSysProtoV2(78, 3, sysdef.SiBloodLineFb, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BloodLineFbSys).c2sSweep
	})
	net.RegisterSysProtoV2(78, 4, sysdef.SiBloodLineFb, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BloodLineFbSys).c2sAddEnergy
	})

	gmevent.Register("boodlinefb.clearTimes", func(player iface.IPlayer, args ...string) bool {
		if s, ok := player.GetSysObj(sysdef.SiBloodLineFb).(*BloodLineFbSys); ok && s.IsOpen() {
			data := s.getData()
			data.BuyTimes = 0
			s.s2cInfo()
		}
		return true
	}, 1)

}
