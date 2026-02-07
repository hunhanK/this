/**
 * @Author: zjj
 * @Date: 2024/8/1
 * @Desc: 屠龙BOSS
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type CumulativeChargeBossSys struct {
	PlayerYYBase
}

func (s *CumulativeChargeBossSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.CumulativeChargeBossMap {
		return
	}
	delete(state.CumulativeChargeBossMap, s.Id)
}

func (s *CumulativeChargeBossSys) GetData() *pb3.PYYCumulativeChargeBossData {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.CumulativeChargeBossMap {
		state.CumulativeChargeBossMap = make(map[uint32]*pb3.PYYCumulativeChargeBossData)
	}
	if state.CumulativeChargeBossMap[s.Id] == nil {
		state.CumulativeChargeBossMap[s.Id] = &pb3.PYYCumulativeChargeBossData{}
	}
	return state.CumulativeChargeBossMap[s.Id]
}
func (s *CumulativeChargeBossSys) s2cInfo() {
	s.SendProto3(135, 10, &pb3.S2C_135_10{
		ActiveId: s.Id,
		Info:     s.GetData(),
	})
}

func (s *CumulativeChargeBossSys) reset() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.CumulativeChargeBossMap {
		state.CumulativeChargeBossMap = make(map[uint32]*pb3.PYYCumulativeChargeBossData)
	}
	state.CumulativeChargeBossMap[s.Id] = &pb3.PYYCumulativeChargeBossData{}
}

func (s *CumulativeChargeBossSys) OnOpen() {
	s.reset()
	s.s2cInfo()
}

func (s *CumulativeChargeBossSys) MergeFix() {
	if s.GetOpenDay() != 1 {
		return
	}

	openZeroTime := time_util.GetZeroTime(time_util.NowSec())
	todayCent := s.GetDailyChargeMoney(openZeroTime)

	if todayCent > 0 {
		data := s.GetData()
		data.DailyCharge = todayCent
		s.s2cInfo()
	}
}

func (s *CumulativeChargeBossSys) NewDay() {
	s.GetData().DailyCharge = 0
	s.GetData().DailyTimes = 0
	s.s2cInfo()
}
func (s *CumulativeChargeBossSys) Login() {
	s.s2cInfo()
}

func (s *CumulativeChargeBossSys) OnReconnect() {
	s.s2cInfo()
}

func (s *CumulativeChargeBossSys) c2sEnterFb(msg *base.Message) error {
	var req pb3.C2S_135_11
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYCumulativeChargeBossConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	player := s.GetPlayer()
	if player.GetFbId() == conf.FuBenId {
		return neterror.ParamsInvalidError("already enter fb")
	}

	times := req.Times
	data := s.GetData()
	nextChallengeTimes := data.DailyTimes + 1
	if data.DailyTimes >= times || nextChallengeTimes != times {
		return neterror.ParamsInvalidError("already challenge %d %d %d", times, data.DailyTimes, nextChallengeTimes)
	}

	timeConf := conf.TimesConf[req.Times]
	if timeConf == nil {
		return neterror.ConfNotFoundError("%s not found %d conf", s.GetPrefix(), req.Times)
	}

	if data.DailyCharge < timeConf.ChargeAmount {
		return neterror.ParamsInvalidError("charge amount %d < %d", data.DailyCharge, timeConf.ChargeAmount)
	}

	err := player.EnterFightSrv(base.LocalFightServer, fubendef.EnterCumulativeChargeBoss, &pb3.EnterPYYCumulativeChargeBossSt{
		ConfIdx:  s.ConfIdx,
		FbId:     conf.FuBenId,
		SceneId:  conf.SceneId,
		ConfName: s.ConfName,
		Times:    times,
		Id:       s.Id,
	})
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}
	logworker.LogPlayerBehavior(player, pb3.LogId_LogEnterCumulativeChargeBoss, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", times),
	})
	return nil
}

func (s *CumulativeChargeBossSys) settlePYYCumulativeChargeBossSt(req *pb3.SettlePYYCumulativeChargeBossSt) error {
	times := req.Times
	data := s.GetData()
	nextChallengeTimes := data.DailyTimes + 1
	if data.DailyTimes >= times || nextChallengeTimes != times {
		return neterror.ParamsInvalidError("already challenge %d %d %d", times, data.DailyTimes, nextChallengeTimes)
	}

	conf := jsondata.GetPYYCumulativeChargeBossConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	timeConf := conf.TimesConf[req.Times]
	if timeConf == nil {
		return neterror.ConfNotFoundError("%s not found %d conf", s.GetPrefix(), req.Times)
	}

	data.DailyTimes = nextChallengeTimes

	// 下发结算
	s.GetPlayer().SendProto3(17, 254, &pb3.S2C_17_254{
		Settle: &pb3.FbSettlement{
			FbId:      conf.FuBenId,
			Ret:       uint32(custom_id.FbSettleResultWin),
			PassTime:  req.PassTime,
			ShowAward: jsondata.StdRewardVecToPb3RewardVec(timeConf.Rewards),
			ExData:    []uint32{nextChallengeTimes},
		},
	})

	if len(timeConf.Rewards) != 0 {
		engine.GiveRewards(s.GetPlayer(), timeConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogSettleCumulativeChargeBoss,
		})
	}

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogSettleCumulativeChargeBoss, &pb3.LogPlayerCounter{
		NumArgs: uint64(nextChallengeTimes),
	})
	s.s2cInfo()
	return nil
}

func (s *CumulativeChargeBossSys) PlayerCharge(*custom_id.ActorEventCharge) {
	data := s.GetData()
	data.DailyCharge = s.GetDailyCharge()
	s.SendProto3(135, 12, &pb3.S2C_135_12{
		ActiveId:    s.Id,
		DailyCharge: data.DailyCharge,
	})
}

func eachPlayerPYYCumulativeChargeBoss(player iface.IPlayer, f func(sys *CumulativeChargeBossSys)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYCumulativeChargeBoss)
	if nil == yyList || len(yyList) == 0 {
		return
	}
	for _, yy := range yyList {
		if !yy.IsOpen() {
			continue
		}
		sys := yy.(*CumulativeChargeBossSys)
		utils.ProtectRun(func() {
			f(sys)
		})
	}
}

func handleF2GSettlePYYCumulativeChargeBossSt(player iface.IPlayer, buf []byte) {
	var req pb3.SettlePYYCumulativeChargeBossSt
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}
	eachPlayerPYYCumulativeChargeBoss(player, func(sys *CumulativeChargeBossSys) {
		err := sys.settlePYYCumulativeChargeBossSt(&req)
		if err != nil {
			player.LogError("err:%v", err)
			return
		}
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYCumulativeChargeBoss, func() iface.IPlayerYY {
		return &CumulativeChargeBossSys{}
	})

	net.RegisterYYSysProtoV2(135, 11, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*CumulativeChargeBossSys).c2sEnterFb
	})
	engine.RegisterActorCallFunc(playerfuncid.F2GSettlePYYCumulativeChargeBossSt, handleF2GSettlePYYCumulativeChargeBossSt)
	gmevent.Register("enterCumulativeChargeBossSys", func(player iface.IPlayer, args ...string) bool {
		yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYCumulativeChargeBoss)
		if nil == yyList || len(yyList) == 0 {
			return false
		}
		for _, yy := range yyList {
			msg := base.NewMessage()
			sys := yy.(*CumulativeChargeBossSys)
			sys.GetData().DailyCharge = 1000000000
			msg.SetCmd(135<<8 | 11)
			err := msg.PackPb3Msg(&pb3.C2S_135_11{
				Base: &pb3.YYBase{
					ActiveId: sys.GetId(),
				},
				Times: sys.GetData().DailyTimes + 1,
			})
			err = sys.c2sEnterFb(msg)
			if err != nil {
				player.LogError("err:%v", err)
			}
			sys.s2cInfo()
			break
		}
		return true
	}, 1)
}
