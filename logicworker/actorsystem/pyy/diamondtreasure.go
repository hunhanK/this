/**
 * @Author: lzp
 * @Date: 2024/7/29
 * @Desc: 寻玉宝库
**/

package pyy

import (
	"encoding/json"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type DiamondTreasureSys struct {
	*PlayerYYBase
}

func (s *DiamondTreasureSys) OnReconnect() {
	s.S2CInfo()
}

func (s *DiamondTreasureSys) Login() {
	s.S2CInfo()
}

func (s *DiamondTreasureSys) OnOpen() {
	s.openActCheckDailyCharge()
	s.S2CInfo()
}

func (s *DiamondTreasureSys) OnEnd() {
	globalVar := gshare.GetStaticVar()
	globalVar.DiamondTreasureRecords = nil
}

func (s *DiamondTreasureSys) NewDay() {
	data := s.GetData()
	data.IsRevDailyRewards = false
	s.S2CInfo()
}

func (s *DiamondTreasureSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.DiamondTreasure == nil {
		return
	}
	delete(state.DiamondTreasure, s.Id)
}

func (s *DiamondTreasureSys) openActCheckDailyCharge() {
	s.OnAddChargeMoney(s.GetDailyCharge())
}

func (s *DiamondTreasureSys) GetData() *pb3.PYY_DiamondTreasure {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.DiamondTreasure == nil {
		state.DiamondTreasure = make(map[uint32]*pb3.PYY_DiamondTreasure)
	}
	if state.DiamondTreasure[s.Id] == nil {
		state.DiamondTreasure[s.Id] = &pb3.PYY_DiamondTreasure{}
	}
	data := state.DiamondTreasure[s.Id]
	if data.UseRecords == nil {
		data.UseRecords = make([]*pb3.ItemGetRecord, 0)
	}
	return data
}

func (s *DiamondTreasureSys) S2CInfo() {
	data := s.GetData()
	s.SendProto3(127, 85, &pb3.S2C_127_85{
		ActId: s.GetId(),
		Data: &pb3.DiamondTreasurePlayerInfo{
			ChargeMoney:       data.ChargeMoney,
			PlayerRds:         data.UseRecords,
			ServerRds:         s.GetServerRd(),
			RemainTimes:       utils.MaxUInt32(0, s.GetLotteryTimes()-data.UseTimes),
			RewardsBit:        data.RewardsBit,
			IsRevDailyRewards: data.IsRevDailyRewards,
		},
	})
}

func (s *DiamondTreasureSys) GetLotteryTimes() uint32 {
	conf := jsondata.GetYYDiamondTreasureConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	times := uint32(0)
	data := s.GetData()
	for _, chargeConf := range conf.ChargeConf {
		if data.ChargeMoney >= chargeConf.ChargeMoney && times < chargeConf.LotteryTimes {
			times = chargeConf.LotteryTimes
		}
	}
	return times
}

func (s *DiamondTreasureSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	cashCent := chargeEvent.CashCent
	chargeId := chargeEvent.ChargeId
	chargeConf := jsondata.GetChargeConf(chargeId)
	if chargeConf == nil {
		return
	}

	s.OnAddChargeMoney(cashCent)
}

func (s *DiamondTreasureSys) OnAddChargeMoney(cashCent uint32) {
	data := s.GetData()
	data.ChargeMoney += cashCent
	s.S2CInfo()
}

func (s *DiamondTreasureSys) GetServerRd() []*pb3.ItemGetRecord {
	globalVar := gshare.GetStaticVar()
	if globalVar.DiamondTreasureRecords == nil {
		globalVar.DiamondTreasureRecords = make([]*pb3.ItemGetRecord, 0)
	}
	return globalVar.DiamondTreasureRecords
}

func (s *DiamondTreasureSys) AddServerRd(rd *pb3.ItemGetRecord) {
	globalVar := gshare.GetStaticVar()
	if globalVar.DiamondTreasureRecords == nil {
		globalVar.DiamondTreasureRecords = make([]*pb3.ItemGetRecord, 0)
	}
	globalVar.DiamondTreasureRecords = append(globalVar.DiamondTreasureRecords, rd)
	if len(globalVar.DiamondTreasureRecords) > 50 {
		globalVar.DiamondTreasureRecords = globalVar.DiamondTreasureRecords[1:]
	}
}

func (s *DiamondTreasureSys) c2sLotteryRewards(msg *base.Message) error {
	var req pb3.C2S_127_86
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf := jsondata.GetYYDiamondTreasureConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("%s config not found", s.GetPrefix())
	}

	data := s.GetData()
	if data.UseTimes >= s.GetLotteryTimes() {
		return neterror.ParamsInvalidError("%s times limit", s.GetPrefix())
	}

	var retIds []uint32
	for id := range conf.LotteryRewardsConf {
		if utils.IsSetBit(data.RewardsBit, id) {
			continue
		}
		retIds = append(retIds, id)
	}

	if len(retIds) == 0 {
		return neterror.ParamsInvalidError("%s times empty", s.GetPrefix())
	}

	pool := &random.Pool{}
	for _, id := range retIds {
		if subConf, ok := conf.LotteryRewardsConf[id]; ok {
			pool.AddItem(subConf, subConf.Weight)
		}
	}

	rConf := pool.RandomOne().(*jsondata.YYDiamondTreasureRewardsConf)

	// 设置标志
	data.RewardsBit = utils.SetBit(data.RewardsBit, rConf.Id)
	data.UseTimes += 1

	rewards := jsondata.StdRewardVec{{Id: rConf.ItemId, Count: int64(rConf.ItemCount)}}
	engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogYYDiamondTreasureReward})

	// 记录
	rd := &pb3.ItemGetRecord{
		ActorId:   s.player.GetId(),
		ActorName: s.player.GetName(),
		ItemId:    rConf.ItemId,
		Count:     rConf.ItemCount,
		TimeStamp: time_util.NowSec(),
	}
	data.UseRecords = append(data.UseRecords, rd)
	s.AddServerRd(rd)

	// 日志
	logArg, _ := json.Marshal(map[string]interface{}{
		"count":   data.UseTimes,
		"idx":     rConf.Id,
		"diamond": rConf.ItemCount,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogYYDiamondTreasureReward, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: string(logArg),
	})

	s.SendProto3(127, 86, &pb3.S2C_127_86{
		ActId:     s.Id,
		ItemId:    rConf.ItemId,
		ItemCount: rConf.ItemCount,
	})
	s.S2CInfo()

	return nil
}

func (s *DiamondTreasureSys) c2sGetDailyRewards(_ *base.Message) error {
	conf := jsondata.GetYYDiamondTreasureConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("%s config not found", s.GetPrefix())
	}

	data := s.GetData()
	if data.IsRevDailyRewards {
		return neterror.ConfNotFoundError("dailyRewards is received")
	}

	data.IsRevDailyRewards = true
	if len(conf.DayRewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.DayRewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogYYDiamondTreasureDayRewards,
		})
	}

	s.SendProto3(127, 87, &pb3.S2C_127_87{
		ActId:             s.Id,
		IsRevDailyRewards: data.IsRevDailyRewards,
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYDiamondTreasure, func() iface.IPlayerYY {
		return &DiamondTreasureSys{
			PlayerYYBase: &PlayerYYBase{},
		}
	})

	net.RegisterYYSysProtoV2(127, 86, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*DiamondTreasureSys).c2sLotteryRewards
	})
	net.RegisterYYSysProtoV2(127, 87, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*DiamondTreasureSys).c2sGetDailyRewards
	})
}
