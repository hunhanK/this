/**
 * @Author: LvYuMeng
 * @Date: 2025/12/15
 * @Desc: 天道商店
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type HeavenStoreSys struct {
	Base
}

func (s *HeavenStoreSys) OnReconnect() {
	s.s2cInfo()
	s.s2cGlobalInfo()
}

func (s *HeavenStoreSys) OnAfterLogin() {
	s.s2cInfo()
	s.s2cGlobalInfo()
}

func (s *HeavenStoreSys) getData() *pb3.HeavenStoreData {
	binary := s.GetBinaryData()
	if binary.HeavenStoreData == nil {
		binary.HeavenStoreData = &pb3.HeavenStoreData{}
	}
	if binary.HeavenStoreData.BuyCount == nil {
		binary.HeavenStoreData.BuyCount = make(map[uint32]uint32)
	}
	return binary.HeavenStoreData
}

func (s *HeavenStoreSys) getSrvData() *pb3.HeavenStoreSrvData {
	globalVar := gshare.GetStaticVar()
	if globalVar.HeavenStoreSrvData == nil {
		globalVar.HeavenStoreSrvData = &pb3.HeavenStoreSrvData{}
	}
	if globalVar.HeavenStoreSrvData.BuyCount == nil {
		globalVar.HeavenStoreSrvData.BuyCount = make(map[uint32]uint32)
	}
	return globalVar.HeavenStoreSrvData
}

func (s *HeavenStoreSys) s2cInfo() {
	rsp := &pb3.S2C_30_40{
		Data: s.getData(),
	}
	s.SendProto3(30, 40, rsp)
}

func (s *HeavenStoreSys) s2cGlobalInfo() {
	rsp := &pb3.S2C_30_44{
		SrvBuyCount: s.getSrvData().BuyCount,
	}
	s.SendProto3(30, 44, rsp)
}

const (
	HeavenStoreOpenSrvDay     = 1
	HeavenStoreOpenNaturalDay = 2
)

func (s *HeavenStoreSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_30_41
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	conf, ok := jsondata.GetHeavenStoreConfById(req.Id)
	if !ok {
		return neterror.ConfNotFoundError("heaven store conf %d is nil", req.Id)
	}
	if len(conf.Consume) == 0 {
		return neterror.ParamsInvalidError("cant direct buy")
	}
	if req.Count == 0 {
		return neterror.ParamsInvalidError("count zero")
	}
	data := s.getData()
	if conf.PerLimit > 0 {
		if data.BuyCount[req.Id]+req.Count > conf.PerLimit {
			return neterror.ParamsInvalidError("per count exceed")
		}
	}
	srvData := s.getSrvData()
	if conf.SrvLimit > 0 {
		if srvData.BuyCount[req.Id]+req.Count > conf.SrvLimit {
			return neterror.ParamsInvalidError("srv count exceed")
		}
	}
	switch conf.OpenType {
	case HeavenStoreOpenSrvDay:
		openDay := gshare.GetOpenServerDay()
		sTime := utils.AtoUint32(conf.StartTime)
		eTime := utils.AtoUint32(conf.EndTime)
		if (sTime > 0 && openDay < sTime) || (eTime > 0 && openDay > eTime) {
			return neterror.ParamsInvalidError("not open")
		}
	case HeavenStoreOpenNaturalDay:
		nowSec := time_util.NowSec()
		sTime := time_util.StrToTime(conf.StartTime + " 00:00:00")
		eTime := time_util.StrToTime(conf.EndTime + " 23:59:59")
		if (sTime > 0 && nowSec < sTime) || (eTime > 0 && nowSec > eTime) {
			return neterror.ParamsInvalidError("not open")
		}
	default:
		return neterror.ConfNotFoundError("open type not define")
	}
	if !s.owner.ConsumeByConf(jsondata.ConsumeMulti(conf.Consume, req.Count), false, common.ConsumeParams{LogId: pb3.LogId_LogHeavenStoreBuyConsume}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	if conf.PerLimit > 0 {
		data.BuyCount[req.Id] += req.Count
	}
	s.SendProto3(30, 41, &pb3.S2C_30_41{
		Id:    req.Id,
		Count: data.BuyCount[req.Id],
	})
	if conf.SrvLimit > 0 {
		srvData.BuyCount[req.Id] += req.Count
		engine.Broadcast(chatdef.CIWorld, 0, 30, 42, &pb3.S2C_30_42{
			Id:    req.Id,
			Count: srvData.BuyCount[req.Id],
		}, 0)
	}
	engine.GiveRewards(s.owner, jsondata.StdRewardMulti(conf.Rewards, int64(req.Count)), common.EngineGiveRewardParam{LogId: pb3.LogId_LogHeavenStoreBuyAwards})
	if len(conf.Attrs) > 0 {
		s.ResetSysAttr(attrdef.SaHeavenStore)
	}
	if conf.BroadcastId > 0 {
		engine.BroadcastTipMsgById(conf.BroadcastId, s.owner.GetId(), s.owner.GetName(), engine.StdRewardToBroadcast(s.owner, conf.Rewards))
	}
	return nil
}

func (s *HeavenStoreSys) c2sDailyAwards(msg *base.Message) error {
	var req pb3.C2S_30_43
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	data := s.getData()
	if data.LastRewardTime > 0 {
		return neterror.ParamsInvalidError("has received")
	}
	data.LastRewardTime = time_util.NowSec()
	s.SendProto3(30, 43, &pb3.S2C_30_43{LastRewardTime: data.LastRewardTime})
	return nil
}

func (s *HeavenStoreSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	for id := range data.BuyCount {
		conf, ok := jsondata.GetHeavenStoreConfById(id)
		if !ok {
			continue
		}
		if len(conf.Attrs) == 0 {
			continue
		}
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, conf.Attrs)
	}
}

func (s *HeavenStoreSys) onNewDay() {
	data := s.getData()
	data.LastRewardTime = 0
	var delIds []uint32
	for id := range data.BuyCount {
		conf, ok := jsondata.GetHeavenStoreConfById(id)
		if !ok {
			continue
		}
		if conf.DailyReset {
			delIds = append(delIds, id)
		}
	}
	for _, id := range delIds {
		delete(data.BuyCount, id)
	}
	s.s2cInfo()
}

func (s *HeavenStoreSys) onCharge() {
	realChargeInfo := s.owner.GetBinaryData().GetRealChargeInfo()
	if nil == realChargeInfo {
		return
	}
	chargeCent := int64(realChargeInfo.TotalChargeMoney)
	data := s.getData()
	diff := chargeCent - data.ExchangeChargeCent
	if diff <= 0 {
		return
	}
	unit := int64(jsondata.GlobalUint("heavenStoreScoreChargeCent"))
	add := diff / unit
	if add <= 0 {
		return
	}
	record := unit * add
	data.ExchangeChargeCent += record
	s.owner.AddMoney(moneydef.HeavenStoreScore, add, true, pb3.LogId_LogHeavenStoreScore)
}

func init() {
	RegisterSysClass(sysdef.SiHeavenStore, func() iface.ISystem {
		return &HeavenStoreSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaHeavenStore, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		if sys, ok := player.GetSysObj(sysdef.SiHeavenStore).(*HeavenStoreSys); ok && sys.IsOpen() {
			sys.calcAttr(calc)
		}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if sys, ok := player.GetSysObj(sysdef.SiHeavenStore).(*HeavenStoreSys); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})

	event.RegActorEvent(custom_id.AeCharge, func(player iface.IPlayer, args ...interface{}) {
		if sys, ok := player.GetSysObj(sysdef.SiHeavenStore).(*HeavenStoreSys); ok {
			sys.onCharge()
		}
	})

	net.RegisterSysProtoV2(30, 41, sysdef.SiHeavenStore, func(s iface.ISystem) func(*base.Message) error {
		return s.(*HeavenStoreSys).c2sBuy
	})

	net.RegisterSysProtoV2(30, 43, sysdef.SiHeavenStore, func(s iface.ISystem) func(*base.Message) error {
		return s.(*HeavenStoreSys).c2sDailyAwards
	})

	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		s, ok := player.GetSysObj(sysdef.SiHeavenStore).(*HeavenStoreSys)
		if !ok || !s.IsOpen() {
			return
		}

		if len(jsondata.HeavenPrivilege2Id) == 0 {
			return
		}

		data := s.getData()

		for k, v := range conf.HeavenStore {
			if v == 0 {
				continue
			}

			id := jsondata.HeavenPrivilege2Id[uint32(k+1)]
			if data.BuyCount[id] == 0 {
				continue
			}

			total += int64(v)
		}

		return
	})
}
