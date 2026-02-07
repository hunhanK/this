/**
 * @Author: zjj
 * @Date: 2024/7/30
 * @Desc: 新灵符
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type NewCameliaSys struct {
	Base
}

func (s *NewCameliaSys) s2cInfo() {
	s.SendProto3(144, 10, &pb3.S2C_144_10{
		Data: s.getData(),
	})
}

func (s *NewCameliaSys) checkCanOpt(id uint32) bool {
	conf := jsondata.GetNewCameliaConf(id)
	if conf == nil {
		return false
	}

	if conf.OpenDay != 0 && conf.OpenDay > gshare.GetOpenServerDay() {
		return false
	}

	if conf.OpenLv != 0 && conf.OpenLv > s.GetOwner().GetLevel() {
		return false
	}

	if conf.OpenCircle != 0 && conf.OpenCircle > s.GetOwner().GetCircle() {
		return false
	}
	return true
}

func (s *NewCameliaSys) getData() *pb3.NewCameliaData {
	data := s.GetBinaryData().NewCamelia
	if data == nil {
		s.GetBinaryData().NewCamelia = &pb3.NewCameliaData{}
		data = s.GetBinaryData().NewCamelia
	}
	if data.CameliaDataMgr == nil {
		data.CameliaDataMgr = make(map[uint32]*pb3.SingleNewCamelia)
	}
	return data
}

func (s *NewCameliaSys) getSingleCameliaData(id uint32, skipInit bool) *pb3.SingleNewCamelia {
	data := s.getData()
	camelia, ok := data.CameliaDataMgr[id]
	if !ok && skipInit {
		return nil
	}
	if !ok || camelia == nil {
		data.CameliaDataMgr[id] = &pb3.SingleNewCamelia{
			Id: id,
		}
		camelia = data.CameliaDataMgr[id]
	}
	if camelia.SubPosDataMgr == nil {
		camelia.SubPosDataMgr = make(map[uint32]*pb3.SingleNewCameliaSubPosData)
	}
	return camelia
}

func (s *NewCameliaSys) getSingleCameliaSubPosData(id uint32, pos uint32, skipInit bool) *pb3.SingleNewCameliaSubPosData {
	data := s.getSingleCameliaData(id, skipInit)
	if data == nil {
		return nil
	}
	posData, ok := data.SubPosDataMgr[pos]
	if !ok && skipInit {
		return nil
	}
	if !ok || posData == nil {
		data.SubPosDataMgr[pos] = &pb3.SingleNewCameliaSubPosData{
			Pos: pos,
		}
		posData = data.SubPosDataMgr[pos]
	}
	return posData
}

func (s *NewCameliaSys) OnReconnect() {
	s.s2cInfo()
}

func (s *NewCameliaSys) OnLogin() {
	s.s2cInfo()
}

func (s *NewCameliaSys) OnOpen() {
	s.s2cInfo()
}

func (s *NewCameliaSys) c2sAddSubPosExp(msg *base.Message) error {
	var req pb3.C2S_144_11
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	id := req.Id
	pos := req.Pos
	number := req.Number
	if number == 0 {
		number = 1
	}

	if id == 0 || pos == 0 {
		return neterror.ParamsInvalidError("id %d pos %d", id, pos)
	}

	conf := jsondata.GetNewCameliaSubPosConf(id, pos)
	if conf == nil || conf.LvConf == nil || conf.AddExp == 0 || conf.AddExpItemId == 0 {
		return neterror.ConfNotFoundError("id %d pos %d not found conf", id, pos)
	}

	if !s.checkCanOpt(id) {
		return neterror.ParamsInvalidError("id %d  not reach opt cand", id)
	}

	data := s.getSingleCameliaSubPosData(id, pos, false)
	nextLv := data.Level + 1
	nextLvConf := conf.LvConf[nextLv]
	if nextLvConf == nil {
		return neterror.ParamsInvalidError("id %d pos %d level %d not found conf", id, pos, nextLv)
	}

	owner := s.GetOwner()
	if data.Exp >= nextLvConf.Exp {
		owner.SendTipMsg(tipmsgid.TpUseItemFailed)
		return nil
	}

	diffExp := nextLvConf.Exp - data.Exp
	var canAddCount uint32
	var canAddExp uint32
	for i := uint32(1); i <= number; i++ {
		confExp := conf.AddExp
		if canAddExp+confExp > diffExp {
			break
		}
		canAddExp += confExp
		canAddCount++
	}

	if !owner.ConsumeByConf(jsondata.ConsumeVec{{
		Id:    conf.AddExpItemId,
		Count: canAddCount,
	}}, false, common.ConsumeParams{LogId: pb3.LogId_LogNewCameliaAddSubPosExp}) {
		return neterror.ConsumeFailedError("id %d pos %d consume failed", id, pos)
	}

	data.Exp += canAddExp
	if data.Exp > nextLvConf.Exp {
		data.Exp = nextLvConf.Exp
	}
	data.FullExp = data.Exp == nextLvConf.Exp
	s.calcSubPosAttr(id)
	s.SendProto3(144, 11, &pb3.S2C_144_11{
		Id:      id,
		Pos:     pos,
		Exp:     data.Exp,
		FullExp: data.FullExp,
		AttrMap: data.AttrMap,
	})
	s.ResetSysAttr(attrdef.SaNewCameliaProperty)

	owner.TriggerQuestEvent(custom_id.QttNewCiMeLiaSubPosLevel, data.Pos, int64(data.Level))

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogNewCameliaAddSubPosExp, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d", pos),
	})

	return nil
}

func (s *NewCameliaSys) c2sUpLevel(msg *base.Message) error {
	var req pb3.C2S_144_12
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	id := req.Id
	if id == 0 {
		return neterror.ParamsInvalidError("id %d", id)
	}

	conf := jsondata.GetNewCameliaConf(id)
	if conf == nil {
		return neterror.ConfNotFoundError("id %d not found conf", id)
	}

	if !s.checkCanOpt(id) {
		return neterror.ParamsInvalidError("id %d  not reach opt cand", id)
	}

	data := s.getSingleCameliaData(id, false)
	var canUpLevel = true
	if len(data.SubPosDataMgr) != len(conf.SubPosMap) {
		return neterror.ParamsInvalidError("id %d not lock all sub pos", id)
	}

	for _, posData := range data.SubPosDataMgr {
		if posData.FullExp {
			continue
		}
		canUpLevel = false
		break
	}

	if !canUpLevel {
		return neterror.ParamsInvalidError("id %d not reach up level cand", id)
	}

	// 升级
	nextLv := data.Level + 1
	data.Level = nextLv
	for _, posData := range data.SubPosDataMgr {
		posData.Level = nextLv
		posData.Exp = 0
		posData.FullExp = false
	}
	s.calcSubPosAttr(id)
	s.SendProto3(144, 12, &pb3.S2C_144_12{
		Data: data,
	})
	s.ResetSysAttr(attrdef.SaNewCameliaProperty)
	owner := s.GetOwner()
	owner.TriggerQuestEvent(custom_id.QttNewCiMeLiaLevel, id, int64(nextLv))
	for _, posData := range data.SubPosDataMgr {
		owner.TriggerQuestEvent(custom_id.QttNewCiMeLiaSubPosLevel, posData.Pos, int64(posData.Level))
	}
	owner.TriggerQuestEventRange(custom_id.QttAnyNewCiMeLiaLevel)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogNewCameliaUpLevel, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d", nextLv),
	})
	return nil
}

func (s *NewCameliaSys) calcSubPosAttr(id uint32) {
	data := s.getSingleCameliaData(id, false)
	cameliaConf := jsondata.GetNewCameliaConf(id)
	if cameliaConf == nil {
		return
	}
	for _, posData := range data.SubPosDataMgr {
		posData.AttrMap = make(map[uint32]uint32)
		posConf := cameliaConf.SubPosMap[posData.Pos]
		if posConf == nil {
			continue
		}
		posLvConf := posConf.LvConf[posData.Level]
		if posLvConf == nil {
			continue
		}

		// 这一级的属性
		for _, attr := range posLvConf.Attrs {
			posData.AttrMap[attr.Type] += attr.Value
		}

		// 提前加下一级的属性
		nextLv := posConf.LvConf[posData.Level+1]
		if nextLv == nil {
			continue
		}
		for _, attr := range nextLv.Attrs {
			// 已经满经验 那就是下一级的属性
			if posData.FullExp {
				posData.AttrMap[attr.Type] = attr.Value
				continue
			}

			// 经验没满 那就是当前等级的经验进度的属性加百分比
			if nextLv.Exp != 0 && posData.Exp != 0 {

				// 对比前后两级属性取差值
				lastLvVal := posData.AttrMap[attr.Type]
				var diffVal uint32
				if lastLvVal < attr.Value {
					diffVal = attr.Value - lastLvVal
				}

				posData.AttrMap[attr.Type] += (diffVal * posData.Exp) / nextLv.Exp
			}
		}
	}
}

func calcNewCameliaProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiCimelia)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*NewCameliaSys)
	if !ok {
		return
	}
	data := sys.getData()
	for _, camelia := range data.CameliaDataMgr {
		cameliaConf := jsondata.GetNewCameliaConf(camelia.Id)
		if cameliaConf == nil {
			continue
		}

		// 灵符加属性
		cameliaLvConf := cameliaConf.LvConf[camelia.Level]
		if cameliaLvConf != nil {
			engine.CheckAddAttrsToCalc(player, calc, cameliaLvConf.Attrs)
		}

		// 子部位加属性
		for _, posData := range camelia.SubPosDataMgr {
			if posData.AttrMap == nil {
				continue
			}
			var attrs jsondata.AttrVec
			for typ, val := range posData.AttrMap {
				attrs = append(attrs, &jsondata.Attr{Type: typ, Value: val})
			}
			if len(attrs) == 0 {
				continue
			}
			engine.CheckAddAttrsToCalc(player, calc, attrs)
		}
	}
}

func init() {
	RegisterSysClass(sysdef.SiCimelia, func() iface.ISystem {
		return &NewCameliaSys{}
	})
	net.RegisterSysProtoV2(144, 11, sysdef.SiCimelia, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NewCameliaSys).c2sAddSubPosExp
	})
	net.RegisterSysProtoV2(144, 12, sysdef.SiCimelia, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NewCameliaSys).c2sUpLevel
	})
	engine.RegAttrCalcFn(attrdef.SaNewCameliaProperty, calcNewCameliaProperty)
	engine.RegQuestTargetProgress(custom_id.QttAnyNewCiMeLiaLevel, handleQttAnyNewCiMeLiaLevel)
}

func handleQttAnyNewCiMeLiaLevel(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	obj := player.GetSysObj(sysdef.SiCimelia)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	sys, ok := obj.(*NewCameliaSys)
	if !ok {
		return 0
	}
	if len(ids) != 1 {
		return 0
	}
	level := ids[0]
	var count uint32
	data := sys.getData()
	for _, info := range data.CameliaDataMgr {
		if info == nil {
			continue
		}
		if info.Level >= level {
			count++
		}
	}
	return count
}
