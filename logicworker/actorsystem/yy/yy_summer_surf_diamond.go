/**
 * @Author: LvYuMeng
 * @Date: 2025/6/16
 * @Desc:
**/

package yy

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
)

type YYSummerSurfDiamond struct {
	YYBase
	remainDiamond int64
}

func (yy *YYSummerSurfDiamond) OnInit() {
}

func (yy *YYSummerSurfDiamond) OnOpen() {
	yy.broadcastRemainDiamond()
}

func (yy *YYSummerSurfDiamond) PlayerLogin(player iface.IPlayer) {
	yy.sendRemainDiamond(player)
}

func (yy *YYSummerSurfDiamond) PlayerReconnect(player iface.IPlayer) {
	yy.sendRemainDiamond(player)
}

func (yy *YYSummerSurfDiamond) sendRemainDiamond(player iface.IPlayer) {
	player.SendProto3(75, 95, &pb3.S2C_75_95{
		ActiveId:      yy.Id,
		RemainDiamond: yy.remainDiamond,
	})
}

func (yy *YYSummerSurfDiamond) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.SummerSurf {
		return
	}
	delete(globalVar.YyDatas.SummerSurf, yy.GetId())
}

func (yy *YYSummerSurfDiamond) GetChangeLibConf(libId uint32) *jsondata.LotteryLibConf {
	conf := yy.GetConf()
	if nil == conf {
		return nil
	}

	if conf.DiamondLibId != libId {
		return nil
	}

	dropConf := conf.GetSummerSurfDropConf(yy.remainDiamond)
	if nil == dropConf {
		return nil
	}

	libConf := jsondata.ShallowCopyLotteryLibConf(libId)
	if nil == libConf {
		return nil
	}

	libConf.Rate = dropConf.Rate

	return libConf
}

func (yy *YYSummerSurfDiamond) GetConf() *jsondata.YYSummerSurfDiamondConfig {
	return jsondata.GetSummerSurfDiamondConf(yy.ConfName, yy.ConfIdx)
}

func (yy *YYSummerSurfDiamond) outBountyPool(libId uint32) (jsondata.StdRewardVec, bool) {
	if yy.remainDiamond == 0 {
		logger.LogError("仙玉库赏金池积累为0")
		return nil, false
	}

	conf := yy.GetConf()
	if nil == conf {
		logger.LogError("yy %d conf is nil", yy.Id)
		return nil, false
	}

	if conf.DiamondLibId != libId {
		return nil, false
	}

	dropConf := conf.GetSummerSurfDropConf(yy.remainDiamond)

	diamondBase := yy.remainDiamond
	diamondMin := dropConf.Min
	diamondMax := dropConf.Max

	var money int64
	if dropConf.Step > 0 {
		step := random.Interval64(0, (diamondMax-diamondMin)/dropConf.Step)
		money += diamondBase * (diamondMin + step*dropConf.Step) / 10000
	}

	if yy.remainDiamond < money { //正常不应该出现一次掏空奖池
		logger.LogError("money %d ,remain %d, exceed limit", money, yy.remainDiamond)
		return nil, false
	}

	if money == 0 {
		logger.LogError("仙玉计算错误")
		return nil, false
	}

	yy.remainDiamond -= money
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2COpSummerSurfDiamond, &pb3.G2COpSummerSurfDiamond{
		YyId:    yy.Id,
		Diamond: money,
		IsAdd:   false,
	})

	rewards := jsondata.StdRewardVec{
		&jsondata.StdReward{
			Id:    jsondata.GetMoneyIdConfByType(moneydef.Diamonds),
			Count: money,
		},
	}

	return rewards, true
}

func (yy *YYSummerSurfDiamond) judge(event *custom_id.ActDrawEvent) bool {
	conf := yy.GetConf()
	if nil == conf {
		return false
	}

	if conf.ActType != event.ActType {
		return false
	}

	if conf.ActId > 0 && conf.ActId != event.ActId {
		return false
	}

	yy.PutBountyPool(int64(conf.DiamondSingle * event.Times))

	return true
}

func (yy *YYSummerSurfDiamond) broadcastRemainDiamond() {
	yy.Broadcast(75, 95, &pb3.S2C_75_95{
		ActiveId:      yy.Id,
		RemainDiamond: yy.remainDiamond,
	})
}

func (yy *YYSummerSurfDiamond) PutBountyPool(addDiamond int64) {
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2COpSummerSurfDiamond, &pb3.G2COpSummerSurfDiamond{
		YyId:    yy.Id,
		Diamond: addDiamond,
		IsAdd:   true,
	})
}

func syncSummerSurfDiamond(buf []byte) {
	var req pb3.C2GSyncSummerSurfDiamond
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		logger.LogError("C2GSyncSummerSurfDiamond err:%v", err)
		return
	}

	yy := yymgr.GetYYByActId(req.YyId)
	if yy == nil || !yy.IsOpen() {
		return
	}

	sys := yy.(*YYSummerSurfDiamond)
	sys.remainDiamond = req.RemainDiamond

	sys.broadcastRemainDiamond()
}

func init() {
	yymgr.RegisterYYType(yydefine.YYSummerSurfDiamond, func() iface.IYunYing {
		return &YYSummerSurfDiamond{}
	})

	engine.RegisterSysCall(sysfuncid.C2GSyncSummerSurfDiamond, syncSummerSurfDiamond)

	event.RegSysEvent(custom_id.SeCrossDisconnect, func(args ...interface{}) {
		allYY := yymgr.GetAllYY(yydefine.YYSummerSurfDiamond)
		for _, iYY := range allYY {
			sys, exist := iYY.(*YYSummerSurfDiamond)
			if !exist || !sys.IsOpen() {
				continue
			}
			sys.remainDiamond = 0
			sys.broadcastRemainDiamond()
			return
		}
	})

	event.RegActorEvent(custom_id.AeActDrawTimes, func(player iface.IPlayer, args ...interface{}) {
		if len(args) < 1 {
			return
		}

		drawEvent, ok := args[0].(*custom_id.ActDrawEvent)
		if !ok {
			return
		}

		allYY := yymgr.GetAllYY(yydefine.YYSummerSurfDiamond)
		for _, iYY := range allYY {
			sys, exist := iYY.(*YYSummerSurfDiamond)
			if !exist || !sys.IsOpen() {
				continue
			}
			sys.judge(drawEvent)
		}
	})

	lotterylibs.RegLotteryAwards(lotterylibs.LotteryAwards_SummerSurfDiamond, func(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool) jsondata.StdRewardVec {
		allYY := yymgr.GetAllYY(yydefine.YYSummerSurfDiamond)
		for _, iYY := range allYY {
			sys, exist := iYY.(*YYSummerSurfDiamond)
			if !exist || !sys.IsOpen() {
				continue
			}
			rewards, ok := sys.outBountyPool(libId)
			if !ok {
				continue
			}
			return rewards
		}
		return nil
	})

	gmevent.Register("addSummerSurfDiamond", func(player iface.IPlayer, args ...string) bool {
		allYY := yymgr.GetAllYY(yydefine.YYSummerSurfDiamond)
		for _, iYY := range allYY {
			sys, exist := iYY.(*YYSummerSurfDiamond)
			if !exist || !sys.IsOpen() {
				continue
			}
			sys.PutBountyPool(utils.AtoInt64(args[0]))
			return true
		}
		return false
	}, 1)
}
