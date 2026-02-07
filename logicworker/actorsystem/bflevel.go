package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type BfLevelSys struct {
	Base
}

func (sys *BfLevelSys) OnInit() {
}

func (sys *BfLevelSys) GetData() *pb3.BenefitLevel {
	binary := sys.GetBinaryData()
	if nil == binary.Benefit {
		binary.Benefit = &pb3.BenefitData{}
	}
	if nil == binary.Benefit.Level {
		binary.Benefit.Level = &pb3.BenefitLevel{}
	}
	return binary.Benefit.Level
}

func (sys *BfLevelSys) S2CInfo() {
	sys.SendProto3(41, 12, &pb3.S2C_41_12{Level: sys.GetData()})
}

func (sys *BfLevelSys) OnAfterLogin() {
	sys.S2CInfo()
}

func (sys *BfLevelSys) OnReconnect() {
	sys.S2CInfo()
}

const (
	bfLevelAwardType   = 1
	bfLevelExAwardType = 2
)

func (sys *BfLevelSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_41_4
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.Wrap(err)
	}
	id := req.GetId()
	awardType := req.GetType()
	if awardType > bfLevelExAwardType {
		return neterror.ParamsInvalidError("the benefit award type(%d) is not exist", awardType)
	}
	var send bool
	var err error
	switch awardType {
	case bfLevelAwardType:
		send, err = sys.levelAward(id, false)
	case bfLevelExAwardType:
		send, err = sys.levelAward(id, true)
	}
	if send {
		sys.SendProto3(41, 4, &pb3.S2C_41_4{Type: awardType, Id: id})
		return nil
	}
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (sys *BfLevelSys) levelAward(level uint32, isVip bool) (bool, error) {
	conf := jsondata.GetBenefitLevelConf(level)
	if nil == conf {
		return false, neterror.ConfNotFoundError("benefit level conf(%d) is nil", level)
	}
	if sys.owner.GetLevel() < level {
		sys.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return false, nil
	}
	data := sys.GetData()
	if isVip {
		if sys.owner.GetVipLevel() < conf.VipLimit {
			sys.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
			return false, nil
		}
		if utils.SliceContainsUint32(data.LevelExAward, level) {
			sys.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
			return false, nil
		}
		data.LevelExAward = append(data.LevelExAward, level)
		engine.GiveRewards(sys.owner, conf.VipAward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBenefitLevel})
	} else {
		if utils.SliceContainsUint32(data.LevelAward, level) {
			sys.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
			return false, nil
		}
		data.LevelAward = append(data.LevelAward, level)
		engine.GiveRewards(sys.owner, conf.Award, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBenefitLevelEx})
	}
	return true, nil
}

func init() {
	RegisterSysClass(sysdef.SiBenefitLevel, func() iface.ISystem {
		return &BfLevelSys{}
	})

	net.RegisterSysProto(41, 4, sysdef.SiBenefitLevel, (*BfLevelSys).c2sAward)

}
