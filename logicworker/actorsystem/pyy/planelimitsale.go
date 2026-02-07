package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

const (
	GoodRefreshTypeDaily = 1 // 每日刷新
	GoodRefreshTypeAct   = 2 // 活动期间不会变
)

const DaySec = 24 * 60 * 60

type PlayerYYPlaneLimitSale struct {
	PlayerYYBase
}

func (s *PlayerYYPlaneLimitSale) Login() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *PlayerYYPlaneLimitSale) OnOpen() {
	if s.GetYYData().PlaneLimitSale != nil {
		delete(s.GetYYData().PlaneLimitSale, s.GetId())
	}
	conf := s.GetConf()
	if conf == nil {
		return
	}
	for k := range conf.Goods {
		data := s.GetDataByType(k)
		data.Remind = true
	}
	s.S2CInfo()
}

// 活动结束发送累积奖励
func (s *PlayerYYPlaneLimitSale) OnEnd() {
	s.GetPlayer().LogInfo("PlayerYYPlaneLimitSale activity end: %d", s.GetId())
	s.TrySendMail()
}

func (s *PlayerYYPlaneLimitSale) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

// 每日重置每日商品购买数量
func (s *PlayerYYPlaneLimitSale) NewDay() {
	if !s.IsOpen() {
		return
	}
	s.ResetGoods()
	s.S2CInfo()
}

func (s *PlayerYYPlaneLimitSale) ResetGoods() {
	conf := s.GetConf()
	if conf == nil {
		return
	}
	data := s.GetData()
	for _, v := range data.Data {
		gConf := conf.Goods[v.Type]
		if gConf == nil {
			continue
		}
		for id := range v.BoughtData {
			ggConf, ok := gConf[id]
			if !ok {
				continue
			}
			if ggConf.RefreshType == GoodRefreshTypeDaily {
				v.BoughtData[id] = 0
			}
		}
	}
}

func (s *PlayerYYPlaneLimitSale) CheckAccAwardsExpired() bool {
	conf := s.GetConf()
	return time_util.NowSec() >= s.GetOpenTime()+conf.AccAwardsDur*DaySec
}

func (s *PlayerYYPlaneLimitSale) TrySendMail() {
	conf := s.GetConf()
	if conf == nil {
		return
	}
	if s.CheckAccAwardsExpired() {
		data := s.GetData()
		var rewards jsondata.StdRewardVec
		var getIds []uint32
		for k, v := range data.Data {
			accConf, ok := conf.AccAwards[k]
			if !ok {
				continue
			}
			for id, aConf := range accConf {
				if utils.SliceContainsUint32(v.Ids, id) {
					continue
				}
				if v.AccMoney >= uint64(aConf.AccValue) {
					rewards = append(rewards, aConf.Rewards...)
					getIds = append(getIds, id)
				}
			}
			// 设置领取标记
			v.Ids = append(v.Ids, getIds...)
		}

		// 发送邮件
		if len(rewards) > 0 {
			s.GetPlayer().SendMail(&mailargs.SendMailSt{
				ConfId:  common.Mail_PlanLimitSaleAccAwards,
				Rewards: rewards,
			})
		}
	}
}

func (s *PlayerYYPlaneLimitSale) GetData() *pb3.PYY_PlaneLimitSale {
	if s.GetYYData().PlaneLimitSale == nil {
		s.GetYYData().PlaneLimitSale = make(map[uint32]*pb3.PYY_PlaneLimitSale)
	}

	data, ok := s.GetYYData().PlaneLimitSale[s.GetId()]
	if !ok {
		data = &pb3.PYY_PlaneLimitSale{}
		s.GetYYData().PlaneLimitSale[s.GetId()] = data
	}

	if data.Data == nil {
		data.Data = make(map[uint32]*pb3.PYY_PlaneLimitSaleSingle)
	}
	return data
}

func (s *PlayerYYPlaneLimitSale) ResetData() {
	if s.GetYYData().PlaneLimitSale == nil {
		return
	}
	delete(s.GetYYData().PlaneLimitSale, s.Id)
}

func (s *PlayerYYPlaneLimitSale) GetDataByType(ty uint32) *pb3.PYY_PlaneLimitSaleSingle {
	data := s.GetData()
	if data.Data == nil {
		data.Data = make(map[uint32]*pb3.PYY_PlaneLimitSaleSingle)
	}
	dData, ok := data.Data[ty]
	if !ok {
		dData = &pb3.PYY_PlaneLimitSaleSingle{}
		dData.Type = ty
		data.Data[ty] = dData
	}
	if dData.BoughtData == nil {
		dData.BoughtData = make(map[uint32]uint32)
	}
	return dData
}

func (s *PlayerYYPlaneLimitSale) S2CInfo() {
	s.SendProto3(69, 20, &pb3.S2C_69_20{
		ActiveId: s.GetId(),
		Info:     s.GetData(),
	})
}

func (s *PlayerYYPlaneLimitSale) GetConf() *jsondata.PYYPlaneLimitSaleConf {
	return jsondata.GetPYYPlaneLimitSaleConf(s.ConfName, s.GetConfIdx())
}

func (s *PlayerYYPlaneLimitSale) c2sBuy(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}

	var req pb3.C2S_69_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf := s.GetConf()
	if conf == nil || conf.Goods == nil {
		return neterror.ConfNotFoundError("get conf failed actId %d", s.GetId())
	}
	if _, ok := conf.Goods[req.Type]; !ok {
		return neterror.ConfNotFoundError("get conf failed (actId:%d, goodType:%d)", s.GetId(), req.Type)
	}

	gShopConf, _ := conf.Goods[req.Type]
	if _, ok := gShopConf[req.GoodId]; !ok {
		return neterror.ConfNotFoundError("get conf failed (actId:%d, goodType:%d, goodId:%d)", s.GetId(), req.Type, req.GoodId)
	}

	data := s.GetDataByType(req.Type)
	goodConf, _ := gShopConf[req.GoodId]
	if data.BoughtData[req.GoodId]+req.Num > goodConf.BuyLimit {
		return neterror.ConfNotFoundError("buy count limit (actId:%d, goodType:%d, goodId:%d)", s.GetId(), req.Type, req.GoodId)
	}

	money := goodConf.Money * req.Num
	if !s.GetPlayer().DeductMoney(goodConf.MoneyType, int64(money), common.ConsumeParams{LogId: pb3.LogId_LogPlaneLimitSaleBuyConsume}) {
		return neterror.ParamsInvalidError("money not enough")
	}

	data.BoughtData[req.GoodId] += req.Num
	data.AccMoney += uint64(money)
	rewards := jsondata.StdRewardMulti(goodConf.Rewards, int64(req.Num))

	if !engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPlaneLimitSaleBuyAward,
	}) {
		return neterror.InternalError("rewards failed (actId:%d, goodType:%d, goodId:%d)", s.GetId(), req.Type, req.GoodId)
	}

	s.SendProto3(69, 21, &pb3.S2C_69_21{
		ActiveId: s.GetId(),
		BuyData: &pb3.PYY_PlaneLimitSaleSingle{
			Type:       req.Type,
			AccMoney:   data.AccMoney,
			BoughtData: data.BoughtData,
		},
	})

	return nil
}

func (s *PlayerYYPlaneLimitSale) c2sSetRemind(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}

	var req pb3.C2S_69_22
	if err := msg.UnPackPb3Msg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	data := s.GetDataByType(req.Type)
	data.Remind = req.Remind

	s.SendProto3(69, 22, &pb3.S2C_69_22{
		ActiveId: s.GetId(),
		Type:     req.Type,
		Remind:   req.Remind,
	})

	return nil
}

func (s *PlayerYYPlaneLimitSale) c2sFetchAccReward(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}

	var req pb3.C2S_69_23
	if err := msg.UnPackPb3Msg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf := s.GetConf()
	if conf == nil || conf.AccAwards == nil {
		return neterror.ConfNotFoundError("get conf failed actId %d", s.GetId())
	}

	// 累计消耗奖励时间已过
	if s.CheckAccAwardsExpired() {
		return neterror.ParamsInvalidError("act expired actId:%d", s.GetId())
	}

	if _, ok := conf.AccAwards[req.Type]; !ok {
		return neterror.ConfNotFoundError("get conf failed (actId:%d, goodType:%d)", s.GetId(), req.Type)
	}

	data := s.GetDataByType(req.Type)
	gAccConf, _ := conf.AccAwards[req.Type]
	var rewards jsondata.StdRewardVec
	if req.Id == 0 {
		for k, v := range gAccConf {
			if utils.SliceContainsUint32(data.Ids, k) {
				continue
			}
			if data.AccMoney >= uint64(v.AccValue) {
				rewards = append(rewards, v.Rewards...)
				data.Ids = append(data.Ids, k)
			}
		}
	} else {
		accConf, ok := gAccConf[req.Id]
		if !ok || data.AccMoney < uint64(accConf.AccValue) {
			return neterror.ConfNotFoundError("fetch limit (actId:%d, goodType:%d, id:%d)", s.GetId(), req.Type, req.Id)
		}
		if utils.SliceContainsUint32(data.Ids, req.Id) {
			return neterror.ConfNotFoundError("awards fetched (actId:%d, goodType:%d, id:%d)", s.GetId(), req.Type, req.Id)
		}
		rewards = accConf.Rewards
		data.Ids = append(data.Ids, req.Id)
	}

	if !engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPlaneLimitSaleAccAward,
	}) {
		return neterror.InternalError("rewards failed (actId:%d, goodType:%d, id:%d)", s.GetId(), req.Type, req.Id)
	}

	s.SendProto3(69, 23, &pb3.S2C_69_23{
		ActiveId: s.GetId(),
		Type:     req.Type,
		Ids:      data.Ids,
	})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYPlaneLimitSale, func() iface.IPlayerYY {
		return &PlayerYYPlaneLimitSale{}
	})

	net.RegisterYYSysProtoV2(69, 21, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerYYPlaneLimitSale).c2sBuy
	})
	net.RegisterYYSysProtoV2(69, 22, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerYYPlaneLimitSale).c2sSetRemind
	})
	net.RegisterYYSysProtoV2(69, 23, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerYYPlaneLimitSale).c2sFetchAccReward
	})
}
