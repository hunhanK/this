/**
 * @Author: lzp
 * @Date: 2024/8/2
 * @Desc:
**/

package pyy

import (
	"encoding/json"
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
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
	"time"
)

type PlayerLuckyTurntable struct {
	PlayerYYBase
}

func (s *PlayerLuckyTurntable) OnReconnect() {
	s.S2CInfo()
}

func (s *PlayerLuckyTurntable) Login() {
	s.S2CInfo()
}

func (s *PlayerLuckyTurntable) OnOpen() {
	s.S2CInfo()
}

func (s *PlayerLuckyTurntable) NewDay() {
	data := s.GetData()
	data.IsRevDailyRewards = false
	s.S2CInfo()
}

func (s *PlayerLuckyTurntable) S2CInfo() {
	data := s.GetData()
	s.SendProto3(127, 95, &pb3.S2C_127_95{
		ActId: s.Id,
		Data: &pb3.LuckyTurntablePlayerInfo{
			ChargeDiamond:     data.ChargeDiamond,
			LeftTimes:         s.GetLotteryTimes() - data.UseTimes,
			IsRevDailyRewards: data.IsRevDailyRewards,
			PlayerRds:         data.UseRecords,
			ServerRds:         s.getServerRd(),
		},
	})
}

func (s *PlayerLuckyTurntable) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.LuckyTurntable == nil {
		return
	}
	delete(state.LuckyTurntable, s.Id)
}

func (s *PlayerLuckyTurntable) OnEnd() {
	s.cleanServerRd()
}

func (s *PlayerLuckyTurntable) GetData() *pb3.PYY_LuckyTurntable {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.LuckyTurntable == nil {
		state.LuckyTurntable = make(map[uint32]*pb3.PYY_LuckyTurntable)
	}
	if state.LuckyTurntable[s.Id] == nil {
		state.LuckyTurntable[s.Id] = &pb3.PYY_LuckyTurntable{}
	}
	data := state.LuckyTurntable[s.Id]
	if data.UseRecords == nil {
		data.UseRecords = make([]*pb3.ItemGetRecord, 0)
	}
	return data
}

func (s *PlayerLuckyTurntable) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	diamond := chargeEvent.Diamond
	chargeId := chargeEvent.ChargeId
	chargeConf := jsondata.GetChargeConf(chargeId)
	if chargeConf != nil && chargeConf.ChargeType == chargedef.Charge {
		s.OnAddChargeDiamond(diamond)
	}
}

func (s *PlayerLuckyTurntable) OnAddChargeDiamond(diamond uint32) {
	data := s.GetData()
	data.ChargeDiamond += diamond
	s.S2CInfo()
}

func (s *PlayerLuckyTurntable) GetLotteryTimes() uint32 {
	conf := jsondata.GetYYLuckyTurntableConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	times := uint32(0)
	data := s.GetData()
	for _, lConf := range conf.Lottery {
		if data.ChargeDiamond >= lConf.ChargeDiamond && lConf.Id > times {
			times = lConf.Id
		}
	}
	return times
}

func (s *PlayerLuckyTurntable) c2sLottery(msg *base.Message) error {
	var req pb3.C2S_127_96
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf := jsondata.GetYYLuckyTurntableConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("%s config not found", s.GetPrefix())
	}

	data := s.GetData()
	if data.UseTimes >= s.GetLotteryTimes() {
		return neterror.ParamsInvalidError("%s times limit", s.GetPrefix())
	}

	lConf := jsondata.GetYYLuckyTurntableLotteryConf(s.ConfName, s.ConfIdx, data.UseTimes+1)
	if lConf == nil {
		return neterror.ParamsInvalidError("%s %d times conf not found", s.GetPrefix(), data.UseTimes)
	}

	cId, count := s.doLottery(lConf.CWeight)
	rId, ratio := s.doLottery(lConf.RWeight)

	data.UseTimes += 1
	itemId := conf.ItemId
	itemCount := float64(count) * float64(ratio) / float64(10000)
	rewards := jsondata.StdRewardVec{{Id: itemId, Count: int64(itemCount)}}

	// 奖励延迟几秒,前端做表现
	if !req.IsSkip {
		s.GetPlayer().SetTimeout(time.Second*time.Duration(int64(conf.Dur)), func() {
			engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
				LogId: pb3.LogId_LogYYLuckyTurntableLotteryRewards,
			})
		})
	} else {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogYYLuckyTurntableLotteryRewards,
		})
	}

	// 记录
	rd := &pb3.ItemGetRecord{
		ActorId:   s.player.GetId(),
		ActorName: s.player.GetName(),
		ItemId:    itemId,
		Count:     count,
		TimeStamp: time_util.NowSec(),
		Ext:       ratio,
	}
	data.UseRecords = append(data.UseRecords, rd)
	s.addServerRd(rd)

	// 日志
	logArg, _ := json.Marshal(map[string]interface{}{
		"useTimes":  data.UseTimes,
		"itemId":    itemId,
		"itemCount": itemCount,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogYYLuckyTurntableLotteryRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: string(logArg),
	})

	s.SendProto3(127, 96, &pb3.S2C_127_96{
		ActId:   s.Id,
		CountId: cId,
		RatioId: rId,
	})
	s.S2CInfo()
	return nil
}

func (s *PlayerLuckyTurntable) c2sGetDailyRewards(_ *base.Message) error {
	conf := jsondata.GetYYLuckyTurntableConf(s.ConfName, s.ConfIdx)
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
			LogId: pb3.LogId_LogYYLuckyTurntableDailyRewards,
		})
	}

	s.S2CInfo()
	return nil
}

func (s *PlayerLuckyTurntable) doLottery(wConfL []*jsondata.LuckyTurntableWeight) (uint32, uint32) {
	pool := &random.Pool{}
	for _, v := range wConfL {
		pool.AddItem(v, v.Weight)
	}
	conf := pool.RandomOne().(*jsondata.LuckyTurntableWeight)
	return conf.Id, conf.Value
}

func (s *PlayerLuckyTurntable) addServerRd(rd *pb3.ItemGetRecord) {
	globalVar := gshare.GetStaticVar()
	if globalVar.LuckyTurntableRecords == nil {
		globalVar.LuckyTurntableRecords = make([]*pb3.ItemGetRecord, 0)
	}
	globalVar.LuckyTurntableRecords = append(globalVar.LuckyTurntableRecords, rd)
	if len(globalVar.LuckyTurntableRecords) > 50 {
		globalVar.LuckyTurntableRecords = globalVar.LuckyTurntableRecords[1:]
	}
}

func (s *PlayerLuckyTurntable) cleanServerRd() {
	globalVar := gshare.GetStaticVar()
	if globalVar.LuckyTurntableRecords == nil {
		return
	}
	globalVar.LuckyTurntableRecords = make([]*pb3.ItemGetRecord, 0)
}

func (s *PlayerLuckyTurntable) getServerRd() []*pb3.ItemGetRecord {
	globalVar := gshare.GetStaticVar()
	if globalVar.LuckyTurntableRecords == nil {
		globalVar.LuckyTurntableRecords = make([]*pb3.ItemGetRecord, 0)
	}
	return globalVar.LuckyTurntableRecords
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYLuckyTurntable, func() iface.IPlayerYY {
		return &PlayerLuckyTurntable{}
	})

	net.RegisterYYSysProtoV2(127, 96, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerLuckyTurntable).c2sLottery
	})

	net.RegisterYYSysProtoV2(127, 97, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerLuckyTurntable).c2sGetDailyRewards
	})
}
