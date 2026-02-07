/**
 * @Author: zjj
 * @Date: 2024/9/25
 * @Desc: 零元购
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
	"sort"
)

const (
	ZeroShopStateBuy    = 1 // 1 购买
	ZeroShopStateReturn = 2 // 2 领取返回奖励
)

type ZeroShopSys struct {
	Base
}

func getZeroShopLogList(subSysId uint32) []*pb3.ZeroShopLog {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return nil
	}

	if staticVar.ZeroShopLogMap == nil {
		staticVar.ZeroShopLogMap = make(map[uint32]*pb3.GlobalZeroShopLogData)
	}

	if staticVar.ZeroShopLogMap[subSysId] == nil {
		staticVar.ZeroShopLogMap[subSysId] = &pb3.GlobalZeroShopLogData{}
	}

	var newList []*pb3.ZeroShopLog
	list := staticVar.ZeroShopLogList
	if len(list) > 0 {
		for _, log := range list {
			if log.SubSysId == 0 {
				continue
			}
			if log.SubSysId == subSysId {
				staticVar.ZeroShopLogMap[subSysId].ZeroShopLogList = append(staticVar.ZeroShopLogMap[subSysId].ZeroShopLogList, log)

			}
			newList = append(newList, log)
		}
	}
	staticVar.ZeroShopLogList = newList
	return staticVar.ZeroShopLogMap[subSysId].ZeroShopLogList
}

func setZeroShopLogList(subSysId uint32, list []*pb3.ZeroShopLog) {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return
	}

	if staticVar.ZeroShopLogMap == nil {
		staticVar.ZeroShopLogMap = make(map[uint32]*pb3.GlobalZeroShopLogData)
	}

	if staticVar.ZeroShopLogMap[subSysId] == nil {
		staticVar.ZeroShopLogMap[subSysId] = &pb3.GlobalZeroShopLogData{}
	}
	staticVar.ZeroShopLogMap[subSysId].ZeroShopLogList = list
}

func appendZeroShopLog(log *pb3.ZeroShopLog) {

	var limit = uint32(20)
	commonConf := jsondata.GetZeroShopCommonConf()
	if commonConf != nil {
		limit = commonConf.LogNum
	}

	list := getZeroShopLogList(log.SubSysId)
	list = append(list, log)
	if uint32(len(list)) <= limit {
		setZeroShopLogList(log.SubSysId, list)
		return
	}

	// 从大到小排序
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt > list[j].CreatedAt
	})
	var ret = make([]*pb3.ZeroShopLog, limit)
	copy(ret, list)
	setZeroShopLogList(log.SubSysId, ret)
}

func handleC2GBroZeroShopLog(buf []byte) {
	msg := &pb3.ZeroShopLog{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}
	if msg.SrvId == engine.GetServerId() && msg.PfId == engine.GetPfId() {
		return
	}
	appendZeroShopLog(msg)
	s2cZeroShopLogs(msg.SubSysId, nil)
}

func s2cZeroShopLogs(subSysId uint32, player iface.IPlayer) {
	if player != nil {
		player.SendProto3(72, 3, &pb3.S2C_72_3{
			Logs: getZeroShopLogList(subSysId),
		})
		return
	}
	engine.Broadcast(chatdef.CIWorld, 0, 72, 3, &pb3.S2C_72_3{
		Logs: getZeroShopLogList(subSysId),
	}, 0)
}

func (s *ZeroShopSys) s2cInfo() {
	s.SendProto3(72, 0, &pb3.S2C_72_0{
		Data: s.getData(),
	})
}

func (s *ZeroShopSys) getData() *pb3.ZeroShopData {
	data := s.GetBinaryData().ZeroShopData
	if data == nil {
		s.GetBinaryData().ZeroShopData = &pb3.ZeroShopData{}
		data = s.GetBinaryData().ZeroShopData
	}
	return data
}

func (s *ZeroShopSys) checkSubSysOpen() {
	mgr := jsondata.ZeroShopSubSysIdConfMgr
	if mgr == nil {
		return
	}
	sysMgr := s.GetOwner().GetSysMgr().(*Mgr)

	var subSysId uint32
	for _, conf := range mgr {
		open := sysMgr.canOpenSys(conf.SubSysId, nil)
		if !open {
			continue
		}
		if subSysId == 0 {
			subSysId = conf.SubSysId
			continue
		}
		if mgr[subSysId].Weight < conf.Weight {
			subSysId = conf.SubSysId
			continue
		}
	}
	s.getData().SubSysId = subSysId
}

func (s *ZeroShopSys) OnReconnect() {
	s.checkSubSysOpen()
	s.s2cInfo()
}

func (s *ZeroShopSys) OnLogin() {
	s.checkSubSysOpen()
	s.s2cInfo()
}

func (s *ZeroShopSys) OnOpen() {
	s.checkSubSysOpen()
	s.s2cInfo()
}

func (s *ZeroShopSys) checkCond(cond jsondata.ZeroShopCond) bool {
	if cond.OpenDay != 0 && cond.OpenDay > gshare.GetOpenServerDay() {
		return false
	}
	owner := s.GetOwner()
	if cond.ActorLv != 0 && cond.ActorLv > owner.GetLevel() {
		return false
	}
	if cond.VipLv != 0 && cond.VipLv > owner.GetVipLevel() {
		return false
	}
	if cond.CombineTimes != 0 && cond.CombineTimes > gshare.GetMergeTimes() {
		return false
	} else {
		if cond.CombineDay != 0 && cond.CombineDay > gshare.GetMergeSrvDay() {
			return false
		}
	}
	return true
}

func (s *ZeroShopSys) crossBro(log *pb3.ZeroShopLog) {
	// 连接上跨服，需要同步到跨服服务器上
	if engine.FightClientExistPredicate(base.SmallCrossServer) {
		if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CBroZeroShopLog, log); err != nil {
			s.LogError("G2CSectBeastDamage call err: %v", err)
		}
	}
}

func (s *ZeroShopSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_72_1
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	productId := req.ProductId
	conf := jsondata.GetZeroShopConf(productId)
	if conf == nil {
		return neterror.ConfNotFoundError("%d product conf not found", productId)
	}

	data := s.getData()
	if pie.Uint32s(data.BuyProductIds).Contains(productId) {
		return neterror.ParamsInvalidError("%d already buy product", productId)
	}

	if !s.checkCond(conf.OpenCond) {
		return neterror.ParamsInvalidError("%d not reach open cond", productId)
	}

	owner := s.GetOwner()
	if len(conf.Consume) != 0 && !owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogZeroShopBuy}) {
		return neterror.ConsumeFailedError("%d consume not enough", productId)
	}

	data.BuyProductIds = append(data.BuyProductIds, productId)
	if len(conf.BuyAwards) != 0 && !engine.GiveRewards(owner, conf.BuyAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogZeroShopBuy}) {
		return neterror.ConsumeFailedError("%d give awards failed", productId)
	}

	var itemList []*pb3.KeyValue
	for _, award := range jsondata.MergeStdReward(conf.BuyAwards) {
		itemList = append(itemList, &pb3.KeyValue{
			Key:   award.Id,
			Value: uint32(award.Count),
		})
	}

	log := &pb3.ZeroShopLog{
		Name:      owner.GetName(),
		ItemList:  itemList,
		ProductId: productId,
		SrvId:     engine.GetServerId(),
		PfId:      engine.GetPfId(),
		CreatedAt: time_util.NowSec(),
		State:     ZeroShopStateBuy,
		SubSysId:  data.SubSysId,
	}
	appendZeroShopLog(log)
	s.crossBro(log)
	engine.BroadcastTipMsgById(tipmsgid.ZeroShopTip, owner.GetId(), owner.GetName(), conf.Name, engine.StdRewardToBroadcast(owner, conf.BuyAwards))
	s.SendProto3(72, 1, &pb3.S2C_72_1{
		ProductId: productId,
	})

	return nil
}

func (s *ZeroShopSys) c2sReturn(msg *base.Message) error {
	var req pb3.C2S_72_2
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	productId := req.ProductId
	conf := jsondata.GetZeroShopConf(productId)
	if conf == nil {
		return neterror.ConfNotFoundError("%d product conf not found", productId)
	}

	data := s.getData()
	if !pie.Uint32s(data.BuyProductIds).Contains(productId) {
		return neterror.ParamsInvalidError("%d not buy product", productId)
	}

	if pie.Uint32s(data.ReturnProductIds).Contains(productId) {
		return neterror.ParamsInvalidError("%d already return product", productId)
	}

	if !s.checkCond(conf.ReturnCond) {
		return neterror.ParamsInvalidError("%d not reach return cond", productId)
	}

	owner := s.GetOwner()
	data.ReturnProductIds = append(data.ReturnProductIds, productId)
	if len(conf.ReturnAwards) != 0 && !engine.GiveRewards(owner, conf.ReturnAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogZeroShopReturn}) {
		return neterror.ConsumeFailedError("%d return awards failed", productId)
	}

	var itemList []*pb3.KeyValue
	for _, award := range jsondata.MergeStdReward(conf.ReturnAwards) {
		itemList = append(itemList, &pb3.KeyValue{
			Key:   award.Id,
			Value: uint32(award.Count),
		})
	}

	log := &pb3.ZeroShopLog{
		Name:      owner.GetName(),
		ItemList:  itemList,
		ProductId: productId,
		SrvId:     engine.GetServerId(),
		PfId:      engine.GetPfId(),
		CreatedAt: time_util.NowSec(),
		State:     ZeroShopStateReturn,
		SubSysId:  data.SubSysId,
	}
	appendZeroShopLog(log)
	s.crossBro(log)

	s.SendProto3(72, 2, &pb3.S2C_72_2{
		ProductId: productId,
	})

	return nil
}

func (s *ZeroShopSys) c2sLog(_ *base.Message) error {
	s2cZeroShopLogs(s.getData().SubSysId, s.GetOwner())
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiZeroShop, func() iface.ISystem {
		return &ZeroShopSys{}
	})
	net.RegisterSysProtoV2(72, 1, sysdef.SiZeroShop, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ZeroShopSys).c2sBuy
	})
	net.RegisterSysProtoV2(72, 2, sysdef.SiZeroShop, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ZeroShopSys).c2sReturn
	})
	net.RegisterSysProtoV2(72, 3, sysdef.SiZeroShop, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ZeroShopSys).c2sLog
	})
	engine.RegisterSysCall(sysfuncid.C2GBroZeroShopLog, handleC2GBroZeroShopLog)
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		s := player.GetSysObj(sysdef.SiZeroShop)
		if s == nil || !s.IsOpen() {
			return
		}
		sys := s.(*ZeroShopSys)
		sys.checkSubSysOpen()
		sys.s2cInfo()
	})
}
