/**
 * @Author: LvYuMeng
 * @Date: 2024/9/19
 * @Desc: 仙魔变身
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"math"
)

type TransfigurationSys struct {
	Base
}

func (s *TransfigurationSys) getData() *pb3.Transfiguration {
	binary := s.owner.GetBinaryData()
	if binary.Transfiguration == nil {
		binary.Transfiguration = &pb3.Transfiguration{}
	}
	if nil == binary.Transfiguration.NormalCampInfo {
		binary.Transfiguration.NormalCampInfo = &pb3.TransfigurationCamp{}
	}
	if nil == binary.Transfiguration.PrivilegeCampInfo {
		binary.Transfiguration.PrivilegeCampInfo = make(map[uint32]*pb3.TransfigurationCamp)
	}
	return binary.Transfiguration
}

func (s *TransfigurationSys) OnLogin() {
	data := s.getData()
	if data.BuyPrivilege {
		s.owner.SetExtraAttr(attrdef.TransfigurationPrivilege, 1)
	}
	s.owner.SetExtraAttr(attrdef.TransfigurationEndTime, int64(data.TransfigurationEndTime))
	s.s2cInfo()
}

func (s *TransfigurationSys) OnOpen() {
	s.s2cInfo()
}

func (s *TransfigurationSys) OnReconnect() {
	s.s2cInfo()
}

func (s *TransfigurationSys) s2cInfo() {
	s.SendProto3(71, 0, &pb3.S2C_71_0{
		Data: s.getData(),
	})
}

const (
	TransfigurationUpdateLv    = 1
	TransfigurationUpdateBreak = 2
)

func (s *TransfigurationSys) getCampInfo(camp uint32) *pb3.TransfigurationCamp {
	data := s.getData()
	if !data.BuyPrivilege {
		return data.NormalCampInfo
	} else {
		if _, ok := data.PrivilegeCampInfo[camp]; !ok {
			data.PrivilegeCampInfo[camp] = &pb3.TransfigurationCamp{}
		}
		return data.PrivilegeCampInfo[camp]
	}
}

func (s *TransfigurationSys) onFlyCampChange() {
	s.ResetSysAttr(attrdef.SaTransfiguration)
}

func (s *TransfigurationSys) c2sLvUp(msg *base.Message) error {
	var req pb3.C2S_71_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	campConf := jsondata.GetTransfigurationCampConf(req.GetCamp())
	if nil == campConf {
		return neterror.ConfNotFoundError("TransfigurationSys camp conf is nil")
	}

	data := s.getData()
	if !data.BuyPrivilege && s.owner.GetExtraAttrU32(attrdef.FlyCamp) != req.GetCamp() {
		return neterror.ParamsInvalidError("TransfigurationSys camp not equal")
	}

	campInfo := s.getCampInfo(req.GetCamp())

	breakConf := campConf.GetTransfigurationBreakLvConf(campInfo.BreakLv)
	if nil == breakConf {
		return neterror.ConfNotFoundError("TransfigurationSys break conf is nil")
	}

	isLvLimit := campInfo.Lv >= breakConf.LvLimit

	if !isLvLimit {
		return s.lvUp(req.GetCamp(), campConf, campInfo)
	} else {
		return s.breakUp(req.GetCamp(), campConf, campInfo)
	}
}

func (s *TransfigurationSys) breakUp(camp uint32, campConf *jsondata.TransfigurationCampConf, campInfo *pb3.TransfigurationCamp) error {
	nextBreakLv := campInfo.BreakLv + 1
	nextBreakConf := campConf.GetTransfigurationBreakLvConf(nextBreakLv)
	if nil == nextBreakConf {
		return neterror.ConfNotFoundError("TransfigurationSys break conf is nil")
	}
	if !s.owner.ConsumeByConf(nextBreakConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogTransfigurationBreakUp,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	campInfo.BreakLv = nextBreakLv
	s.SendProto3(71, 1, &pb3.S2C_71_1{
		Camp:     camp,
		CampInfo: campInfo,
		Type:     TransfigurationUpdateBreak,
	})
	s.ResetSysAttr(attrdef.SaTransfiguration)
	return nil
}

func (s *TransfigurationSys) lvUp(camp uint32, campConf *jsondata.TransfigurationCampConf, campInfo *pb3.TransfigurationCamp) error {
	nextLv := campInfo.Lv + 1
	lvConf := campConf.GetTransfigurationLvConf(nextLv)
	if nil == lvConf {
		return neterror.ConfNotFoundError("TransfigurationSys lv conf is nil")
	}
	if !s.owner.ConsumeByConf(lvConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogTransfigurationLvUp,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	campInfo.Lv = nextLv
	s.SendProto3(71, 1, &pb3.S2C_71_1{
		Camp:     camp,
		CampInfo: campInfo,
		Type:     TransfigurationUpdateLv,
	})
	s.ResetSysAttr(attrdef.SaTransfiguration)
	return nil
}

func (s *TransfigurationSys) c2sTransfigure(msg *base.Message) error {
	if s.GetOwner().HasState(custom_id.EsStopTransfigure) {
		s.GetOwner().SendTipMsg(tipmsgid.BattleShieldTransformTip1)
		return nil
	}
	conf := jsondata.GetTransfigurationConf()
	if nil == conf {
		return neterror.ConfNotFoundError("TransfigurationSys conf is nil")
	}
	data := s.getData()
	nowSec := time_util.NowSec()
	if data.UseEndTime >= nowSec {
		return neterror.ParamsInvalidError("in Transfiguration use cd")
	}
	privilegeAddTime, _ := s.GetOwner().GetPrivilege(privilegedef.EnumTransfigurationAddTime)
	privilegeSubCd, _ := s.GetOwner().GetPrivilege(privilegedef.EnumTransfigurationSubUseCd)
	data.UseEndTime = nowSec + uint32(utils.MaxInt64(int64(conf.UseCd)-privilegeSubCd, 0))
	data.TransfigurationEndTime = nowSec + conf.TransfigurationCd + uint32(privilegeAddTime)
	s.owner.SetExtraAttr(attrdef.TransfigurationEndTime, int64(data.TransfigurationEndTime))
	var bId = conf.TransfigurationBuffId
	if data.BuyPrivilege {
		bId = conf.TransfigurationBuffIdPlus
	}
	if bId != 0 {
		s.owner.AddBuff(bId)
	}

	if data.BuyPrivilege {
		for _, buffId := range conf.PrivilegeBuffList {
			s.owner.AddBuff(buffId)
		}
	} else {
		flyCamp := s.owner.GetExtraAttrU32(attrdef.FlyCamp)
		campConf, ok := conf.Camp[flyCamp]
		if !ok {
			return neterror.ConfNotFoundError("TransfigurationSys camp conf is nil")
		}
		for _, buffId := range campConf.BuffList {
			s.owner.AddBuff(buffId)
		}
	}

	s.SendProto3(71, 4, &pb3.S2C_71_4{UseEndTime: data.UseEndTime})
	return nil
}

func (s *TransfigurationSys) GetMaxStorageChaosEnergyLimit() int64 {
	conf := jsondata.GetTransfigurationConf()
	if nil == conf {
		return 0
	}
	privilegeAdd, _ := s.owner.GetPrivilege(privilegedef.EnumTransfigurationMemoryAdd)
	maxStorage := int64(conf.MemoryLimit) + privilegeAdd
	return maxStorage
}

func check_privilegeTransfigurationChargeHandler(player iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	conf := jsondata.GetTransfigurationConf()
	if nil == conf {
		return false
	}
	if conf.ChargeId != chargeConf.ChargeId {
		return false
	}
	flyCamp := player.GetExtraAttrU32(attrdef.FlyCamp)
	if flyCamp == 0 {
		return false
	}
	sys, ok := player.GetSysObj(sysdef.SiTransfiguration).(*TransfigurationSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	if sys.getData().BuyPrivilege {
		return false
	}
	return true
}

func privilegeTransfigurationChargeHandler(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiTransfiguration).(*TransfigurationSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	data := sys.getData()
	if data.BuyPrivilege {
		return false
	}

	data.BuyPrivilege = true
	campInfo := data.NormalCampInfo
	flyCamp := player.GetExtraAttrU32(attrdef.FlyCamp)
	data.NormalCampInfo = nil
	data.PrivilegeCampInfo[flyCamp] = campInfo

	currentCount := sys.owner.GetMoneyCount(moneydef.ChaosEnergy)
	limit := sys.GetMaxStorageChaosEnergyLimit()
	addCount := limit - currentCount
	if addCount > 0 {
		player.AddMoney(moneydef.ChaosEnergy, addCount, true, pb3.LogId_LogTransfigurationPrivilege)
	}

	sys.s2cInfo()
	player.SetExtraAttr(attrdef.TransfigurationPrivilege, 1)
	sys.ResetSysAttr(attrdef.SaTransfiguration)
	logworker.LogPlayerBehavior(player, pb3.LogId_LogTransfigurationPrivilege, &pb3.LogPlayerCounter{})
	return true
}

func calcTransfigurationSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiTransfiguration).(*TransfigurationSys)
	if !ok || !sys.IsOpen() {
		return
	}
	conf := jsondata.GetTransfigurationConf()
	if nil == conf {
		return
	}
	data := sys.getData()
	if data.BuyPrivilege {
		for camp, campInfo := range data.PrivilegeCampInfo {
			campConf, ok := conf.Camp[camp]
			if !ok {
				continue
			}
			if lvConf := campConf.GetTransfigurationLvConf(campInfo.GetLv()); nil != lvConf {
				engine.CheckAddAttrsToCalc(player, calc, lvConf.Attrs)
			}
			if breakConf := campConf.GetTransfigurationBreakLvConf(campInfo.GetBreakLv()); nil != breakConf {
				engine.CheckAddAttrsToCalc(player, calc, breakConf.Attrs)
			}
		}
		engine.CheckAddAttrsToCalc(player, calc, conf.PrivilegeAttrs)
	} else {
		flyCamp := player.GetExtraAttrU32(attrdef.FlyCamp)
		campInfo := data.NormalCampInfo
		campConf, ok := conf.Camp[flyCamp]
		if !ok {
			return
		}
		if lvConf := campConf.GetTransfigurationLvConf(campInfo.GetLv()); nil != lvConf {
			engine.CheckAddAttrsToCalc(player, calc, lvConf.Attrs)
		}
		if breakConf := campConf.GetTransfigurationBreakLvConf(campInfo.GetBreakLv()); nil != breakConf {
			engine.CheckAddAttrsToCalc(player, calc, breakConf.Attrs)
		}
	}
}

func onTransfigurationFlyCampChange(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiTransfiguration).(*TransfigurationSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.onFlyCampChange()
}

func UseItemAddChaosEnergy(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	sys, ok := player.GetSysObj(sysdef.SiTransfiguration).(*TransfigurationSys)
	if !ok || !sys.IsOpen() {
		player.SendTipMsg(tipmsgid.TpSySNotOpen)
		return false, false, 0
	}

	addNum := int64(conf.Param[0])

	limitNum := sys.GetMaxStorageChaosEnergyLimit()

	currentCount := sys.owner.GetMoneyCount(moneydef.ChaosEnergy)

	if currentCount+addNum > limitNum {
		return
	}

	residueNum := limitNum - currentCount
	useItemCount := int64(math.Floor(float64(residueNum) / float64(addNum)))
	if useItemCount > param.Count {
		useItemCount = param.Count
	}

	add := addNum * useItemCount
	if !player.AddMoney(moneydef.ChaosEnergy, add, true, pb3.LogId_LogUseItemAddChaosEnergey) {
		return false, false, 0
	}

	return true, true, useItemCount
}

func init() {
	RegisterSysClass(sysdef.SiTransfiguration, func() iface.ISystem {
		return &TransfigurationSys{}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddChaosEnergy, UseItemAddChaosEnergy)

	event.RegActorEvent(custom_id.AeFlyCampChange, onTransfigurationFlyCampChange)

	engine.RegAttrCalcFn(attrdef.SaTransfiguration, calcTransfigurationSysAttr)

	engine.RegChargeEvent(chargedef.PrivilegeTransfiguration, check_privilegeTransfigurationChargeHandler, privilegeTransfigurationChargeHandler)

	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		sys, ok := player.GetSysObj(sysdef.SiTransfiguration).(*TransfigurationSys)
		if !ok || !sys.IsOpen() {
			return
		}
		if !sys.getData().BuyPrivilege {
			return
		}
		return int64(conf.Transfiguration), nil
	})

	net.RegisterSysProtoV2(71, 1, sysdef.SiTransfiguration, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*TransfigurationSys).c2sLvUp
	})
	net.RegisterSysProtoV2(71, 4, sysdef.SiTransfiguration, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*TransfigurationSys).c2sTransfigure
	})

	gmevent.Register("setTransLv", func(player iface.IPlayer, args ...string) bool {
		s, ok := player.GetSysObj(sysdef.SiTransfiguration).(*TransfigurationSys)
		if !ok || !s.IsOpen() {
			return false
		}
		if len(args) < 2 {
			return false
		}
		camp := utils.AtoUint32(args[0])
		lv := utils.AtoUint32(args[1])
		campInfo := s.getCampInfo(camp)
		campInfo.Lv = lv
		s.SendProto3(71, 1, &pb3.S2C_71_1{
			Camp:     camp,
			CampInfo: campInfo,
			Type:     TransfigurationUpdateLv,
		})
		s.ResetSysAttr(attrdef.SaTransfiguration)
		return true
	}, 1)
}
