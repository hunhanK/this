/**
 * @Author: LvYuMeng
 * @Date: 2025/12/10
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type DiamondChargeTokens struct {
	Base
}

func (s *DiamondChargeTokens) getData() *pb3.DiamondChargeTokens {
	binary := s.GetBinaryData()
	if binary.DiamondChargeTokens == nil {
		binary.DiamondChargeTokens = &pb3.DiamondChargeTokens{}
	}
	return binary.DiamondChargeTokens
}

func (s *DiamondChargeTokens) OnInit() {
	binary := s.GetBinaryData()
	if binary.DiamondChargeTokens == nil {
		binary.DiamondChargeTokens = &pb3.DiamondChargeTokens{}
	}
}

func (s *DiamondChargeTokens) OnLogin() {
	data := s.getData()
	s.owner.SetExtraAttr(attrdef.DiamondChargeTokens, data.Tokens)
	s.owner.SetExtraAttr(attrdef.DiamondChargeTokensTemporaryLimit, data.TemporaryLimit)
	s.owner.SetExtraAttr(attrdef.DiamondChargeTokensUse, data.UseTokens)
}

func (s *DiamondChargeTokens) OnReconnect() {
	s.SendProto3(36, 11, &pb3.S2C_36_11{LastRewardTime: s.getData().LastRewardTime})
}

func (s *DiamondChargeTokens) OnAfterLogin() {
	s.SendProto3(36, 11, &pb3.S2C_36_11{LastRewardTime: s.getData().LastRewardTime})
}

func (s *DiamondChargeTokens) c2sUseTokens(msg *base.Message) error {
	var req pb3.C2S_36_7
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}
	chargeId := req.GetChargeId()
	conf := jsondata.GetChargeConf(chargeId)
	if nil == conf {
		return neterror.ParamsInvalidError("charge conf(%d) is nil", chargeId)
	}
	canCharge := engine.GetChargeCheckResult(conf.ChargeType, s.owner, conf)
	rsp := &pb3.S2C_36_7{ChargeId: chargeId, CanCharge: canCharge}

	if !canCharge {
		s.SendProto3(36, 7, rsp)
		return nil
	}
	result := s.checkDiamondChargeToken(chargeId)
	rsp.UseToken = result

	s.SendProto3(36, 7, rsp)
	return nil
}

func (s *DiamondChargeTokens) c2sDailyAward(msg *base.Message) error {
	var req pb3.C2S_36_11
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}
	conf := jsondata.GetDiamondChargeTokensConf()
	if nil == conf {
		return neterror.ConfNotFoundError("")
	}
	data := s.getData()
	nowSec := time_util.NowSec()
	if data.LastRewardTime > 0 {
		return neterror.ParamsInvalidError("received")
	}
	data.LastRewardTime = nowSec
	engine.GiveRewards(s.owner, conf.DailyAwards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogDiamonChargeTokensDailyAwards,
	})
	s.SendProto3(36, 11, &pb3.S2C_36_11{LastRewardTime: data.LastRewardTime})
	return nil
}

func (s *DiamondChargeTokens) GetPermanentLimit() (permanentLimit int64) {
	if conf := jsondata.GetDiamondChargeTokensConf(); nil != conf {
		permanentLimit += conf.InitPermanentLimit
	}
	privilegeLimit, _ := s.owner.GetPrivilege(privilegedef.EnumDiamondChargeTokensPermanentLimit)
	permanentLimit += privilegeLimit
	attrLimit := s.owner.GetFightAttr(attrdef.DiamondChargeTokensPermanentLimit)
	permanentLimit += attrLimit
	return
}

func (s *DiamondChargeTokens) subDiamondChargeTokensUse(tokens int64) bool {
	data := s.getData()
	if tokens < 0 || data.Tokens < tokens {
		return false
	}
	canUse := data.GetTemporaryLimit() + s.GetPermanentLimit() - data.GetUseTokens()
	if canUse < tokens {
		return false
	}

	data.UseTokens += tokens
	data.Tokens -= tokens

	s.owner.SetExtraAttr(attrdef.DiamondChargeTokens, data.Tokens)
	s.owner.SetExtraAttr(attrdef.DiamondChargeTokensUse, data.UseTokens)
	return true
}

func (s *DiamondChargeTokens) checkDiamondChargeToken(chargeId uint32) bool {
	conf := jsondata.GetChargeConf(chargeId)
	if conf == nil {
		s.LogError("charge conf(%d) is nil", chargeId)
		return false
	}
	if !conf.CanUseDiamondChargeToken {
		s.LogError("can't UseDiamondChargeToken chargeId:%d error", chargeId)
		return false
	}
	pay := s.subDiamondChargeTokensUse(int64(conf.CashCent))
	if !pay {
		s.LogError("subDiamondChargeTokensUse err")
		return false
	}
	var params = &pb3.OnChargeParams{
		ChargeId:           chargeId,
		CashCent:           conf.CashCent,
		SkipLogFirstCharge: true,
	}

	chargeSys, ok := s.owner.GetSysObj(sysdef.SiCharge).(*ChargeSys)
	if !ok {
		s.LogError("charge %d err charge sys get failed", chargeId)
		return false
	}
	chargeSys.OnCharge(params, pb3.LogId_LogChargeByUseDiamonTokens)

	return true
}

func (s *DiamondChargeTokens) onNewDay() {
	data := s.getData()
	data.TemporaryLimit = 0
	data.UseTokens = 0
	s.owner.SetExtraAttr(attrdef.DiamondChargeTokensTemporaryLimit, data.TemporaryLimit)
	s.owner.SetExtraAttr(attrdef.DiamondChargeTokensUse, data.UseTokens)
	data.LastRewardTime = 0
	s.SendProto3(36, 11, &pb3.S2C_36_11{LastRewardTime: data.LastRewardTime})
}

func init() {
	RegisterSysClass(sysdef.SiDiamondChargeTokens, func() iface.ISystem {
		return &DiamondChargeTokens{}
	})

	net.RegisterSysProtoV2(36, 7, sysdef.SiDiamondChargeTokens, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DiamondChargeTokens).c2sUseTokens
	})
	net.RegisterSysProtoV2(36, 11, sysdef.SiDiamondChargeTokens, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DiamondChargeTokens).c2sDailyAward
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if s, ok := player.GetSysObj(sysdef.SiDiamondChargeTokens).(*DiamondChargeTokens); ok && s.IsOpen() {
			s.onNewDay()
		}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseDiamondChargeTokensTemporaryLimit, func(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
		if len(conf.Param) < 1 {
			return
		}
		s, ok := player.GetSysObj(sysdef.SiDiamondChargeTokens).(*DiamondChargeTokens)
		if !ok || !s.IsOpen() {
			return
		}
		data := s.getData()
		addLimit := int64(conf.Param[0]) * param.Count
		data.TemporaryLimit += addLimit
		s.owner.SetExtraAttr(attrdef.DiamondChargeTokensTemporaryLimit, data.TemporaryLimit)
		return true, true, param.Count
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseDiamondChargeTokensAdd, func(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
		if len(conf.Param) < 1 {
			return
		}
		s, ok := player.GetSysObj(sysdef.SiDiamondChargeTokens).(*DiamondChargeTokens)
		if !ok || !s.IsOpen() {
			return
		}
		data := s.getData()
		data.Tokens += int64(conf.Param[0]) * param.Count
		s.owner.SetExtraAttr(attrdef.DiamondChargeTokens, data.Tokens)
		return true, true, param.Count
	})
}
