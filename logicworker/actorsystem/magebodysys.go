package actorsystem

import (
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"
	"math"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
)

/*
	desc:法身系统
	author: LvYuMeng
	maintainer:ChenJunJi
*/

const defaultEdictPos = 1
const defaultInitStar = 1

type MageBodySystem struct {
	Base
	*miscitem.EquipContainer
}

func (sys *MageBodySystem) GetData() *pb3.MageBody {
	binary := sys.GetBinaryData()
	mageBody := binary.GetMageBody()
	return mageBody
}

func (sys *MageBodySystem) OnInit() {
	mainData := sys.GetMainData()
	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}

	if nil == itemPool.Edicts {
		itemPool.Edicts = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewEquipContainer(&mainData.ItemPool.Edicts)

	container.TakeOnLogId = pb3.LogId_LogEdictTakeOnLogId
	container.TakeOffLogId = pb3.LogId_LogEdictTakeOffLogId

	container.AddItem = sys.owner.AddItemPtr
	container.DelItem = sys.owner.RemoveItemByHandle
	container.GetItem = sys.owner.GetItemByHandle
	container.GetBagAvailable = sys.owner.GetBagAvailableCount
	container.CheckTakeOnPosHandle = sys.CheckTakeOnPosHandle
	container.ResetProp = sys.ResetProp

	container.AfterTakeOn = sys.AfterTakeOn
	container.AfterTakeOff = sys.AfterTakeOff

	sys.EquipContainer = container

	if nil == sys.GetData() {
		binary := sys.GetBinaryData()
		binary.MageBody = &pb3.MageBody{}
	}
}

func (sys *MageBodySystem) OnOpen() {
	data := sys.GetData()
	data.Lv = 1
	data.Star = defaultInitStar
	sys.GetOwner().SetExtraAttr(attrdef.MageBodyLv, int64(data.Lv))
	sys.ResetSysAttr(attrdef.SaMageBody)
	sys.owner.TriggerQuestEventRange(custom_id.QttMageLvStar)
	sys.S2CInfo()
	sys.GetOwner().UpdateStatics(model.FieldMageBodyLv_, data.Lv)
}

func (sys *MageBodySystem) ResetProp() {
	sys.ResetSysAttr(attrdef.SaEdicts)
}

func (sys *MageBodySystem) OnLogin() {
}

func (sys *MageBodySystem) OnAfterLogin() {
	sys.S2CInfo()
	sys.GetOwner().SetExtraAttr(attrdef.MageBodyLv, int64(sys.GetData().Lv))
	sys.S2CEdictsInfo()
}

func (sys *MageBodySystem) OnReconnect() {
	sys.S2CInfo()
	sys.S2CEdictsInfo()
}

// 下发法身信息
func (sys *MageBodySystem) S2CInfo() {
	sys.SendProto3(20, 0, &pb3.S2C_20_0{
		Info: sys.GetData(),
	})
}

func (sys *MageBodySystem) EquipOnP(itemId uint32) bool {
	if _, edict := sys.GetEquipByPos(defaultEdictPos); nil != edict {
		return edict.ItemId == itemId
	}
	return false
}

func (sys *MageBodySystem) TakeOnWithItemConfAndWithoutRewardToBag(player iface.IPlayer, itemConf *jsondata.ItemConf, logId uint32) error {
	if !sys.checkItemConf(itemConf.Id) {
		return neterror.ParamsInvalidError("not meet take cond")
	}
	_, edict := sys.GetEquipByPos(defaultEdictPos)
	if nil == edict {
		return neterror.InternalError("edict equip is nil")
	}
	newEdict := sys.UpGradeEquip(sys.owner, defaultEdictPos,
		edict.ItemId, itemConf.Id, nil, false)
	sys.AfterTakeOn(newEdict)
	return nil
}

func (sys *MageBodySystem) CheckTakeOnPosHandle(st *pb3.ItemSt, pos uint32) bool {
	return sys.checkItemConf(st.GetItemId())
}

func (sys *MageBodySystem) checkItemConf(itemId uint32) bool {
	itemConf := jsondata.GetItemConfig(itemId)
	if nil == itemConf {
		return false
	}
	if !sys.owner.CheckItemCond(itemConf) {
		return false
	}
	if !itemdef.IsEdicts(itemConf.Type) {
		return false
	}
	return true
}

// 下发敕令信息
func (sys *MageBodySystem) S2CEdictsInfo() {
	if mData := sys.GetMainData(); nil != mData {
		sys.SendProto3(20, 1, &pb3.S2C_20_1{
			Edict: mData.ItemPool.Edicts,
		})
	}
}

func (sys *MageBodySystem) onBreakFail(remove argsdef.RemoveMaps) error {
	mageBody := sys.GetData()
	mageBody.FailBreakTimes++
	sys.SendProto3(20, 7, &pb3.S2C_20_7{FailBreakTimes: mageBody.FailBreakTimes})
	sys.SendProto3(20, 2, &pb3.S2C_20_2{
		Lv:     mageBody.Lv,
		Star:   mageBody.Star,
		IsFail: true,
	})
	if privilegeBack, _ := sys.owner.GetPrivilege(privilegedef.EnumMageBodyFailBack); privilegeBack > 0 {
		var backItem jsondata.StdRewardVec
		for itemId, count := range remove.ItemMap {
			backItem = append(backItem, &jsondata.StdReward{
				Id:    itemId,
				Count: int64(math.Ceil(float64(count) * float64(privilegeBack) / 10000)),
				Bind:  true,
			})
		}
		for moneyType, count := range remove.MoneyMap {
			backItem = append(backItem, &jsondata.StdReward{
				Id:    jsondata.GetMoneyIdConfByType(moneyType),
				Count: int64(math.Ceil(float64(count) * float64(privilegeBack) / 10000)),
			})
		}
		engine.GiveRewards(sys.owner, backItem, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMageBodyFailBack})
	}
	return nil
}

func (sys *MageBodySystem) c2sStarUp(msg *base.Message) error {
	mageBody := sys.GetData()
	lv := mageBody.Lv
	star := mageBody.Star

	lvConf := jsondata.GetMageBodyConfByLv(lv)
	if nil == lvConf {
		return neterror.ParamsInvalidError("magebody lvConf(%d) nil", lv)
	}

	isBreak := nil == lvConf.Star[star+1]

	var (
		upConf *jsondata.MageBodyConf
		upStar uint32
		upLv   uint32
	)

	if isBreak {
		upLv = lv + 1
		upConf = jsondata.GetMageBodyConfByLv(lv + 1)
		upStar = defaultInitStar
	} else {
		upLv = lv
		upConf = lvConf
		upStar = star + 1
	}

	if nil == upConf {
		return neterror.ConfNotFoundError("lvConf is nil")
	}

	upStarConf, ok := upConf.Star[upStar]
	if !ok {
		return neterror.ConfNotFoundError("starConf %d is nil", upStar)
	}

	combat := sys.owner.GetExtraAttr(attrdef.FightValue)
	if upStarConf.Combat > combat {
		sys.owner.SendTipMsg(tipmsgid.TpPowerNotEnough)
		return nil
	}

	success, remove := sys.owner.ConsumeByConfWithRet(upStarConf.StarConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogMageBodyStarUp})
	if !success {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	privilegeAdd, _ := sys.owner.GetPrivilege(privilegedef.EnumMageBodyBreakRatio)
	attrAddRate := uint32(sys.owner.GetFightAttr(attrdef.MageBodyBreakAddRate))
	failAddRate := mageBody.FailBreakTimes * upStarConf.FailAddRate
	if upStarConf.Rate > 0 && !random.Hit(upStarConf.Rate+uint32(privilegeAdd)+attrAddRate+failAddRate, 10000) {
		return sys.onBreakFail(remove)
	}

	mageBody.FailBreakTimes = 0
	mageBody.Lv = upLv
	mageBody.Star = upStar
	sys.GetOwner().SetExtraAttr(attrdef.MageBodyLv, int64(mageBody.Lv))
	sys.GetOwner().SetExtraAttr(attrdef.MageBodyStar, int64(mageBody.Star))

	if upStarConf.SkillId > 0 {
		if !sys.GetOwner().LearnSkill(upStarConf.SkillId, upStarConf.SkillLv, true) {
			sys.LogError("player %d learn skill %d lv %d failed", sys.GetOwner().GetId(), upStarConf.SkillId, upStarConf.SkillLv)
		}
	}

	sys.ResetSysAttr(attrdef.SaMageBody)
	rsp := &pb3.S2C_20_2{
		Lv:   mageBody.Lv,
		Star: mageBody.Star,
	}
	sys.SendProto3(20, 2, rsp)
	sys.SendProto3(20, 7, &pb3.S2C_20_7{FailBreakTimes: mageBody.FailBreakTimes})

	sys.GetOwner().UpdateStatics(model.FieldMageBodyLv_, mageBody.Lv)

	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogMageBodyStarUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"lv":   mageBody.Lv,
			"star": mageBody.Star,
		}),
	})

	sys.owner.TriggerQuestEventRange(custom_id.QttMageLvStar)
	return nil
}

func (sys *MageBodySystem) c2sExchange(msg *base.Message) error {
	var req pb3.C2S_20_5
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	mageBody := sys.GetData()
	if utils.SliceContainsUint32(mageBody.Award, req.GetId()) {
		return neterror.ParamsInvalidError("mageBody exchange has receive")
	}
	conf := jsondata.GetExchangeEdictConfById(req.GetId())
	if nil == conf {
		return neterror.ParamsInvalidError("mageBody exchange conf(%d) nil", req.GetId())
	}
	if conf.Score > 0 {
		if !sys.owner.DeductMoney(moneydef.EdictScore, conf.Score, common.ConsumeParams{LogId: pb3.LogId_LogMageBodyExchange}) {
			sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
	}
	mageBody.Award = append(mageBody.Award, req.GetId())
	engine.GiveRewards(sys.owner, conf.Award, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMageBodyExchange})
	sys.SendProto3(20, 5, &pb3.S2C_20_5{Id: req.GetId()})
	return nil
}

func (sys *MageBodySystem) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_20_3
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if defaultEdictPos != req.GetPos() {
		return neterror.ParamsInvalidError("Edict taken on pos only one")
	}

	if _, oldEdict := sys.GetEquipByPos(req.GetPos()); nil != oldEdict {
		if err := sys.Replace(req.GetHandle(), req.GetPos()); nil != err {
			return err
		}
		return nil
	}
	if err, _ := sys.TakeOn(req.GetHandle(), req.GetPos()); nil != err {
		return err
	}
	return nil
}

func (sys *MageBodySystem) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_20_4
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if defaultEdictPos != req.GetPos() {
		return neterror.ParamsInvalidError("Edict taken off pos only one")
	}
	if sys.owner.GetBagAvailableCount() <= 0 {
		sys.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}
	if err := sys.TakeOff(req.GetPos()); nil != err {
		return err
	}
	return nil
}

func (sys *MageBodySystem) AfterTakeOn(equip *pb3.ItemSt) {
	sys.SendProto3(20, 3, &pb3.S2C_20_3{Edict: equip})
}

func (sys *MageBodySystem) AfterTakeOff(equip *pb3.ItemSt, pos uint32) {
	sys.SendProto3(20, 4, &pb3.S2C_20_4{Pos: pos})
}

func calcMageBodyAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys := player.GetSysObj(sysdef.SiMageBody)
	if nil == sys || !sys.IsOpen() {
		return
	}
	binary := player.GetBinaryData()
	mageBody := binary.GetMageBody()
	if nil == mageBody {
		return
	}
	lvConf := jsondata.GetMageBodyConfByLv(mageBody.Lv)
	if nil == lvConf {
		return
	}
	starConf := lvConf.Star[mageBody.Star]
	if nil == starConf {
		return
	}
	engine.CheckAddAttrsToCalc(player, calc, starConf.Attrs)
}

func calcEdictsAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	binary := player.GetBinaryData()
	mageBody := binary.GetMageBody()
	if nil == mageBody {
		return
	}
	for _, edict := range player.GetMainData().ItemPool.Edicts {
		conf := jsondata.GetItemConfig(edict.GetItemId())
		if nil == conf {
			continue
		}
		//基础属性
		engine.CheckAddAttrsToCalc(player, calc, conf.StaticAttrs)
		//极品属性
		engine.CheckAddAttrsToCalc(player, calc, conf.PremiumAttrs)
	}
}

// 使用敕令卷轴积分: 增加虎符积分（法身系统）
func useEdictScore(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}
	score := param.Count * int64(conf.Param[0])
	if score > 0 {
		player.AddMoney(moneydef.EdictScore, score, true, pb3.LogId_LogEdictScrollUse)
	}
	return true, true, param.Count
}

func getLvStarByQuest(lv, star uint32) uint32 {
	target := jsondata.GetMageBodyLvStarConf(lv - 1)
	if lvConf := jsondata.GetMageBodyConfByLv(lv); nil != lvConf {
		for st := range lvConf.Star {
			if st <= star {
				target++
			}
		}
	}
	return target
}

func onQuestMageLvStar(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	binary := actor.GetBinaryData()
	if nil != binary && nil != binary.GetMageBody() {
		mageBody := binary.GetMageBody()
		target := getLvStarByQuest(mageBody.Lv, mageBody.Star)
		return target
	}
	return 0
}

func init() {
	RegisterSysClass(sysdef.SiMageBody, func() iface.ISystem {
		return &MageBodySystem{}
	})
	miscitem.RegCommonUseItemHandle(itemdef.UseItemEdictScore, useEdictScore)

	engine.RegAttrCalcFn(attrdef.SaMageBody, calcMageBodyAttr)
	engine.RegAttrCalcFn(attrdef.SaEdicts, calcEdictsAttr)

	gmevent.Register("magebody", func(actor iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		sys, ok := actor.GetSysObj(sysdef.SiMageBody).(*MageBodySystem)
		if !ok {
			return false
		}
		lv := utils.AtoUint32(args[0])
		star := utils.AtoUint32(args[1])
		mageBody := sys.GetData()
		lvConf := jsondata.GetMageBodyConfByLv(lv)
		if nil == lvConf {
			return false
		}
		if nil == lvConf.Star[star] {
			return false
		}

		mageBody.Lv = lv
		mageBody.Star = star
		sys.GetOwner().SetExtraAttr(attrdef.MageBodyLv, int64(mageBody.Lv))

		sys.ResetSysAttr(attrdef.SaMageBody)
		rsp := &pb3.S2C_20_2{
			Lv:   mageBody.Lv,
			Star: mageBody.Star,
		}
		sys.SendProto3(20, 2, rsp)

		sys.owner.TriggerQuestEventRange(custom_id.QttMageLvStar)
		return true
	}, 1)

	engine.RegQuestTargetProgress(custom_id.QttMageLvStar, onQuestMageLvStar)

	net.RegisterSysProto(20, 2, sysdef.SiMageBody, (*MageBodySystem).c2sStarUp)
	net.RegisterSysProto(20, 3, sysdef.SiMageBody, (*MageBodySystem).c2sTakeOn)
	net.RegisterSysProto(20, 4, sysdef.SiMageBody, (*MageBodySystem).c2sTakeOff)
	net.RegisterSysProto(20, 5, sysdef.SiMageBody, (*MageBodySystem).c2sExchange)
}
