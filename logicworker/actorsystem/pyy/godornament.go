/**
 * @Author: lzp
 * @Date: 2025/2/6
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
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
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type GodOrnamentSys struct {
	PlayerYYBase
}

func (s *GodOrnamentSys) OnReconnect() {
	s.s2cInfo()
}

func (s *GodOrnamentSys) Login() {
	s.s2cInfo()
}

func (s *GodOrnamentSys) OnOpen() {
	s.s2cInfo()
}

func (s *GodOrnamentSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.GodOrnament == nil {
		return
	}
	delete(state.GodOrnament, s.Id)
}

func (s *GodOrnamentSys) s2cInfo() {
	s.SendProto3(127, 140, &pb3.S2C_127_140{
		ActId: s.GetId(),
		Data:  s.getData(),
	})
}

func (s *GodOrnamentSys) getData() *pb3.PYY_GodOrnament {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.GodOrnament == nil {
		state.GodOrnament = make(map[uint32]*pb3.PYY_GodOrnament)
	}
	if state.GodOrnament[s.Id] == nil {
		state.GodOrnament[s.Id] = &pb3.PYY_GodOrnament{}
	}
	return state.GodOrnament[s.Id]
}

func (s *GodOrnamentSys) getRecords() *[]*pb3.ItemsGetRecord {
	globalVar := gshare.GetStaticVar()
	if globalVar.GodOrnamentRecords == nil {
		globalVar.GodOrnamentRecords = make([]*pb3.ItemsGetRecord, 0)
	}
	return &globalVar.GodOrnamentRecords
}

func (s *GodOrnamentSys) addRecord(rd *pb3.ItemsGetRecord) {
	conf := jsondata.GetPYYGodOrnamentConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	limit := int(conf.RecordLimit)

	// 全服记录
	records := s.getRecords()
	*records = append(*records, rd)
	if len(*records) > limit {
		*records = (*records)[len(*records)-limit:]
	}
}

func (s *GodOrnamentSys) c2sFetchDayRewards(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity")
	}
	var req pb3.C2S_127_141
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.GetPYYGodOrnamentRewards(s.ConfName, s.ConfIdx, req.Day)
	if conf == nil {
		return neterror.ParamsInvalidError("config not found")
	}

	data := s.getData()
	if !data.IsBuy {
		return neterror.ParamsInvalidError("cannot buy")
	}

	day := s.GetOpenDay()
	if req.Day > day {
		return neterror.ParamsInvalidError("open day limit")
	}

	if utils.SliceContainsUint32(data.IdL, req.Day) {
		return neterror.ParamsInvalidError("day: %d rewards has fetched", req.Day)
	}

	data.IdL = append(data.IdL, req.Day)

	if len(conf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYGodOrnamentDayAward})
	}

	player := s.GetPlayer()
	engine.BroadcastTipMsgById(tipmsgid.GodornamentTip, player.GetId(), player.GetName(), req.Day, engine.StdRewardToBroadcast(player, conf.Rewards))
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYGodOrnamentDayAward, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", req.Day),
	})

	items := make(map[uint32]uint32)
	for _, reward := range conf.Rewards {
		items[reward.Id] = uint32(reward.Count)
	}
	s.addRecord(&pb3.ItemsGetRecord{
		ActorId:   player.GetId(),
		ActorName: player.GetName(),
		Items:     items,
		TimeStamp: time_util.NowSec(),
		Ext:       req.Day,
	})

	s.SendProto3(127, 141, &pb3.S2C_127_141{
		ActId: s.GetId(),
		Day:   req.Day,
	})
	return nil
}

func (s *GodOrnamentSys) c2sGetRecords(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity")
	}
	var req pb3.C2S_127_142
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	records := s.getRecords()
	s.SendProto3(127, 142, &pb3.S2C_127_142{
		ActId:   s.GetId(),
		Records: *records,
	})
	return nil
}

func (s *GodOrnamentSys) chargeCheck(chargeId uint32) bool {
	data := s.getData()
	if s.GetPlayer().GetVipLevel() == 0 {
		return false
	}
	if data.IsBuy {
		return false
	}

	conf := jsondata.GetPYYGodOrnamentConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return false
	}

	if conf.GiftId != chargeId {
		return false
	}

	return true
}

func (s *GodOrnamentSys) chargeBack() bool {
	data := s.getData()
	data.IsBuy = true
	s.s2cInfo()
	return true
}

func GodOrnamentCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYGodOrnament)
	for _, obj := range yyObjs {
		if s, ok := obj.(*GodOrnamentSys); ok && s.IsOpen() {
			if s.chargeCheck(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func GodOrnamentChargeBack(player iface.IPlayer, _ *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYGodOrnament)
	for _, obj := range yyObjs {
		if s, ok := obj.(*GodOrnamentSys); ok && s.IsOpen() {
			if s.chargeBack() {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYGodOrnament, func() iface.IPlayerYY {
		return &GodOrnamentSys{}
	})

	engine.RegChargeEvent(chargedef.GodOrnament, GodOrnamentCheck, GodOrnamentChargeBack)
	net.RegisterYYSysProtoV2(127, 141, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GodOrnamentSys).c2sFetchDayRewards
	})
	net.RegisterYYSysProtoV2(127, 142, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GodOrnamentSys).c2sGetRecords
	})
}
