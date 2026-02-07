package yy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/net"
	"strconv"
	"strings"
	"time"
)

type NiEnBeast struct {
	YYBase
	timer      *time_util.Timer
	monLiveMap map[uint32]bool
	srvMailAt  uint32
}

func (yy *NiEnBeast) reset() {
	yy.monLiveMap = make(map[uint32]bool)
}

func (yy *NiEnBeast) OnInit() {
	yy.reset()
	yy.CalcTime()
}

func (yy *NiEnBeast) NewDay() {
	yy.reset()
	yy.CalcTime()
	yy.broMonState()
}

func (yy *NiEnBeast) PlayerLogin(player iface.IPlayer) {
	err := yy.getMonState(player, nil)
	if err != nil {
		player.LogError("err:%v", err)
	}
}

func (yy *NiEnBeast) PlayerReconnect(player iface.IPlayer) {
	err := yy.getMonState(player, nil)
	if err != nil {
		player.LogError("err:%v", err)
	}
}

func (yy *NiEnBeast) getConf() *jsondata.YYNiEnBeastConf {
	return jsondata.GetYYNiEnBeastConf(yy.ConfName, yy.ConfIdx)
}

func (yy *NiEnBeast) refreshBoss() {
	conf := yy.getConf()
	if nil == conf {
		return
	}
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FCreateNiEnBeastFb, &pb3.CommonSt{
		U32Param:  yy.GetId(),
		U32Param2: yy.GetConfIdx(),
	})
	if err != nil {
		yy.LogError("err:%v", err)
		return
	}
	yy.monLiveMap[conf.Monster.MonId] = true
	engine.Broadcast(chatdef.CIWorld, 0, 8, 23, &pb3.S2C_8_23{
		ActiveId:  yy.GetId(),
		MonsterId: conf.Monster.MonId,
	}, 0)
}

func (yy *NiEnBeast) broMonState() {
	engine.Broadcast(chatdef.CIWorld, 0, 8, 22, &pb3.S2C_8_22{
		ActiveId:   yy.GetId(),
		MonLiveMap: yy.monLiveMap,
		SrvMailAt:  yy.srvMailAt,
	}, 0)
}

func (yy *NiEnBeast) timeStringToSeconds(timeStr string) (int, error) {
	// 解析时间字符串
	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid time format")
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}

	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}

	seconds, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, err
	}

	// 计算总秒数
	totalSeconds := hours*3600 + minutes*60 + seconds
	return totalSeconds, nil
}

func (yy *NiEnBeast) CalcTime() {
	if nil != yy.timer {
		yy.timer.Stop()
		yy.timer = nil
	}
	conf := yy.getConf()
	if nil == conf {
		return
	}
	nowSec := time_util.NowSec()
	confTime := time_util.GetDaysZeroTime(0)
	// conf.Time 是 "17:00:00"
	// 转换为今天的时间戳
	seconds, _ := yy.timeStringToSeconds(conf.Time)
	genTimeAt := confTime + uint32(seconds)
	if nowSec <= genTimeAt {
		yy.timer = timer.SetTimeout(time.Duration(genTimeAt-nowSec)*time.Second, func() {
			yy.LogDebug("-----------------------年兽来袭刷新boss-----------------------")
			yy.refreshBoss()
			yy.broMonState()
		})
	}

}

func (yy *NiEnBeast) BossDeath(bossId uint32) {
	_, ok := yy.monLiveMap[bossId]
	if !ok {
		return
	}
	yy.monLiveMap[bossId] = false
	yy.broMonState()
}

func (yy *NiEnBeast) enterFb(player iface.IPlayer, _ *base.Message) error {
	err := player.EnterFightSrv(base.SmallCrossServer, fubendef.EnterNiEnBeastFb, &pb3.CommonSt{
		U32Param:  yy.GetId(),
		U32Param2: yy.GetConfIdx(),
	})
	if err != nil {
		player.LogError("err:%v", err)
		return err
	}
	return nil
}

func (yy *NiEnBeast) getRankData(player iface.IPlayer, _ *base.Message) error {
	err := player.CallActorSmallCrossFunc(actorfuncid.G2FGetNiEnBeastRankReq, &pb3.CommonSt{
		U32Param:  yy.GetId(),
		U32Param2: yy.GetConfIdx(),
	})
	if err != nil {
		player.LogError("err:%v", err)
		return err
	}
	return nil
}

func (yy *NiEnBeast) getMonState(player iface.IPlayer, _ *base.Message) error {
	player.SendProto3(8, 22, &pb3.S2C_8_22{
		ActiveId:   yy.GetId(),
		MonLiveMap: yy.monLiveMap,
		SrvMailAt:  yy.srvMailAt,
	})
	return nil
}

func f2gSyncNiEnBeast(buf []byte) {
	var req pb3.CommonSt
	if nil != pb3.Unmarshal(buf, &req) {
		return
	}
	yyId := req.U32Param
	rangeAllNiEnBeast(func(yy iface.IYunYing) {
		if yy.GetId() != yyId {
			return
		}
		yy.(*NiEnBeast).BossDeath(req.U32Param3)
	})
}

func niEnBeastRefresh(_ iface.IPlayer, _ ...string) bool {
	rangeAllNiEnBeast(func(yy iface.IYunYing) {
		yy.(*NiEnBeast).refreshBoss()
	})
	return true
}

func rangeAllNiEnBeast(doLogic func(ying iface.IYunYing)) {
	allYY := yymgr.GetAllYY(yydefine.YYNiEnBeast)
	for _, v := range allYY {
		if !v.IsOpen() {
			continue
		}
		utils.ProtectRun(func() {
			doLogic(v)
		})
	}
}
func niEnBeastEnter(player iface.IPlayer, args ...string) bool {
	rangeAllNiEnBeast(func(yy iface.IYunYing) {
		err := yy.(*NiEnBeast).enterFb(player, nil)
		if err != nil {
			player.LogError("err:%v", err)
			return
		}
	})
	return true
}

func f2gSyncNiEnBeastSrvAwardsAt(buf []byte) {
	var req pb3.CommonSt
	if nil != pb3.Unmarshal(buf, &req) {
		return
	}
	yyId := req.U32Param
	rangeAllNiEnBeast(func(yy iface.IYunYing) {
		if yy.GetId() != yyId {
			return
		}
		yy.(*NiEnBeast).srvMailAt = time_util.NowSec()
		yy.(*NiEnBeast).broMonState()
	})
}

func init() {
	yymgr.RegisterYYType(yydefine.YYNiEnBeast, func() iface.IYunYing {
		return &NiEnBeast{}
	})

	engine.RegisterSysCall(sysfuncid.F2GSyncNiEnBeastDeath, f2gSyncNiEnBeast)
	engine.RegisterSysCall(sysfuncid.F2GSyncNiEnBeastSrvAwardsAt, f2gSyncNiEnBeastSrvAwardsAt)
	net.RegisterGlobalYYSysProto(8, 20, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*NiEnBeast).enterFb
	})
	net.RegisterGlobalYYSysProto(8, 21, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*NiEnBeast).getRankData
	})
	net.RegisterGlobalYYSysProto(8, 22, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*NiEnBeast).getMonState
	})

	gmevent.Register("nienbeast", niEnBeastRefresh, 1)
	gmevent.Register("nienbeast.enter", niEnBeastEnter, 1)
}
