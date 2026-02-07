/**
 * @Author: LvYuMeng
 * @Date: 2024/8/1
 * @Desc: 仙宗拍卖
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type SectAuctionSys struct {
	Base
}

func (s *SectAuctionSys) OnAfterLogin() {
	s.s2cInfo()
	s.reqCrossInfo()
}

func (s *SectAuctionSys) OnReconnect() {
	s.s2cInfo()
	s.reqCrossInfo()
}

func (s *SectAuctionSys) s2cInfo() {
	s.SendProto3(70, 24, &pb3.S2C_70_24{Data: s.data()})
}

func (s *SectAuctionSys) reqCrossInfo() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CReqSectAuctionInfo, &pb3.CommonSt{
		U64Param:  s.owner.GetId(),
		U32Param:  engine.GetPfId(),
		U32Param2: engine.GetServerId(),
	})
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
}

func (s *SectAuctionSys) data() *pb3.AuctionData {
	binary := s.GetBinaryData()
	if nil == binary.SectAuctionData {
		binary.SectAuctionData = &pb3.AuctionData{}
	}
	return binary.SectAuctionData
}

const sectAuctionRecord = 50

func (s *SectAuctionSys) c2sRecord(_ *base.Message) error {
	s.SendProto3(70, 23, &pb3.S2C_70_23{Records: s.data().Records})
	return nil
}

func (s *SectAuctionSys) addRecord(record *pb3.AuctionRecord) {
	data := s.data()
	data.Records = append(data.Records, record)
	if len(data.Records) > sectAuctionRecord {
		data.Records = data.Records[1:]
	}
}

func (s *SectAuctionSys) retBid(req *pb3.C2GSectAuctionBidReq) {
	consumeMoney := jsondata.ConsumeVec{
		{Type: custom_id.ConsumeTypeMoney,
			Id:    req.GetMoneyType(),
			Count: req.GetMoneyCount(),
		},
	}
	success, remove := s.owner.ConsumeByConfWithRet(consumeMoney, false, common.ConsumeParams{
		LogId: pb3.LogId_LogSectAuctionConsume,
	})
	if !success {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return
	}
	ret := &pb3.G2CSectAuctionBidRet{
		ActorId:     s.owner.GetId(),
		PfId:        engine.GetPfId(),
		SrvId:       engine.GetServerId(),
		GoodsId:     req.GetGoodsId(),
		BuyWay:      req.GetBuyWay(),
		CurrentBind: req.GetCurrentBind(),
	}
	for k, v := range remove.MoneyMap {
		if v == 0 {
			continue
		}
		ret.BidMoney = append(ret.BidMoney, &pb3.KeyVal64{
			Key:   k,
			Value: uint64(v),
		})
	}
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSectAuctionBid, &pb3.LogPlayerCounter{
		NumArgs: req.GetGoodsId(),
	})
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSectAuctionBidRet, ret)
	if err != nil {
		s.LogError("player bid err:%v, req:%v", err, req)
		return
	}
}

func onSectAuctionBidSuccess(buf []byte) {
	msg := &pb3.C2GSectAuctionBidSuccess{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}

	rewards := jsondata.StdRewardVec{
		{
			Id:    msg.GetItemId(),
			Count: int64(msg.GetCount()),
		},
	}
	mailmgr.SendMailToActor(msg.GetActorId(), &mailargs.SendMailSt{
		ConfId:  common.MailSectAuctionBidSuccess,
		Rewards: rewards,
	})
	engine.SendPlayerMessage(msg.GetActorId(), gshare.OfflineSectAuctionRecord, msg)
}

func onSectAuctionBidReq(buf []byte) {
	msg := &pb3.C2GSectAuctionBidReq{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}
	player := manager.GetPlayerPtrById(msg.GetActorId())
	if nil == player {
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiSectAuction).(*SectAuctionSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.retBid(msg)
}

func offlineSectAuctionRecord(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.C2GSectAuctionBidSuccess)
	if !ok {
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiSectAuction).(*SectAuctionSys)
	if !ok {
		return
	}
	sys.addRecord(&pb3.AuctionRecord{
		ItemId:    st.GetItemId(),
		Count:     st.GetCount(),
		MoneyType: st.GetMoneyType(),
		Price:     st.GetPrice(),
		BuyWay:    st.GetBuyWay(),
		TimeStamp: st.GetTimeStamp(),
	})
}

func init() {
	RegisterSysClass(sysdef.SiSectAuction, func() iface.ISystem {
		return &SectAuctionSys{}
	})
	engine.RegisterSysCall(sysfuncid.C2GSectAuctionBidSuccess, onSectAuctionBidSuccess)
	engine.RegisterSysCall(sysfuncid.C2GSectAuctionBidReq, onSectAuctionBidReq)

	engine.RegisterMessage(gshare.OfflineSectAuctionRecord, func() pb3.Message {
		return &pb3.C2GSectAuctionBidSuccess{}
	}, offlineSectAuctionRecord)

	net.RegisterSysProtoV2(70, 23, sysdef.SiSectAuction, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectAuctionSys).c2sRecord
	})

	gmevent.Register("addSectAuction", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		itemId := utils.AtoUint32(args[0])
		itemCount := utils.AtoInt64(args[1])
		err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CGmAddSectAuctionItem, &pb3.CommonSt{
			U64Param:  player.GetId(),
			U32Param:  engine.GetPfId(),
			U32Param2: engine.GetServerId(),
			U32Param3: itemId,
			I64Param:  itemCount,
		})
		return err == nil
	}, 1)
}
