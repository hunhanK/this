/**
 * @Author: zjj
 * @Date: 2024/11/18
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type NewDiamondBenefitSys struct {
	Base
}

func (s *NewDiamondBenefitSys) s2cInfo() {
	s.SendProto3(41, 20, &pb3.S2C_41_20{
		Data: s.getData(),
	})
}

func (s *NewDiamondBenefitSys) getData() *pb3.NewDiamondBenefitData {
	data := s.GetBinaryData().NewDiamondBenefitData
	if data == nil {
		s.GetBinaryData().NewDiamondBenefitData = &pb3.NewDiamondBenefitData{}
		data = s.GetBinaryData().NewDiamondBenefitData
	}
	return data
}

func (s *NewDiamondBenefitSys) OnReconnect() {
	checkNextRefreshStoreAt(s.owner)
	s.s2cInfo()
}

func (s *NewDiamondBenefitSys) OnLogin() {
	checkNextRefreshStoreAt(s.owner)
	s.s2cInfo()
}

func (s *NewDiamondBenefitSys) OnOpen() {
	checkNextRefreshStoreAt(s.owner)
	s.s2cInfo()
}

// 因为会出现商城开了 这玩意儿没开 但是业务控制刷新 之前是应用在个人运营活动去刷商城 为了应用这个功能只能这样加了
func checkNextRefreshStoreAt(player iface.IPlayer) {
	data := player.GetBinaryData()
	defer func() {
		player.SendProto3(41, 22, &pb3.S2C_41_22{
			NextRefreshStoreAt: data.NextRefreshStoreAt,
		})
	}()
	zeroTime := time_util.GetBeforeDaysZeroTime(0)
	conf := jsondata.GetNewDiamondBenefitConf()
	if conf == nil {
		return
	}
	if data.NextRefreshStoreAt != 0 && data.NextRefreshStoreAt > zeroTime {
		return
	}
	if data.NextRefreshStoreAt != 0 {
		for subType := range custom_id.NewDiamondStoreSubTypeSet {
			player.ResetSpecCycleBuy(custom_id.StoreTypeNewDiamond, subType)
		}
	}
	data.NextRefreshStoreAt = zeroTime + conf.RefreshDay*86400 - 1
}

func (s *NewDiamondBenefitSys) c2sExchangeCode(msg *base.Message) error {
	var req pb3.C2S_41_21
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	if pie.Strings(data.UseExchangeCodes).Contains(req.ExchangeCode) {
		return neterror.ParamsInvalidError("%s already exchange", req.ExchangeCode)
	}
	conf := jsondata.GetNewDiamondBenefitConf()
	if conf == nil {
		return neterror.ConfNotFoundError("NewDiamondBenefitConf not found")
	}
	var exchangeConf *jsondata.NewDiamondBenefitExchangeCode
	for _, ec := range conf.ExchangeCode {
		if ec.Code != req.ExchangeCode {
			continue
		}
		exchangeConf = ec
		break
	}
	if exchangeConf == nil {
		return neterror.ConfNotFoundError("NewDiamondBenefitExchangeCode %s not found", req.ExchangeCode)
	}

	nowSec := time_util.NowSec()
	if data.NextExchangeAt != 0 && data.NextExchangeAt > nowSec {
		return neterror.ParamsInvalidError("please wait %d - %d seconds ", nowSec, data.NextExchangeAt)
	}

	data.UseExchangeCodes = append(data.UseExchangeCodes, req.ExchangeCode)
	data.NextExchangeAt = nowSec + conf.ExchangeCodeCd
	if len(exchangeConf.Rewards) > 0 {
		engine.GiveRewards(s.GetOwner(), exchangeConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogNewDiamondBenefitExchangeCode,
		})
	}
	s.SendProto3(41, 21, &pb3.S2C_41_21{
		ExchangeCode:   req.ExchangeCode,
		NextExchangeAt: data.NextExchangeAt,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogNewDiamondBenefitExchangeCode, &pb3.LogPlayerCounter{
		StrArgs: req.ExchangeCode,
	})
	return nil
}

func handleNewDiamondBenefitCharge(player iface.IPlayer, args ...interface{}) {
	owner := player
	if len(args) < 1 {
		player.LogError("player %d charge params get err,args %v", owner.GetId(), args)
		return
	}
	chargeEvent, ok := args[0].(*custom_id.ActorEventCharge)
	if !ok {
		return
	}
	benefitConf := jsondata.GetNewDiamondBenefitConf()
	if benefitConf == nil {
		player.LogError("not found new diamond benefit conf")
		return
	}
	for _, diamondConf := range benefitConf.ReturnDiamond {
		if diamondConf.ChargeId != chargeEvent.ChargeId {
			continue
		}
		var count int64
		if len(diamondConf.Rewards) > 0 {
			count = diamondConf.Rewards[0].Count
		}
		mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_NewDiamondBenefitReturnDiamonds,
			Rewards: diamondConf.Rewards,
			Content: &mailargs.CommonMailArgs{
				Str1: fmt.Sprintf("%d", chargeEvent.CashCent/100),
				Str2: fmt.Sprintf("%d", count),
			},
		})
		logworker.LogPlayerBehavior(player, pb3.LogId_LogNewDiamondBenefitReturnDiamond, &pb3.LogPlayerCounter{
			NumArgs: uint64(chargeEvent.ChargeId),
		})
		break
	}
}

func init() {
	RegisterSysClass(sysdef.SiNewDiamondBenefit, func() iface.ISystem {
		return &NewDiamondBenefitSys{}
	})
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		checkNextRefreshStoreAt(player)
	})
	event.RegActorEvent(custom_id.AeCharge, func(player iface.IPlayer, args ...interface{}) {
		handleNewDiamondBenefitCharge(player, args...)
	})
	net.RegisterSysProtoV2(41, 21, sysdef.SiNewDiamondBenefit, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NewDiamondBenefitSys).c2sExchangeCode
	})
	gmevent.Register("ndb.exchangeCode", func(player iface.IPlayer, args ...string) bool {
		msg, err := base.MakeMessage(41, 21, &pb3.C2S_41_21{
			ExchangeCode: args[0],
		})
		if err != nil {
			player.LogError("err:%v", err)
			return false
		}
		player.DoNetMsg(41, 21, msg)
		return true
	}, 1)
	gmevent.Register("ndb.checkNextRefreshStoreAt", func(player iface.IPlayer, args ...string) bool {
		player.GetBinaryData().NextRefreshStoreAt = time_util.GetBeforeDaysZeroTime(1)
		checkNextRefreshStoreAt(player)
		return true
	}, 1)
	gmevent.Register("ndb.clear", func(player iface.IPlayer, args ...string) bool {
		sys := player.GetSysObj(sysdef.SiNewDiamondBenefit).(*NewDiamondBenefitSys)
		if sys == nil {
			return false
		}
		sys.getData().NextExchangeAt = 0
		sys.getData().UseExchangeCodes = nil
		sys.s2cInfo()
		return true
	}, 1)
}
