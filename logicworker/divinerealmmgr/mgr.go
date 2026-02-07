/**
 * @Author: lzp
 * @Date: 2025/7/25
 * @Desc:
**/

package divinerealmmgr

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
)

type DivineRealmDividendMgr struct {
	roundExploitRanks map[uint32]*base.Rank
}

var drDividendMgr = new(DivineRealmDividendMgr)

func GetMgr() *DivineRealmDividendMgr {
	return drDividendMgr
}

func (mgr *DivineRealmDividendMgr) mgrInit() {
	mgr.loadRank()
}

func (mgr *DivineRealmDividendMgr) onNewDay() {
	data := mgr.getData()
	data.DivineRealmBoxData = nil
	data.ExploitRanks = nil

	mgr.s2cBoxData()
}

func (mgr *DivineRealmDividendMgr) getData() *pb3.DivineRealmDividend {
	globalVar := gshare.GetStaticVar()
	if globalVar.DivineRealmDividend == nil {
		globalVar.DivineRealmDividend = &pb3.DivineRealmDividend{}
	}

	dividend := globalVar.DivineRealmDividend

	if dividend.ExploitRanks == nil {
		dividend.ExploitRanks = make(map[uint32]*pb3.DivineRealmRoundExploit)
	}
	if dividend.DivineRealmBoxData == nil {
		dividend.DivineRealmBoxData = make(map[uint32]*pb3.DivineRealmBox)
	}

	return dividend
}

func (mgr *DivineRealmDividendMgr) loadRank() {
	data := mgr.getData()
	for round, v := range data.ExploitRanks {
		r := mgr.getRoundExploitRank(round)
		for _, one := range v.Rank {
			r.Update(one.Id, one.Score)
		}
	}
}

func (mgr *DivineRealmDividendMgr) getRoundExploitRank(round uint32) *base.Rank {
	if mgr.roundExploitRanks == nil {
		mgr.roundExploitRanks = map[uint32]*base.Rank{}
	}

	if _, ok := mgr.roundExploitRanks[round]; !ok {
		mgr.roundExploitRanks[round] = base.NewRank(200)
	}

	return mgr.roundExploitRanks[round]
}

func (mgr *DivineRealmDividendMgr) s2cBoxData() {
	data := mgr.getData()
	engine.Broadcast(chatdef.CIWorld, 0, 77, 20, &pb3.S2C_77_20{
		Data: data.DivineRealmBoxData,
	}, 0)
}

func (mgr *DivineRealmDividendMgr) GiveDividends(boxData *pb3.DivineRealmBox) {
	conf := jsondata.GetDivineRealmConquerConf()
	if conf == nil {
		return
	}

	actorDividends := mgr.calcActorDividends(boxData)
	if len(actorDividends) == 0 {
		return
	}

	for actorId, dividend := range actorDividends {
		if dividend > 0 {
			mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
				ConfId: common.Mail_DivineRealmBoxDividends,
				Rewards: jsondata.StdRewardVec{
					{Id: conf.BosDividendsItem, Count: int64(dividend)},
				},
			})
		}
	}
}

func (mgr *DivineRealmDividendMgr) PackBoxDividends(boxId uint32) []*pb3.DivineRealmShowDividend {
	data := mgr.getData()
	boxData, ok := data.DivineRealmBoxData[boxId]
	if !ok {
		return nil
	}

	actorDividends := mgr.calcActorDividends(boxData)
	rank := mgr.getRoundExploitRank(boxData.Round)
	var dividendPackL []*pb3.DivineRealmShowDividend
	for _, one := range rank.GetList(1, len(actorDividends)) {
		playerData, ok := manager.GetData(one.Id, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			logger.LogError("PackBoxDividends, playerId: %d error", one.Id)
			continue
		}
		dividend, ok := actorDividends[one.Id]
		if !ok {
			continue
		}
		if dividend > 0 {
			dividendPackL = append(dividendPackL, &pb3.DivineRealmShowDividend{
				ActorId:  playerData.GetId(),
				Name:     playerData.GetName(),
				Score:    one.Score,
				Dividend: dividend,
			})
		}
	}
	return dividendPackL
}

func (mgr *DivineRealmDividendMgr) GetBoxData() map[uint32]*pb3.DivineRealmBox {
	data := mgr.getData()
	return data.DivineRealmBoxData
}

func (mgr *DivineRealmDividendMgr) calcActorDividends(boxData *pb3.DivineRealmBox) map[uint64]uint32 {
	conf := jsondata.GetDivineRealmConquerConf()
	if conf == nil {
		return nil
	}

	baseRatio := conf.BoxDividendsBase
	limit := conf.BoxDividendsLimit

	if baseRatio <= 0 || limit <= 0 {
		return nil
	}

	round := boxData.Round
	rank := mgr.getRoundExploitRank(round)
	if rank.Empty() {
		return nil
	}

	sumPrice := boxData.Price
	actorDividends := make(map[uint64]uint32)

	// 基础分红
	baseDividend := utils.CalcMillionRate(sumPrice, baseRatio)
	for _, one := range rank.GetList(1, int(limit)) {
		actorDividends[one.GetId()] += baseDividend
	}

	// 额外分红
	restDividend := sumPrice - baseDividend*uint32(len(actorDividends))
	if restDividend > 0 {
		var sum uint32
		for _, v := range conf.BoxDividendsExtra {
			actorId := rank.GetIdByRank(v.Rank)
			if actorId == 0 {
				continue
			}
			sum += v.Weight
		}

		for _, v := range conf.BoxDividendsExtra {
			actorId := rank.GetIdByRank(v.Rank)
			if actorId > 0 {
				extraDividend := restDividend * v.Weight / sum
				maxExtraDividend := utils.CalcMillionRate(restDividend, v.MaxRatio)

				extraDividend = utils.MinUInt32(maxExtraDividend, extraDividend)
				actorDividends[actorId] += extraDividend
			}
		}
	}
	return actorDividends
}

func (mgr *DivineRealmDividendMgr) refreshBox(sType, round uint32) {
	conf := jsondata.GetDivineRealmBoxConf(sType)
	if conf == nil {
		return
	}

	data := GetMgr().getData()

	idx := len(data.DivineRealmBoxData) + 1
	length := len(data.DivineRealmBoxData) + int(conf.BoxNum)
	for i := idx; i <= length; i++ {
		pool := new(random.Pool)
		for i := range conf.BoxPrice {
			pool.AddItem(conf.BoxPrice[i], conf.BoxPrice[i].DiscountWeight)
		}

		ret := pool.RandomOne().(*jsondata.DivineRealmBoxPrice)

		data.DivineRealmBoxData[uint32(i)] = &pb3.DivineRealmBox{
			Id:        uint32(i),
			BoxType:   sType,
			Round:     round,
			MoneyType: ret.MoneyType,
			Price:     ret.Price,
			DisCount:  ret.PriceDiscount,
		}
	}

	mgr.s2cBoxData()
}

func handleC2SDivineRealmBoxDisplay(buf []byte) {
	var req pb3.CommonSt
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		return
	}

	sType := req.U32Param
	round := req.U32Param2

	mgr := GetMgr()
	mgr.refreshBox(sType, round)
}

func handleRoundCampExploitRank(buf []byte) {
	var req pb3.C2GDivineRealmRoundCampExploitRank
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		return
	}

	mgr := GetMgr()
	data := mgr.getData()
	round := req.Round

	data.ExploitRanks[round] = &pb3.DivineRealmRoundExploit{
		Round: round,
		Rank:  req.Rank,
	}

	rank := mgr.getRoundExploitRank(round)
	rank.Clear()

	for _, one := range req.Rank {
		rank.Update(one.Id, one.Score)
	}
}

func init() {
	engine.RegisterSysCall(sysfuncid.C2GDivineRealmBoxDisplay, handleC2SDivineRealmBoxDisplay)
	engine.RegisterSysCall(sysfuncid.C2GDivineRealmRoundExploitRank, handleRoundCampExploitRank)

	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		GetMgr().onNewDay()
	})
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		GetMgr().mgrInit()
	})

	gmevent.Register("refreshDividendBoxes", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		sType := utils.AtoUint32(args[0])
		round := utils.AtoUint32(args[1])
		mgr := GetMgr()
		data := mgr.getData()
		data.DivineRealmBoxData = nil

		mgr.refreshBox(sType, round)
		return true
	}, 1)
	gmevent.Register("clearDividendBoxes", func(player iface.IPlayer, args ...string) bool {
		mgr := GetMgr()
		data := mgr.getData()
		data.DivineRealmBoxData = nil
		engine.Broadcast(chatdef.CIWorld, 0, 77, 20, &pb3.S2C_77_20{
			Data: data.DivineRealmBoxData,
		}, 0)
		return true
	}, 1)
}
