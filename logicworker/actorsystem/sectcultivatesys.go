/**
 * @Author: LvYuMeng
 * @Date: 2024/12/17
 * @Desc: 宗门修炼
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type SectCultivateSys struct {
	Base
}

func (s *SectCultivateSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *SectCultivateSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SectCultivateSys) s2cInfo() {
	s.SendProto3(167, 10, &pb3.S2C_167_10{Data: s.getData()})
}

func (s *SectCultivateSys) getData() *pb3.SectCultivateData {
	binary := s.GetBinaryData()
	if nil == binary.SectCultivateData {
		binary.SectCultivateData = &pb3.SectCultivateData{}
	}
	if nil == binary.SectCultivateData.Level {
		binary.SectCultivateData.Level = make(map[uint32]uint32)
	}
	return binary.SectCultivateData
}

func (s *SectCultivateSys) c2sLvUp(msg *base.Message) error {
	var req pb3.C2S_167_11
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	data := s.getData()
	nextLevel := data.Level[req.Id] + 1

	nextConf, ok := jsondata.GetSectCultivateLevelConf(req.Id, nextLevel)
	if !ok {
		return neterror.ConfNotFoundError("SectCultivate id:%d, level:%d conf is nil", req.Id, nextLevel)
	}

	for _, v := range nextConf.Cond {
		if !CheckReach(s.owner, v.Type, v.Val) {
			return neterror.ParamsInvalidError("cond not reach")
		}
	}

	if !s.owner.ConsumeByConf(nextConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSectCultivateLevelUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.Level[req.Id] = nextLevel
	s.SendProto3(167, 11, &pb3.S2C_167_11{Id: req.Id, Level: nextLevel})

	s.ResetSysAttr(attrdef.SaSectCultivate)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSectCultivateLevelUp, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Id),
		StrArgs: fmt.Sprintf("%d", nextLevel),
	})
	return nil
}

func (s *SectCultivateSys) GetLevel(id uint32) uint32 {
	data := s.getData()
	return data.Level[id]
}

func (s *SectCultivateSys) calAttrs(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	for id, level := range data.Level {
		lvConf, ok := jsondata.GetSectCultivateLevelConf(id, level)
		if !ok {
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, lvConf.Attrs)
	}
}

func calcSectCultivateAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s := player.GetSysObj(sysdef.SiSectCultivate).(*SectCultivateSys)
	if !s.IsOpen() {
		return
	}

	s.calAttrs(calc)
}

func init() {
	RegisterSysClass(sysdef.SiSectCultivate, func() iface.ISystem {
		return &SectCultivateSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaSectCultivate, calcSectCultivateAttr)

	net.RegisterSysProtoV2(167, 11, sysdef.SiSectCultivate, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectCultivateSys).c2sLvUp
	})
}
