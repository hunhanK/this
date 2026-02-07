package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type FairyMagicSys struct {
	Base
}

func (sys *FairyMagicSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.FairyMagicData {
		binary.FairyMagicData = make(map[uint32]*pb3.FairyMagic)
	}
}

func (sys *FairyMagicSys) OnAfterLogin() {
	sys.S2CInfo()
}

func (sys *FairyMagicSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *FairyMagicSys) S2CInfo() {
	binary := sys.GetBinaryData()
	sys.SendProto3(27, 100, &pb3.S2C_27_100{FairyMagicData: binary.FairyMagicData})
}

func (sys *FairyMagicSys) GetData(mType uint32) *pb3.FairyMagic {
	binary := sys.GetBinaryData()
	if data, ok := binary.FairyMagicData[mType]; ok {
		return data
	}
	return nil
}

func (sys *FairyMagicSys) Active(mType, id uint32) *pb3.FairyMagic {
	binary := sys.GetBinaryData()
	data := binary.FairyMagicData[mType]
	if nil == data {
		data = &pb3.FairyMagic{Magic: make(map[uint32]bool)}
	}
	data.Magic[id] = true
	binary.FairyMagicData[mType] = data
	return nil
}

func (sys *FairyMagicSys) GetActiveCount(mType uint32) int {
	data := sys.GetData(mType)
	if nil == data {
		return 0
	}
	return len(data.Magic)
}

func (sys *FairyMagicSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_27_101
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	mType := req.GetType()
	typeConf := jsondata.GetFairyMagicConfByType(mType)
	if nil == typeConf {
		return neterror.ConfNotFoundError("fairy magic conf type(%d) is nil", mType)
	}
	if len(typeConf.Front) > 0 { //前置类型未激活
		for _, tid := range typeConf.Front {
			if tConf := jsondata.GetFairyMagicConfByType(tid); nil != tConf {
				if len(tConf.Magic) > sys.GetActiveCount(tid) {
					sys.owner.SendTipMsg(tipmsgid.TpNedActivePreFairyMagic)
					return nil
				}
			} else {
				return neterror.ConfNotFoundError("fairy magic conf type(%d) is nil", tid)
			}
		}
	}
	id := req.GetId()
	data := sys.GetData(mType)
	var conf *jsondata.FairyMagicAttrConf
	for _, v := range typeConf.Magic { //前置秘术未激活
		if v.Id < id {
			if nil == data || nil == data.Magic || !data.Magic[v.Id] {
				sys.owner.SendTipMsg(tipmsgid.TpNedActivePreFairyMagic)
				return nil
			}
		}
		if v.Id == id {
			conf = v
		}
	}
	if nil == conf {
		return neterror.ConfNotFoundError("fairy magic conf type(%d) id(%d) is nil", mType, id)
	}
	if !sys.owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyMagicActive}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	sys.Active(mType, id)
	sys.SendProto3(27, 101, &pb3.S2C_27_101{
		Type: mType,
		Id:   id,
	})
	if fairySys, ok := sys.owner.GetSysObj(sysdef.SiFairy).(*FairySystem); ok {
		if fairySys.battlePos > 0 {
			data := fairySys.GetData()
			if fairy, _ := fairySys.GetFairy(data.BattleFairy[fairySys.battlePos]); nil != fairy {
				binary := sys.GetBinaryData()
				sys.owner.CallActorFunc(actorfuncid.SyncFairyMagic, &pb3.SyncFairyMagic{Handle: fairy.GetHandle(), Magic: binary.FairyMagicData})
			}
		}
	}
	sys.ResetSysAttr(attrdef.SaFairyMagic)
	return nil
}

func calcSaFairyMagic(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFairyMagic).(*FairyMagicSys)
	if !ok || !sys.IsOpen() {
		return
	}
	data := player.GetBinaryData().FairyMagicData
	for mType, v := range data {
		if tpConf := jsondata.GetFairyMagicConfByType(mType); nil != tpConf {
			for _, attrConf := range tpConf.Magic {
				if nil != v.Magic && v.Magic[attrConf.Id] {
					calc.AddValue(attrdef.FairyMagicPower, attrdef.AttrValueAlias(attrConf.ActorFightVal))
				}
			}
		}
	}
}

func init() {
	RegisterSysClass(sysdef.SiFairyMagic, func() iface.ISystem {
		return &FairyMagicSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaFairyMagic, calcSaFairyMagic)

	gmevent.Register("fairyMagic", func(actor iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		sys, ok := actor.GetSysObj(sysdef.SiFairyMagic).(*FairyMagicSys)
		if !ok {
			return false
		}
		mType := utils.AtoUint32(args[0])
		id := utils.AtoUint32(args[1])
		sys.Active(mType, id)
		sys.SendProto3(27, 101, &pb3.S2C_27_101{
			Type: mType,
			Id:   id,
		})
		if fairySys, ok := sys.owner.GetSysObj(sysdef.SiFairy).(*FairySystem); ok {
			if fairySys.battlePos > 0 {
				data := fairySys.GetData()
				if fairy, _ := fairySys.GetFairy(data.BattleFairy[fairySys.battlePos]); nil != fairy {
					binary := sys.GetBinaryData()
					sys.owner.CallActorFunc(actorfuncid.SyncFairyMagic, &pb3.SyncFairyMagic{Handle: fairy.GetHandle(), Magic: binary.FairyMagicData})
				}
			}
		}
		return true
	}, 1)
	net.RegisterSysProto(27, 101, sysdef.SiFairyMagic, (*FairyMagicSys).c2sActive)
}
