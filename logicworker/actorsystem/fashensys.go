/**
 * @Author:
 * @Date:
 * @Desc:
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
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type FaShenSys struct {
	Base
}

func (s *FaShenSys) s2cInfo() {
	s.SendProto3(9, 10, &pb3.S2C_9_10{
		Data: s.getData(),
	})
}

func (s *FaShenSys) getData() *pb3.FaShenData {
	data := s.GetBinaryData().FaShenData
	if data == nil {
		s.GetBinaryData().FaShenData = &pb3.FaShenData{}
		data = s.GetBinaryData().FaShenData
	}
	if data.FsMap == nil {
		data.FsMap = make(map[uint32]*pb3.FaShen)
	}
	return data
}

func (s *FaShenSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FaShenSys) OnLogin() {
	s.s2cInfo()
}

func (s *FaShenSys) OnOpen() {
	s.s2cInfo()
}

func (s *FaShenSys) getFaShen(id uint32) *pb3.FaShen {
	data := s.getData()
	if data.FsMap[id] == nil {
		return nil
	}
	return data.FsMap[id]
}

func (s *FaShenSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_9_11
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	id := req.Id
	faShen := s.getFaShen(id)
	if faShen != nil {
		return neterror.ParamsInvalidError("%d already unlock", id)
	}

	fsConfig := jsondata.GetFaShenConfig(id)
	if fsConfig == nil {
		return neterror.ConfNotFoundError("%d not found conf", id)
	}

	owner := s.GetOwner()
	if len(fsConfig.Active) == 0 || !owner.ConsumeByConf(fsConfig.Active, false, common.ConsumeParams{LogId: pb3.LogId_LogFaShenActive}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	data := s.getData()
	data.FsMap[id] = &pb3.FaShen{
		Id:   id,
		Star: 0,
	}

	s.SendProto3(9, 11, &pb3.S2C_9_11{
		Fs: data.FsMap[id],
	})
	s.ResetSysAttr(attrdef.SaFaShen)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogFaShenActive, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
	})
	s.owner.TriggerQuestEvent(custom_id.QttFaShenActive, 0, 1)
	return nil
}

func (s *FaShenSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_9_12
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	id := req.Id
	faShen := s.getFaShen(id)
	if faShen == nil {
		return neterror.ParamsInvalidError("%d not active", id)
	}

	fsConfig := jsondata.GetFaShenConfig(id)
	if fsConfig == nil {
		return neterror.ConfNotFoundError("%d not found conf", id)
	}

	nextStar := faShen.Star + 1
	if fsConfig.StarConf == nil || fsConfig.StarConf[nextStar] == nil {
		return neterror.ConfNotFoundError("%d %d not found star conf", id, nextStar)
	}
	nextStarConf := fsConfig.StarConf[nextStar]

	owner := s.GetOwner()
	if len(nextStarConf.Consume) == 0 || !owner.ConsumeByConf(nextStarConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFaShenUpStar}) {
		return neterror.ConsumeFailedError("consume failed")
	}
	faShen.Star = nextStar
	s.SendProto3(9, 12, &pb3.S2C_9_12{
		Id:   id,
		Star: nextStar,
	})
	s.ResetSysAttr(attrdef.SaFaShen)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogFaShenUpStar, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d", nextStar),
	})
	return nil
}

func (s *FaShenSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	owner := s.GetOwner()
	for _, faShen := range data.FsMap {
		config := jsondata.GetFaShenConfig(faShen.Id)
		if config == nil || config.StarConf == nil {
			return
		}
		faShenStarConf := config.StarConf[faShen.Star]
		if faShenStarConf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(owner, calc, faShenStarConf.Attrs)
	}
}

func calcFaShenAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiFaShen)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*FaShenSys)
	if !ok {
		return
	}
	sys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiFaShen, func() iface.ISystem {
		return &FaShenSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaFaShen, calcFaShenAttr)
	net.RegisterSysProtoV2(9, 11, sysdef.SiFaShen, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaShenSys).c2sActive
	})
	net.RegisterSysProtoV2(9, 12, sysdef.SiFaShen, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaShenSys).c2sUpStar
	})
}
