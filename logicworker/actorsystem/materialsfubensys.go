package actorsystem

/**
* @Author: YangQibin
* @Desc: 材料副本
* @Date: 2023/3/21
 */

import (
	"github.com/gzjjyz/srvlib/utils"
	jsoniter "github.com/json-iterator/go"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	pyy "jjyz/gameserver/logicworker/actorsystem/yy"
	"jjyz/gameserver/logicworker/beasttidemgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type (
	materialFbLastPassInfoSt struct {
		fbId       uint32
		star       uint32
		diffLevel  uint32
		awardsRate uint32
	}

	MaterialsFbSys struct {
		Base
		data *pb3.MaterialFuBenData

		lastPassInfoSt *materialFbLastPassInfoSt
	}
)

func (sys *MaterialsFbSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *MaterialsFbSys) init() bool {
	if sys.GetBinaryData().MaterialFubenData == nil {
		data := &pb3.MaterialFuBenData{}
		sys.GetBinaryData().MaterialFubenData = data
	}
	sys.data = sys.GetBinaryData().MaterialFubenData

	if sys.data.Data == nil {
		sys.data.Data = make(map[uint32]*pb3.MaterialsFuBenSt)
	}

	for _, mfbs := range sys.data.Data {
		if nil == mfbs.PassInfo {
			mfbs.PassInfo = make(map[uint32]*pb3.MaterialsFuBenPassSt)
		}
	}
	return true
}

func (sys *MaterialsFbSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *MaterialsFbSys) OnOpen() {
	sys.init()
	sys.s2cInfo()
}

func (sys *MaterialsFbSys) OnLogin() {
	sys.SendProto3(17, 20, &pb3.S2C_17_20{Data: sys.data})
}

func (sys *MaterialsFbSys) s2cInfo() {
	sys.SendProto3(17, 20, &pb3.S2C_17_20{Data: sys.data})
}

func (sys *MaterialsFbSys) c2sInfo(msg *base.Message) error {
	sys.s2cInfo()
	return nil
}

func (sys *MaterialsFbSys) fuBenIsOpen(fubenId uint32) bool {
	fbConf := jsondata.GetMaterialsFbConf(fubenId)
	if fbConf == nil {
		return false
	}

	// 结束时间校验
	if fbConf.EndDay > 0 && gshare.GetOpenServerDay() >= fbConf.EndDay {
		return false
	}

	return fbConf.OpenSrvDay == 0 || (fbConf.OpenSrvDay > 0 && gshare.GetOpenServerDay() >= fbConf.OpenSrvDay)
}

func (sys *MaterialsFbSys) c2sBuyChallengeTimes(msg *base.Message) error {
	var req pb3.C2S_17_23
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if !sys.fuBenIsOpen(req.FuBenId) {
		return neterror.ParamsInvalidError("fuben %d not open", req.FuBenId)
	}

	fbConf := jsondata.GetMaterialsFbConf(req.FuBenId)
	if nil == fbConf {
		return neterror.ParamsInvalidError("fuben %d not exist", req.FuBenId)
	}

	// 判断次数
	fbData, ok := sys.data.Data[req.FuBenId]
	if !ok {
		fbData = &pb3.MaterialsFuBenSt{}
		sys.data.Data[req.FuBenId] = fbData
	}

	if fbData.BuyedTimes+req.Nums > fbConf.UnFreeNum {
		return neterror.ParamsInvalidError("buy limit")
	}

	consumes := jsondata.ConsumeVec{}
	for tmpBuyedTimes := fbData.BuyedTimes; tmpBuyedTimes < fbData.BuyedTimes+req.Nums; tmpBuyedTimes++ {
		var consume *jsondata.Consume
		if tmpBuyedTimes >= uint32(len(fbConf.UnFreeConsume)) {
			consume = fbConf.UnFreeConsume[len(fbConf.UnFreeConsume)-1]
		} else {
			consume = fbConf.UnFreeConsume[tmpBuyedTimes]
		}
		consumes = append(consumes, consume)
	}

	fbData.BuyedTimes += req.Nums

	if !sys.GetOwner().ConsumeByConf(consumes, true, common.ConsumeParams{LogId: pb3.LogId_LogMaterialFubenBuyTimes}) {
		return neterror.ParamsInvalidError("consume not enough")
	}

	sys.SendProto3(17, 23, &pb3.S2C_17_23{FuBenId: req.FuBenId, Nums: fbData.BuyedTimes})
	return nil
}

func (sys *MaterialsFbSys) c2sAttackFuben(msg *base.Message) error {
	var req pb3.C2S_17_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if !sys.fuBenIsOpen(req.FuBenId) {
		return neterror.ParamsInvalidError("fuben %d not open", req.FuBenId)
	}

	conf := jsondata.GetMaterialsFbLvConf(req.FuBenId, req.Level)

	if nil == conf {
		return neterror.ParamsInvalidError("fuben %d level %d not exist", req.FuBenId, req.Level)
	}

	checker := new(manager.CondChecker)
	successed, err := checker.Check(sys.GetOwner(), conf.Cond.Expr, conf.Cond.Conf, req.FuBenId, req.Level)
	if err != nil {
		return neterror.ParamsInvalidError("%s", err)
	}

	if !successed {
		return neterror.ParamsInvalidError("condition not meet")
	}

	fbConf := jsondata.GetMaterialsFbConf(req.FuBenId)

	// 判断免费次数
	fbData, ok := sys.data.Data[req.FuBenId]
	if !ok {
		fbData = &pb3.MaterialsFuBenSt{
			PassInfo: make(map[uint32]*pb3.MaterialsFuBenPassSt),
		}
		sys.data.Data[req.FuBenId] = fbData
	}

	privilegeTimes, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumMaterialFbExtraTimes)
	if fbData.ChallengedTimes >= fbConf.FreeNum+fbData.BuyedTimes+uint32(privilegeTimes) {
		return neterror.ParamsInvalidError("次数不足 chalTimes %d freeNum %d buyedTimes %d privilegeTimes %d", fbData.ChallengedTimes, fbConf.FreeNum, fbData.BuyedTimes, privilegeTimes)
	}

	err = sys.GetOwner().EnterFightSrv(base.LocalFightServer, fubendef.EnterMaterialFuben, &pb3.EnterMaterialsFuben{
		FubenId: req.FuBenId,
		DiffLev: req.Level,
	})

	if err != nil {
		return neterror.InternalError("enter material fuben failed fubenId %d level %d err %s", req.FuBenId, req.Level, err)
	}

	// 触发资源找回事件
	sys.GetOwner().TriggerEvent(custom_id.AeCompleteRetrieval, int(req.FuBenId), int(1))

	return nil
}

func (sys *MaterialsFbSys) c2sQuickAttackFuben(msg *base.Message) error {
	var req pb3.C2S_17_22
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if !sys.fuBenIsOpen(req.FuBenId) {
		return neterror.ParamsInvalidError("fuben %d not open", req.FuBenId)
	}

	if foo, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumMaterialFbQuickAttack); foo <= 0 {
		sys.GetOwner().SendTipMsg(tipmsgid.TpCantQuickAttackMaterialsFuben)
		return nil
	}

	diffLvconf := jsondata.GetMaterialsFbLvConf(req.FuBenId, req.Level)
	if nil == diffLvconf {
		return neterror.ParamsInvalidError("fuben %d level %d not exist", req.FuBenId, req.Level)
	}

	checker := new(manager.CondChecker)
	successed, err := checker.Check(sys.GetOwner(), diffLvconf.Cond.Expr, diffLvconf.Cond.Conf, req.FuBenId, req.Level)
	if err != nil {
		return neterror.ParamsInvalidError("%s", err)
	}

	if !successed {
		return neterror.ParamsInvalidError("condition not meet")
	}

	fbConf := jsondata.GetMaterialsFbConf(req.FuBenId)

	// 判断次数
	fbData, ok := sys.data.Data[req.FuBenId]
	if !ok {
		return neterror.ParamsInvalidError("haven`t pass fuben %d", req.FuBenId)
	}

	passInfo := fbData.PassInfo[req.Level]
	if nil == passInfo {
		return neterror.ParamsInvalidError("haven`t pass fuben %d level %d", req.FuBenId, req.Level)
	}

	privilegeTimes, err := sys.GetOwner().GetPrivilege(privilegedef.EnumMaterialFbExtraTimes)
	if err != nil {
		sys.owner.LogError("get privilegeTimes failed %d", privilegeTimes)
	}

	if fbData.ChallengedTimes >= fbConf.FreeNum+fbData.BuyedTimes+uint32(privilegeTimes) {
		return neterror.ParamsInvalidError("num limit")
	}

	// 判断星级是否满足扫荡要求
	if int(passInfo.Star) < len(diffLvconf.TrippleStar) {
		return neterror.ParamsInvalidError("star not meet")
	}

	// todo 暴力判断是否在材料副本中
	fbId := sys.GetOwner().GetFbId()
	ownerInFbConf := jsondata.GetMaterialsFbConf(fbId)
	if ownerInFbConf != nil {
		conf := jsondata.GetFbConf(fbId)
		sys.GetOwner().SendTipMsg(tipmsgid.EnterFbLimit, conf.FbName)
		return neterror.ParamsInvalidError("owner in materials fu ben fbId:%d", fbId)
	}

	reward := diffLvconf.NormalAwards

	// 2024.7.24 扫荡改成把剩余挑战次数花完
	retTimes := sys.getRemainTimes(req.FuBenId, fbData)
	reward = jsondata.StdRewardMulti(reward, int64(retTimes))

	// 材料副本加成(万分比)
	materialFuBenAddRate := sys.GetOwner().GetFightAttr(attrdef.MaterialFuBenAddRate)
	reward = jsondata.CalcStdRewardByRate(reward, materialFuBenAddRate)

	if ratio := beasttidemgr.GetBeastTideAddition(pyy.BTEffectTypeDoubleMaterial); ratio > 0 {
		reward = jsondata.StdRewardMulti(reward, ratio)
	}

	sys.GetOwner().TriggerEvent(custom_id.AeCompleteRetrieval, int(req.FuBenId), int(retTimes))
	sys.GetOwner().TriggerQuestEvent(custom_id.QttPassMaterialsFb, req.FuBenId, int64(retTimes))
	sys.GetOwner().TriggerQuestEvent(custom_id.QttPassAnyMaterialsFb, 0, int64(retTimes))
	sys.GetOwner().TriggerQuestEvent(custom_id.QttAchievementsPassAnyMaterialsFb, 0, int64(retTimes))

	res := &pb3.S2C_17_22{
		FuBenId:         req.FuBenId,
		Level:           req.Level,
		Awards:          jsondata.StdRewardVecToPb3RewardVec(reward),
		ChallengedTimes: fbData.ChallengedTimes,
		RetTimes:        retTimes,
	}

	sys.lastPassInfoSt = &materialFbLastPassInfoSt{
		fbId:      req.FuBenId,
		star:      passInfo.Star,
		diffLevel: req.Level,
	}

	sys.SendProto3(17, 22, res)
	return nil
}

func (sys *MaterialsFbSys) evalStar(fubenId, diffLevel, param uint32) (star int) {
	diffLevelConf := jsondata.GetMaterialsFbLvConf(fubenId, diffLevel)
	if nil == diffLevelConf {
		sys.owner.LogError("fuben %d level %d not exist", fubenId, diffLevel)
		return star
	}

	stars := []int{}
	for i := len(diffLevelConf.TrippleStar); i >= 1; i-- {
		stars = append(stars, i)
	}

	for i, p := range diffLevelConf.TrippleStar {
		if param < p {
			star = stars[i]
			break
		}
	}
	return star
}

func (sys *MaterialsFbSys) getRemainTimes(fbId uint32, data *pb3.MaterialsFuBenSt) uint32 {
	fbConf := jsondata.GetMaterialsFbConf(fbId)
	pTimes, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumMaterialFbExtraTimes)
	allTimes := fbConf.FreeNum + data.BuyedTimes + uint32(pTimes)
	return utils.MaxUInt32(0, allTimes-data.ChallengedTimes)
}

func (sys *MaterialsFbSys) checkout(settle *pb3.FbSettlement) {
	if len(settle.ExData) < 3 {
		sys.owner.LogError("len of ExData is not correct fubenId %d", settle.FbId)
		return
	}

	diffLevel := settle.ExData[0]
	evalParam := settle.ExData[1]
	awardsRate := settle.ExData[2]
	diffLevelConf := jsondata.GetMaterialsFbLvConf(settle.FbId, diffLevel)
	if nil == diffLevelConf {
		sys.owner.LogError("fuben %d level %d not exist", settle.FbId, diffLevel)
		return
	}

	fbConf := jsondata.GetMaterialsFbConf(settle.FbId)
	if fbConf == nil {
		sys.owner.LogError("fuben %d not exist", settle.FbId)
		return
	}

	fbData, ok := sys.data.Data[settle.FbId]
	if !ok {
		fbData = &pb3.MaterialsFuBenSt{
			PassInfo: make(map[uint32]*pb3.MaterialsFuBenPassSt),
		}
		sys.data.Data[settle.FbId] = fbData
	}

	// 历史最高星级
	hisMaxStar := uint32(0)
	if nil != fbData.PassInfo[diffLevel] {
		hisMaxStar = fbData.PassInfo[diffLevel].Star
	}

	res := &pb3.S2C_17_254{
		Settle: settle,
	}

	logArgs := map[string]any{}
	logArgs["passwd"] = settle.Ret == 2
	logArgs["fbId"] = settle.FbId
	logArgs["diffLevel"] = diffLevel

	if settle.Ret == 2 {
		star := 0
		if awardsRate == 0 {
			star = sys.evalStar(settle.FbId, diffLevel, evalParam)
			logArgs["star"] = star
		}

		sys.lastPassInfoSt = &materialFbLastPassInfoSt{
			fbId:       settle.FbId,
			star:       uint32(star),
			diffLevel:  diffLevel,
			awardsRate: awardsRate,
		}

		var (
			maxStar = len(diffLevelConf.TrippleStar)
			reward  jsondata.StdRewardVec
		)
		if awardsRate > 0 {
			reward = jsondata.StdRewardMultiRate(diffLevelConf.NormalAwards, float64(awardsRate)/100)
		} else {
			if star >= maxStar && hisMaxStar < uint32(maxStar) {
				reward = jsondata.MergeStdReward(diffLevelConf.NormalAwards, diffLevelConf.TrippleStarAwards)
			} else {
				reward = diffLevelConf.NormalAwards
			}
		}

		// 材料副本加成(万分比)
		materialFuBenAddRate := sys.GetOwner().GetFightAttr(attrdef.MaterialFuBenAddRate)
		reward = jsondata.CalcStdRewardByRate(reward, materialFuBenAddRate)

		if ratio := beasttidemgr.GetBeastTideAddition(pyy.BTEffectTypeDoubleMaterial); ratio > 0 {
			reward = jsondata.StdRewardMulti(reward, ratio)
		}
		res.Settle.ShowAward = jsondata.StdRewardVecToPb3RewardVec(reward)
		res.Settle.ExData = []uint32{uint32(star), diffLevel}
		sys.GetOwner().TriggerQuestEvent(custom_id.QttPassMaterialsFb, settle.FbId, 1)
		sys.GetOwner().TriggerQuestEvent(custom_id.QttPassAnyMaterialsFb, 0, 1)
		sys.GetOwner().TriggerQuestEvent(custom_id.QttAchievementsPassAnyMaterialsFb, 0, 1)
	}

	jsbytes, _ := jsoniter.Marshal(logArgs)
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogCheckoutMaterialFb, &pb3.LogPlayerCounter{
		StrArgs: string(jsbytes),
	})

	sys.SendProto3(17, 21, &pb3.S2C_17_21{
		FubenId:  settle.FbId,
		PassInfo: sys.data.Data[settle.FbId],
	})

	sys.SendProto3(17, 254, res)
}

func (sys *MaterialsFbSys) c2sRewardLastPass(msg *base.Message) error {
	var req pb3.C2S_17_24
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("%s", err)
	}

	if sys.lastPassInfoSt == nil {
		return neterror.ParamsInvalidError("no last pass info")
	}

	if !sys.fuBenIsOpen(sys.lastPassInfoSt.fbId) {
		return neterror.ParamsInvalidError("fuben %d not open", sys.lastPassInfoSt.fbId)
	}

	fbConf := jsondata.GetMaterialsFbConf(sys.lastPassInfoSt.fbId)
	if fbConf == nil {
		return neterror.ParamsInvalidError("fuben %d not exist", sys.lastPassInfoSt.fbId)
	}

	diffLevelConf := jsondata.GetMaterialsFbLvConf(sys.lastPassInfoSt.fbId, sys.lastPassInfoSt.diffLevel)
	if nil == diffLevelConf {
		return neterror.ParamsInvalidError("fuben %d level %d not exist", sys.lastPassInfoSt.fbId, sys.lastPassInfoSt.diffLevel)
	}

	fbData, ok := sys.data.Data[sys.lastPassInfoSt.fbId]
	if !ok {
		fbData = &pb3.MaterialsFuBenSt{
			PassInfo: make(map[uint32]*pb3.MaterialsFuBenPassSt),
		}

		sys.data.Data[sys.lastPassInfoSt.fbId] = fbData
	}

	passInfo, ok := fbData.PassInfo[sys.lastPassInfoSt.diffLevel]
	if !ok {
		passInfo = &pb3.MaterialsFuBenPassSt{}
		fbData.PassInfo[sys.lastPassInfoSt.diffLevel] = passInfo
	}

	var (
		maxStar         int
		reward          jsondata.StdRewardVec
		firstTripleStar bool
	)

	if awardsRate := sys.lastPassInfoSt.awardsRate; awardsRate > 0 {
		reward = jsondata.StdRewardMultiRate(diffLevelConf.NormalAwards, float64(awardsRate)/100)
	} else {
		maxStar = len(diffLevelConf.TrippleStar)

		if int(sys.lastPassInfoSt.star) >= maxStar && passInfo.Star < uint32(maxStar) {
			firstTripleStar = true
			reward = jsondata.MergeStdReward(diffLevelConf.NormalAwards, diffLevelConf.TrippleStarAwards)
		} else {
			reward = diffLevelConf.NormalAwards
		}
	}

	passInfo.Star = utils.MaxUInt32(passInfo.Star, sys.lastPassInfoSt.star)

	usedTimes := uint32(1)
	if req.IsQuick {
		// 2024.7.24 扫荡改成把剩余挑战次数花完
		usedTimes = sys.getRemainTimes(fbConf.FbId, fbData)
	}
	reward = jsondata.StdRewardMulti(reward, int64(usedTimes))

	// 材料副本加成(万分比)
	materialFuBenAddRate := sys.GetOwner().GetFightAttr(attrdef.MaterialFuBenAddRate)
	reward = jsondata.CalcStdRewardByRate(reward, materialFuBenAddRate)

	if req.Double {
		if firstTripleStar {
			return neterror.ParamsInvalidError("首次三星通关不可双倍领取")
		}

		consumes := diffLevelConf.DoubleConsume
		consumes = jsondata.ConsumeMulti(consumes, usedTimes)
		if !sys.GetOwner().ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogMaterialFubenDoubleCost}) {
			return neterror.ParamsInvalidError("double Consume failed")
		}
		reward = jsondata.StdRewardMulti(reward, 2)
	}

	if ratio := beasttidemgr.GetBeastTideAddition(pyy.BTEffectTypeDoubleMaterial); ratio > 0 {
		reward = jsondata.StdRewardMulti(reward, ratio)
	}

	// 设置状态
	fbData.ChallengedTimes += usedTimes
	sys.lastPassInfoSt = nil

	// 发送奖励
	rewardParm := common.EngineGiveRewardParam{LogId: pb3.LogId_LogMaterialFbReward}
	if !engine.GiveRewards(sys.GetOwner(), reward, rewardParm) {
		return neterror.InternalError("give reward failed")
	}

	// 触发资源找回事件
	sys.GetOwner().TriggerQuestEventRangeTimes(custom_id.QttPassFbTimes, fbConf.FbId, int64(usedTimes))
	sys.GetOwner().TriggerQuestEvent(custom_id.QttAchievementsPassFbTimes, fbConf.FbId, int64(usedTimes))
	sys.GetOwner().TriggerQuestEvent(custom_id.QttAchievementsEnterFbTimes, fbConf.FbId, int64(usedTimes))

	sys.GetOwner().TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionPassMaterialFb, int(usedTimes))

	sys.SendProto3(17, 24, &pb3.S2C_17_24{
		ChallengedTimes: fbData.ChallengedTimes,
	})
	return nil
}

func checkOutMaterialFb(player iface.IPlayer, buf []byte) {
	var req pb3.FbSettlement
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("unmarshal failed %s", err)
		return
	}
	materialsFbSys, ok := player.GetSysObj(sysdef.SiMaterialsFuben).(*MaterialsFbSys)
	if !ok || !materialsFbSys.IsOpen() {
		return
	}
	materialsFbSys.checkout(&req)
}

func gmEnterMaterialFuBen(player iface.IPlayer, args ...string) bool {
	if len(args) < 2 {
		return false
	}

	fubenId := utils.AtoUint32(args[0])
	diffLev := utils.AtoUint32(args[1])
	req := pb3.C2S_17_21{
		FuBenId: fubenId,
		Level:   diffLev,
	}

	msg := base.NewMessage()

	msg.PackPb3Msg(&req)

	materialsFbSys, ok := player.GetSysObj(sysdef.SiMaterialsFuben).(*MaterialsFbSys)
	if !ok || !materialsFbSys.IsOpen() {
		return ok
	}

	if err := materialsFbSys.c2sAttackFuben(msg); err != nil {
		player.LogError("gmEnterMaterialFuBen failed %s", err)
		return false
	}

	return true
}

func (sys *MaterialsFbSys) onNewDay() {
	for _, mfbs := range sys.data.Data {
		mfbs.ChallengedTimes = 0
		mfbs.BuyedTimes = 0
	}
	sys.SendProto3(17, 20, &pb3.S2C_17_20{Data: sys.data})
}

func init() {
	RegisterSysClass(sysdef.SiMaterialsFuben, func() iface.ISystem {
		return &MaterialsFbSys{}
	})

	net.RegisterSysProto(17, 20, sysdef.SiMaterialsFuben, (*MaterialsFbSys).c2sInfo)
	net.RegisterSysProto(17, 21, sysdef.SiMaterialsFuben, (*MaterialsFbSys).c2sAttackFuben)
	net.RegisterSysProto(17, 22, sysdef.SiMaterialsFuben, (*MaterialsFbSys).c2sQuickAttackFuben)
	net.RegisterSysProto(17, 23, sysdef.SiMaterialsFuben, (*MaterialsFbSys).c2sBuyChallengeTimes)
	net.RegisterSysProtoV2(17, 24, sysdef.SiMaterialsFuben,
		func(sys iface.ISystem) func(*base.Message) error {
			return func(msg *base.Message) error {
				return sys.(*MaterialsFbSys).c2sRewardLastPass(msg)
			}
		})

	engine.RegisterActorCallFunc(playerfuncid.CheckOutMaterialFuBen, checkOutMaterialFb)

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		materialsFbSys, ok := actor.GetSysObj(sysdef.SiMaterialsFuben).(*MaterialsFbSys)
		if !ok || !materialsFbSys.IsOpen() {
			return
		}
		materialsFbSys.onNewDay()
	})

	gmevent.Register("attackMaterialFb", gmEnterMaterialFuBen, 1)
}
