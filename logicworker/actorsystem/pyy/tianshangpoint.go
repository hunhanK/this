/**
 * @Author: lzp
 * @Date: 2024/6/27
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

const (
	RefreshType0 = 0 // 永久
	RefreshType1 = 1 // 每日
	RefreshType2 = 2 // 每周
)

type TShangPointSys struct {
	PlayerYYBase
}

func (s *TShangPointSys) OnReconnect() {
	s.s2cInfo()
}

func (s *TShangPointSys) Login() {
	s.s2cInfo()
}

func (s *TShangPointSys) OnOpen() {
	data := s.GetData()
	data.RefreshTime = time_util.NowSec()
	s.s2cInfo()
}

func (s *TShangPointSys) OnEnd() {
	// 活动结束,货币按照比例转为道具
	conf := jsondata.GetPYYTShangPointConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	point := s.GetPoint()
	var rewards jsondata.StdRewardVec
	rewards = append(rewards, &jsondata.StdReward{
		Id:    conf.P2ItemId,
		Count: int64(conf.P2ItemCount) * point,
	})
	s.GetPlayer().SendMail(&mailargs.SendMailSt{
		ConfId:  common.Mail_YYTShangPoint2ItemAwards,
		Rewards: rewards,
	})
	s.ClearPoint()
}

func (s *TShangPointSys) NewDay() {
	// 每日刷新
	s.dayRefresh()

	// 每周刷新
	now := time_util.NowSec()
	data := s.GetData()
	if !time_util.IsSameWeek(now, data.RefreshTime) {
		s.weekRefresh()
		data.RefreshTime = now
	}

	s.s2cInfo()
}

func (s *TShangPointSys) dayRefresh() {
	data := s.GetData()
	for k := range data.BuyData {
		conf := jsondata.GetPYYTShangPointGoodConf(s.ConfName, s.ConfIdx, k)
		if conf != nil && conf.RefreshType == RefreshType1 {
			data.BuyData[k] = 0
		}
	}
}

func (s *TShangPointSys) weekRefresh() {
	data := s.GetData()
	for k := range data.BuyData {
		conf := jsondata.GetPYYTShangPointGoodConf(s.ConfName, s.ConfIdx, k)
		if conf != nil && conf.RefreshType == RefreshType2 {
			data.BuyData[k] = 0
		}
	}
}

func (s *TShangPointSys) s2cInfo() {
	s.SendProto3(127, 80, &pb3.S2C_127_80{
		ActId: s.GetId(),
		Data:  s.GetData(),
	})
}

func (s *TShangPointSys) ResetData() {
	if s.GetYYData().TShangPoint == nil {
		return
	}
	delete(s.GetYYData().TShangPoint, s.Id)
}
func (s *TShangPointSys) GetData() *pb3.PYY_TShangPoint {
	if s.GetYYData().TShangPoint == nil {
		s.GetYYData().TShangPoint = make(map[uint32]*pb3.PYY_TShangPoint)
	}

	data, ok := s.GetYYData().TShangPoint[s.GetId()]
	if !ok {
		data = &pb3.PYY_TShangPoint{}
		s.GetYYData().TShangPoint[s.GetId()] = data
	}
	if data.BuyData == nil {
		data.BuyData = make(map[uint32]uint32)
	}
	return data
}

func (s *TShangPointSys) GetPoint() int64 {
	return s.GetPlayer().GetMoneyCount(moneydef.TSPoint)
}

func (s *TShangPointSys) AddPoint(num int64) {
	data := s.GetData()
	data.AccPoint += uint32(num)
}

func (s *TShangPointSys) ClearPoint() {
	count := s.GetPlayer().GetMoneyCount(moneydef.TSPoint)
	s.GetPlayer().DeductMoney(moneydef.TSPoint, count, common.ConsumeParams{
		LogId: pb3.LogId_LogTShangPointActEndClear,
	})
}

func (s *TShangPointSys) c2sBuy(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}

	var req pb3.C2S_127_81
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYTShangPointGoodConf(s.ConfName, s.ConfIdx, req.Id)
	if conf == nil {
		return neterror.ParamsInvalidError("get conf err actId=%d id=%d", s.GetId(), req.Id)
	}

	data := s.GetData()
	if data.AccPoint < conf.UnLockPoint {
		return neterror.ParamsInvalidError("point limit actId=%d id=%d", s.GetId(), req.Id)
	}

	buyCount, _ := data.BuyData[req.Id]
	if buyCount+req.Count > conf.CountLimit {
		return neterror.ParamsInvalidError("count limit actId=%d id=%d", s.GetId(), req.Id)
	}

	openDay := gshare.GetOpenServerDay()
	if len(conf.OpenDay) >= 2 {
		if openDay < conf.OpenDay[0] || openDay > conf.OpenDay[1] {
			return neterror.ParamsInvalidError("openDay limit actId=%d id=%d", s.GetId(), req.Id)
		}
	}

	consumes := jsondata.ConsumeMulti(conf.Consume, req.Count)
	if !s.GetPlayer().ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogTShangPointBuyConsume}) {
		return neterror.ParamsInvalidError("consumes not enough")
	}

	data.BuyData[req.Id] += req.Count
	reward := &jsondata.StdReward{
		Id:    conf.ItemId,
		Count: int64(conf.ItemCount * req.Count),
	}
	engine.GiveRewards(s.GetPlayer(), []*jsondata.StdReward{reward}, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogTShangPointBuyAwards,
	})

	s.GetPlayer().SendProto3(127, 81, &pb3.S2C_127_81{
		ActId: s.GetId(),
		Id:    req.Id,
		Count: req.Count,
	})
	return nil
}

func onMoneyChange(player iface.IPlayer, args ...interface{}) {
	if len(args) < 2 {
		return
	}
	mt, ok := args[0].(uint32)
	if !ok || mt != moneydef.TSPoint {
		return
	}
	count, ok := args[1].(int64)
	if !ok || count <= 0 {
		return
	}

	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYTShangPoint)
	for _, obj := range yyList {
		if s, ok := obj.(*TShangPointSys); ok && s.IsOpen() {
			s.AddPoint(count)
			s.SendProto3(127, 82, &pb3.S2C_127_82{
				ActId: s.GetId(),
				Data:  &pb3.PYY_TShangPoint{AccPoint: s.GetData().AccPoint},
			})
		}
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYTShangPoint, func() iface.IPlayerYY {
		return &TShangPointSys{}
	})

	net.RegisterYYSysProtoV2(127, 81, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*TShangPointSys).c2sBuy
	})

	event.RegActorEvent(custom_id.AeMoneyChange, onMoneyChange)
}
