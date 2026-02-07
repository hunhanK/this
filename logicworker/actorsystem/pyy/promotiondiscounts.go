/**
 * @Author: LvYuMeng
 * @Date: 2025/6/28
 * @Desc: 冲榜特惠
**/

package pyy

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type PromotionDiscountsSys struct {
	PlayerYYBase
}

func (s *PromotionDiscountsSys) OnOpen() {
	s.clearRecord()
	s.s2cInfo()
}

func (s *PromotionDiscountsSys) ResetData() {
	state := s.GetYYData()
	if nil == state.PromotionDiscounts {
		return
	}
	delete(state.PromotionDiscounts, s.GetId())
}

func (s *PromotionDiscountsSys) Login() {
	s.s2cInfo()
}

func (s *PromotionDiscountsSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PromotionDiscountsSys) s2cInfo() {
	s.SendProto3(47, 30, &pb3.S2C_47_30{
		ActiveId: s.GetId(),
		Gift:     s.getData().Gift,
		OpenDay:  s.GetOpenDay(),
	})
}

func (s *PromotionDiscountsSys) getData() *pb3.PYY_PromotionDiscounts {
	state := s.GetYYData()
	if nil == state.PromotionDiscounts {
		state.PromotionDiscounts = make(map[uint32]*pb3.PYY_PromotionDiscounts)
	}
	if state.PromotionDiscounts[s.Id] == nil {
		state.PromotionDiscounts[s.Id] = &pb3.PYY_PromotionDiscounts{}
	}
	if nil == state.PromotionDiscounts[s.Id].Gift {
		state.PromotionDiscounts[s.Id].Gift = make(map[uint32]uint32)
	}
	return state.PromotionDiscounts[s.Id]
}

func (s *PromotionDiscountsSys) globalRecord() *pb3.PromotionDiscountsRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.PromotionDiscountsRecordList == nil {
		globalVar.PyyDatas.PromotionDiscountsRecordList = make(map[uint32]*pb3.PromotionDiscountsRecordList)
	}
	if globalVar.PyyDatas.PromotionDiscountsRecordList[s.Id] == nil {
		globalVar.PyyDatas.PromotionDiscountsRecordList[s.Id] = &pb3.PromotionDiscountsRecordList{}
	}
	if globalVar.PyyDatas.PromotionDiscountsRecordList[s.Id].StartTime == 0 {
		globalVar.PyyDatas.PromotionDiscountsRecordList[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.PromotionDiscountsRecordList[s.Id]
}

func (s *PromotionDiscountsSys) clearRecord() {
	if record := s.globalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.PromotionDiscountsRecordList, s.GetId())
	}
}

func (s *PromotionDiscountsSys) addRecord(record *pb3.PromotionDiscountsRecord) {
	conf := s.GetConf()
	if nil == conf {
		return
	}

	myData := s.getData()
	s.record(&myData.Records, record, int(conf.RecordNum))

	gData := s.globalRecord()
	s.record(&gData.Records, record, int(conf.RecordNum))
}

func (s *PromotionDiscountsSys) record(records *[]*pb3.PromotionDiscountsRecord, record *pb3.PromotionDiscountsRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *PromotionDiscountsSys) GetConf() *jsondata.PYYPromotionDiscountsConf {
	return jsondata.GetPYYPromotionDiscountsConf(s.ConfName, s.ConfIdx)
}

func (s *PromotionDiscountsSys) NewDay() {
	data := s.getData()
	data.Gift = nil
	s.s2cInfo()
}

const (
	PromotionDiscountsTypeFree       = 1 // 免费礼包
	PromotionDiscountsTypeConsumeBuy = 2 // 货币直购
	PromotionDiscountsTypeCharge     = 3 // 充值直购
)

func (s *PromotionDiscountsSys) judgeBuy(giftConf *jsondata.PromotionDiscountsGiftConf) (bool, error) {
	data := s.getData()

	if data.Gift[giftConf.Id] >= giftConf.Count {
		return false, neterror.ParamsInvalidError("gift conf(%d) buy limit", giftConf.Id)
	}

	if giftConf.MinSrvDay > 0 && gshare.GetOpenServerDay() < giftConf.MinSrvDay {
		return false, neterror.ParamsInvalidError("gift conf(%d) min srv day limit", giftConf.Id)
	}

	if giftConf.MinSrvDay > 0 && gshare.GetOpenServerDay() < giftConf.MaxSrvDay {
		return false, neterror.ParamsInvalidError("gift conf(%d) min srv day limit", giftConf.Id)
	}

	if giftConf.VipLimit > 0 && s.GetPlayer().GetVipLevel() < giftConf.VipLimit {
		return false, neterror.ParamsInvalidError("gift conf(%d) vip limit", giftConf.Id)
	}

	if giftConf.OpenDay != s.GetOpenDay() {
		return false, neterror.ParamsInvalidError("day not equal")
	}

	return true, nil
}

func (s *PromotionDiscountsSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_47_31
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := s.GetConf()
	if nil == conf {
		return neterror.ConfNotFoundError("PromotionDiscountsSys conf is nil")
	}

	giftId := req.GetId()

	giftConf := conf.GetGiftConfById(giftId)
	if nil == giftConf {
		return neterror.ConfNotFoundError("PromotionDiscountsSys gift conf(%d) nil", giftId)
	}

	if canBuy, err := s.judgeBuy(giftConf); !canBuy {
		return err
	}

	switch giftConf.Type {
	case PromotionDiscountsTypeFree:
	case PromotionDiscountsTypeConsumeBuy:
		if !s.GetPlayer().ConsumeByConf(giftConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogPYYPromotionDiscountsConsume}) {
			s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
	default:
		return neterror.ConfNotFoundError("PromotionDiscountsSys no define type(%d)", giftConf.Type)
	}

	data := s.getData()
	data.Gift[giftId]++

	engine.GiveRewards(s.GetPlayer(), giftConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYPromotionDiscountsAwards})

	s.addRecord(&pb3.PromotionDiscountsRecord{
		ActorId:   s.GetPlayer().GetId(),
		Name:      s.GetPlayer().GetName(),
		GiftId:    giftId,
		TimeStamp: time_util.NowSec(),
	})

	if giftConf.BroadcastId > 0 {
		engine.BroadcastTipMsgById(giftConf.BroadcastId, s.GetPlayer().GetId(), s.GetPlayer().GetName(), giftConf.Name, engine.StdRewardToBroadcast(s.GetPlayer(), giftConf.Rewards))
	}

	s.SendProto3(47, 31, &pb3.S2C_47_31{
		ActiveId: s.GetId(),
		Id:       giftId,
		Times:    data.Gift[giftId],
	})

	return nil
}

const (
	PromotionDiscountRecordGType = 1
	PromotionDiscountRecordPType = 2
)

func (s *PromotionDiscountsSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_47_32
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_47_32{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case PromotionDiscountRecordGType:
		gData := s.globalRecord()
		rsp.Records = gData.Records
	case PromotionDiscountRecordPType:
		data := s.getData()
		rsp.Records = data.Records
	}

	s.SendProto3(47, 32, rsp)
	return nil
}

func (s *PromotionDiscountsSys) chargeCheck(chargeConf *jsondata.ChargeConf) bool {
	conf := s.GetConf()
	if nil == conf {
		return false
	}

	giftConf := conf.GetGiftConfByChargeId(chargeConf.ChargeId, s.GetOpenDay())

	if giftConf == nil {
		return false
	}

	if canBuy, err := s.judgeBuy(giftConf); !canBuy {
		s.GetPlayer().LogError("err:%v", err)
		return false
	}

	return true
}

func (s *PromotionDiscountsSys) chargeBack(chargeConf *jsondata.ChargeConf) bool {
	if !s.chargeCheck(chargeConf) {
		return false
	}

	conf := s.GetConf()
	if nil == conf {
		return false
	}

	giftConf := conf.GetGiftConfByChargeId(chargeConf.ChargeId, s.GetOpenDay())

	if nil == giftConf {
		logger.LogWarn("PromotionDiscountsSys charge conf(%d) is nil", chargeConf.ChargeId)
		return false
	}

	data := s.getData()
	if data.Gift[giftConf.Id] >= giftConf.Count {
		logger.LogError("player(%d) buy gift(%d) repeated!", s.GetPlayer().GetId(), giftConf.Id)
		return false
	}

	data.Gift[giftConf.Id]++

	var rewards jsondata.StdRewardVec
	rewards = append(rewards, giftConf.Rewards...)

	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYPromotionDiscountsAwards})
	}

	s.GetPlayer().SendShowRewardsPop(rewards)

	if giftConf.BroadcastId > 0 {
		engine.BroadcastTipMsgById(giftConf.BroadcastId, s.GetPlayer().GetId(), s.GetPlayer().GetName(), giftConf.Name, engine.StdRewardToBroadcast(s.GetPlayer(), giftConf.Rewards))
	}

	s.SendProto3(47, 31, &pb3.S2C_47_31{
		ActiveId: s.GetId(),
		Id:       giftConf.Id,
		Times:    data.Gift[giftConf.Id],
	})

	s.addRecord(&pb3.PromotionDiscountsRecord{
		ActorId:   s.GetPlayer().GetId(),
		Name:      s.GetPlayer().GetName(),
		GiftId:    giftConf.Id,
		TimeStamp: time_util.NowSec(),
	})

	return true
}

func promotionDiscountsChargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYPromotionDiscounts)
	for _, obj := range yyList {
		if s, ok := obj.(*PromotionDiscountsSys); ok && s.IsOpen() {
			if s.chargeCheck(conf) {
				return true
			}
		}
	}
	return false
}

func promotionDiscountsChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYPromotionDiscounts)
	for _, obj := range yyList {
		if s, ok := obj.(*PromotionDiscountsSys); ok && s.IsOpen() {
			if s.chargeBack(conf) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYPromotionDiscounts, func() iface.IPlayerYY {
		return &PromotionDiscountsSys{}
	})

	net.RegisterYYSysProtoV2(47, 31, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*PromotionDiscountsSys).c2sBuy
	})

	net.RegisterYYSysProtoV2(47, 32, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*PromotionDiscountsSys).c2sRecord
	})

	engine.RegChargeEvent(chargedef.PromotionDiscounts, promotionDiscountsChargeCheck, promotionDiscountsChargeBack)
}
