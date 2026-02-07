package actorsystem

import (
	"errors"
	wordmonitor2 "github.com/gzjjyz/wordmonitor"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"time"
	"unicode/utf8"

	"github.com/gzjjyz/srvlib/utils"
)

/*
desc:称号系统
author: twl
*/
type TitleSys struct {
	Base
	timer *time_util.Timer
}

func (sys *TitleSys) OnInit() {
	role := sys.GetBinaryData()
	if nil == role.TitleLs {
		role.TitleLs = make(map[uint32]*pb3.TitleInfo)
	}
}

// OnLogin 玩家登陆
func (sys *TitleSys) OnLogin() {
	role := sys.GetBinaryData()
	sys.checkSetTimer()

	if takeOnId := role.GetCurrTitleId(); takeOnId > 0 {
		if sys.IsActive(takeOnId) {
			sys.SetTakeOnId(takeOnId)
		}
	}
}

func (sys *TitleSys) OnOpen() {
	role := sys.GetBinaryData()
	sys.checkSetTimer()

	if takeOnId := role.GetCurrTitleId(); takeOnId > 0 {
		if sys.IsActive(takeOnId) {
			sys.SetTakeOnId(takeOnId)
		}
	}
	sys.ResetSysAttr(attrdef.SaTitle)
}

func (sys *TitleSys) OnAfterLogin() {
	sys.s2cInfo()
	currTitleId := sys.GetBinaryData().CurrTitleId
	sys.owner.SetExtraAttr(attrdef.TitleId, attrdef.AttrValueAlias(currTitleId))
	sys.owner.SyncShowStr(custom_id.ShowStrCustomTitle)
}

func (sys *TitleSys) OnReconnect() {
	sys.s2cInfo()
}

// OnTitleOverdue 称号过期
func (sys *TitleSys) OnTitleOverdue(id uint32) {
	sys.TakeOff(id, 0)
	sys.SendProto3(9, 3, &pb3.S2C_9_3{TitleId: id})
}

// 设置下一次超时的定时器
func (sys *TitleSys) checkSetTimer() {
	role := sys.GetBinaryData()
	if nil == role {
		return
	}
	// 遍历; 把超时过期的称号移除掉
	titleLs := role.TitleLs
	nowSec := time_util.NowSec()

	var next uint32
	for titleId, titleInfo := range titleLs {
		if titleInfo.EndTime <= 0 || nowSec < titleInfo.EndTime {
			if next == 0 || (titleInfo.EndTime != 0 && next > titleInfo.EndTime) {
				next = titleInfo.EndTime
			}
			continue //未过期
		}
		// 过期了
		sys.OnTitleOverdue(titleId)
		delete(titleLs, titleId)
	}
	//sys.ResetSysAttr(property.TitleProperty)
	if nil != sys.timer {
		sys.timer.Stop()
	}
	if next > 0 && nowSec < next {
		sys.timer = sys.owner.SetTimeout(time.Duration(next-nowSec)*time.Second, func() {
			sys.checkSetTimer()
		})
	}
}

// GetTakeOnId 当前穿戴称号id
func (sys *TitleSys) GetTakeOnId() uint32 {
	role := sys.GetBinaryData()
	return role.GetCurrTitleId()
}

// SetTakeOnId 设置当前穿戴称号id
func (sys *TitleSys) SetTakeOnId(currTitleId uint32) {
	role := sys.GetBinaryData()
	role.CurrTitleId = currTitleId

	sys.owner.SetExtraAttr(attrdef.TitleId, attrdef.AttrValueAlias(currTitleId))
	sys.owner.SyncShowStr(custom_id.ShowStrCustomTitle)
}

// 更新称号
func (sys *TitleSys) titleUpdate(titleId, option uint32) {
	titleInfo := sys.GetBinaryData().TitleLs[titleId]
	currTitleId := sys.GetBinaryData().GetCurrTitleId()
	var rsp = &pb3.S2C_9_2{
		TitleId:     titleId,
		CurrTitleId: currTitleId,
		Option:      option,
		Info:        titleInfo,
	}
	sys.SendProto3(9, 2, rsp)
}

func (sys *TitleSys) PileUp(titleId, num, option uint32, checkConsume bool) bool {
	conf := jsondata.GetTitleConfig(titleId)
	if nil == conf {
		return false
	}

	if conf.TimeOut == 0 {
		return false
	}

	if num <= 0 {
		return false
	}

	if checkConsume && !sys.owner.ConsumeByConf([]*jsondata.Consume{{Id: conf.ItemId, Count: num}}, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveTitle}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return false
	}

	role := sys.GetBinaryData()
	titleInfo, ok := role.TitleLs[titleId]
	if !ok {
		return false
	}

	if titleInfo.EndTime == 0 {
		return false
	}

	nowSec := time_util.NowSec()
	startTime := titleInfo.EndTime
	if startTime < nowSec {
		startTime = nowSec
	}

	titleInfo.EndTime = startTime + conf.TimeOut*num

	if conf.SpecialType == custom_id.TitleSpecialTypeCustom {
		titleInfo.ChangeTimes++
	}

	sys.titleUpdate(titleId, option)

	if titleInfo.EndTime > 0 {
		sys.checkSetTimer()
	}
	sys.owner.GetAttrSys().ResetSysAttr(attrdef.SaTitle)

	return true
}

// Active 激活称号
func (sys *TitleSys) Active(titleId, timeout, option uint32, isFormSys bool) {
	conf := jsondata.GetTitleConfig(titleId)
	if nil == conf {
		return
	}

	// 检查消耗
	if !isFormSys && !sys.owner.ConsumeByConf([]*jsondata.Consume{{Id: conf.ItemId, Count: 1}}, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveTitle}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return
	}

	// 执行激活
	role := sys.GetBinaryData()
	if nil != role.TitleLs[titleId] {
		return
	}
	titleInfo := &pb3.TitleInfo{
		EndTime: timeout,
		Lv:      1,
	}

	if conf.SpecialType == custom_id.TitleSpecialTypeCustom {
		titleInfo.ChangeTimes = 1
	}

	role.TitleLs[titleId] = titleInfo

	sys.titleUpdate(titleId, option)
	sys.GetOwner().TriggerEvent(custom_id.AeRareTitleActiveFashion, &custom_id.FashionSetEvent{
		FType:     conf.FType,
		FashionId: conf.Id,
	})
	sys.GetOwner().TriggerEvent(custom_id.AeActiveFashion, &custom_id.FashionSetEvent{
		SetId:     conf.SetId,
		FType:     conf.FType,
		FashionId: conf.Id,
	})
	// 判断是否是第一个
	if len(role.TitleLs) == 1 {
		// 记录一下第一的称号的名字
		sys.owner.TriggerEvent(custom_id.AeActFirstTitle, titleId)
		// 第一个称号直接穿戴
		sys.TakeOn(titleId, 0)
	}
	if titleInfo.EndTime > 0 {
		sys.checkSetTimer()
	}
	sys.owner.GetAttrSys().ResetSysAttr(attrdef.SaTitle)
}

// DisActive 回收称号
func (sys *TitleSys) DisActive(titleId uint32, needReCalc bool) {
	if titleId == 0 {
		return
	}
	sys.TakeOff(titleId, 0) // 先脱下 再去删除
	role := sys.GetBinaryData()
	titleMap := role.TitleLs
	delete(titleMap, titleId)
	if needReCalc { // 需要重算属性
		sys.owner.GetAttrSys().ResetSysAttr(attrdef.SaTitle)
	}
}

// TakeOn 请求穿戴称号
func (sys *TitleSys) TakeOn(titleId, option uint32) {
	if sys.GetTakeOnId() == titleId {
		return
	}
	if !sys.IsActive(titleId) {
		//sys.owner.SendTipMsg(common.TpItemNotEnough)
		return
	}

	sys.SetTakeOnId(titleId)
	sys.titleUpdate(titleId, option)
}

// TakeOff 脱称号
func (sys *TitleSys) TakeOff(titleId, option uint32) {
	if sys.GetTakeOnId() != titleId {
		return
	}
	sys.SetTakeOnId(0)
	sys.titleUpdate(titleId, option)
}

// IsActive 称号是否已激活
func (sys *TitleSys) IsActive(titleId uint32) bool {
	role := sys.GetBinaryData()
	_, isAct := role.TitleLs[titleId]
	return isAct // 能找到就是已激活就是true
}

// GetTitleInfo 获取称号实体
func (sys *TitleSys) GetTitleInfo(titleId uint32) (*pb3.TitleInfo, bool) {
	role := sys.GetBinaryData()
	title, isAct := role.TitleLs[titleId]
	return title, isAct
}

// 升级称号
func (sys *TitleSys) upgrade(titleId, option uint32) {
	role := sys.GetBinaryData()
	conf := jsondata.GetTitleConfig(titleId)
	titleInfo := role.TitleLs[titleId]
	if titleInfo == nil {
		return
	}
	//检查下一级
	if titleInfo.Lv >= uint32(len(conf.LvTitle)) { // 已满级
		return
	}
	// 检查消耗
	lvTitle := conf.LvTitle[titleInfo.Lv-1]
	if !sys.owner.ConsumeByConf([]*jsondata.Consume{{Id: conf.ItemId, Count: lvTitle.Count}}, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveTitle}) {
		return
	}
	//升级
	titleInfo.Lv += 1
	if conf.SpecialType == custom_id.TitleSpecialTypeCustom {
		titleInfo.ChangeTimes++
	}
	role.TitleLs[titleId] = titleInfo
	sys.titleUpdate(titleId, option)
	sys.owner.GetAttrSys().ResetSysAttr(attrdef.SaTitle)
}

func (sys *TitleSys) s2cInfo() {
	if role := sys.GetBinaryData(); nil != role {
		sys.SendProto3(9, 1, &pb3.S2C_9_1{TitleLs: role.TitleLs,
			CurrTitleId: sys.GetBinaryData().GetCurrTitleId()})
	}
}

// 称号操作
func (sys *TitleSys) c2sTitleOption(msg *base.Message) error {
	var req pb3.C2S_9_2
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return neterror.ParamsInvalidError("UnpackPb3Msg c2sTitleOption :%v", err)
	}
	id := req.GetTitleId()
	option := req.GetOption()
	conf := jsondata.GetTitleConfig(id)
	if nil == conf {
		return neterror.ParamsInvalidError("c2sTitleOption conf nil id:%v", id)
	}

	if !sys.judgeOpen(conf.SpecialType) {
		return neterror.SysNotExistError("sys not open")
	}

	switch option {
	case custom_id.Title_option_act:
		if !sys.IsActive(id) {
			timeOut := conf.TimeOut
			if timeOut > 0 {
				timeOut = time_util.NowSec() + timeOut
			}
			sys.Active(conf.Id, timeOut, custom_id.Title_option_act, false)
		} else {
			sys.PileUp(id, 1, custom_id.Title_option_act, true)
		}
	case custom_id.Title_option_wear:
		if sys.IsActive(id) {
			sys.TakeOn(id, custom_id.Title_option_wear)
		}
	case custom_id.Title_option_cast:
		if sys.IsActive(id) {
			sys.TakeOff(id, custom_id.Title_option_cast)
		}
	case custom_id.Title_option_up: // 升级称号
		if sys.IsActive(id) {
			sys.upgrade(id, custom_id.Title_option_up)
		}
	case custom_id.Title_option_custom: // 升级称号
		if sys.IsActive(id) {
			return sys.opCustom(id, req.Name)
		}
	}
	return nil
}

func (sys *TitleSys) canCustom(titleId uint32, name string) (bool, error) {
	conf := jsondata.GetTitleConfig(titleId)
	if nil == conf {
		return false, neterror.ConfNotFoundError("title %d conf is nil", titleId)
	}

	if conf.SpecialType != custom_id.TitleSpecialTypeCustom {
		return false, neterror.ConfNotFoundError("title %d cant custom", titleId)
	}

	titleInfo := sys.GetBinaryData().TitleLs[titleId]
	if titleInfo == nil {
		return false, neterror.ConfNotFoundError("title %d not active", titleId)
	}

	if titleInfo.ChangeTimes <= 0 {
		return false, neterror.ConfNotFoundError("title %d change time not enough", titleId)
	}

	length := uint32(utf8.RuneCountInString(name))
	if conf.CustomMaxLength < length || length == 0 {
		return false, neterror.ConfNotFoundError("length is invalid")
	}

	return true, nil
}

func (sys *TitleSys) opCustom(titleId uint32, name string) error {
	if ok, err := sys.canCustom(titleId, name); !ok {
		return err
	}

	engine.SendWordMonitor(
		wordmonitor.TitleCustom,
		wordmonitor.ChangeTitleCustom,
		name,
		wordmonitoroption.WithPlayerId(sys.GetOwner().GetId()),
		wordmonitoroption.WithRawData(titleId),
		wordmonitoroption.WithCommonData(sys.GetOwner().BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(sys.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
	)
	return nil
}

func (sys *TitleSys) custom(titleId uint32, name string) error {
	if ok, err := sys.canCustom(titleId, name); !ok {
		return err
	}

	titleInfo := sys.GetBinaryData().TitleLs[titleId]
	titleInfo.Name = name
	titleInfo.ChangeTimes--
	sys.titleUpdate(titleId, custom_id.Title_option_custom)
	sys.owner.SyncShowStr(custom_id.ShowStrCustomTitle)
	return nil
}

func (sys *TitleSys) judgeOpen(specialType uint32) bool {
	if specialType == custom_id.TitleSpecialTypeCustom {
		sysMgr := sys.GetOwner().GetSysMgr().(*Mgr)
		open := sysMgr.canOpenSys(sysdef.SiCustomTitle, nil)
		return open
	}
	return true
}

func GmTitle(actor iface.IPlayer, args ...string) bool {
	sys, ok := actor.GetSysObj(sysdef.SiTitle).(*TitleSys)
	if !ok {
		return false
	}
	if len(args) < 2 {
		return false
	}
	sys.Active(utils.AtoUint32(args[0]), utils.AtoUint32(args[1]), 0, true)
	return true
}

func GmTitleCancel(actor iface.IPlayer, args ...string) bool {
	sys, ok := actor.GetSysObj(sysdef.SiTitle).(*TitleSys)
	if !ok {
		return false
	}
	if len(args) < 1 {
		return false
	}
	role := sys.GetBinaryData()

	id := utils.AtoUint32(args[0])
	delete(role.TitleLs, id)

	sys.ResetSysAttr(attrdef.SaTitle)

	return true
}

// 计算称号属性
func titleProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	titleSys := player.GetBinaryData().TitleLs
	now := time_util.NowSec()
	for u, titleInfo := range titleSys {
		tConf := jsondata.GetTitleConfig(u)
		// 过期的不算
		if nil == tConf || (now > titleInfo.EndTime && titleInfo.EndTime > 0) {
			continue
		}
		// 基础属性
		engine.CheckAddAttrsToCalc(player, calc, tConf.Attr)
		// 等级属性
		if titleInfo.Lv > 0 {
			lvIndex := titleInfo.Lv - 1
			if len(tConf.LvTitle) == 0 {
				continue
			}
			engine.CheckAddAttrsToCalc(player, calc, tConf.LvTitle[lvIndex].Lvattrs)
		}
	}
}

// 激活称号
func activeTitle(actor iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.OfflineTitle)
	if !ok {
		return
	}

	nowSec := time_util.NowSec()
	// 已过期
	if st.ExpireTime > 0 && st.ExpireTime <= nowSec {
		return
	}
	if sys, ok := actor.GetSysObj(sysdef.SiTitle).(*TitleSys); ok {
		sys.Active(st.TitleId, st.ExpireTime, custom_id.Title_option_act, true)
	}
}

func f2gOfflineTitle(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	actorId, titleId, expiredTime := msg.U64Param, msg.U32Param, msg.U32Param2
	engine.SendPlayerMessage(actorId, gshare.OfflineActiveTitle, &pb3.OfflineTitle{
		TitleId:    titleId,
		ExpireTime: expiredTime,
	})
}

func useItemTitle(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys, ok := player.GetSysObj(sysdef.SiTitle).(*TitleSys)
	if !ok || !sys.IsOpen() {
		return false, false, 0
	}

	titleConf := jsondata.GetTitleConfByItem(param.ItemId)
	if nil == titleConf {
		return false, false, 0
	}

	if !sys.judgeOpen(titleConf.SpecialType) {
		return false, false, 0
	}

	if !sys.IsActive(titleConf.Id) {
		return false, false, 0
	}

	if !sys.PileUp(titleConf.Id, uint32(param.Count), custom_id.Title_option_act, false) {
		return false, false, 0
	}

	return true, true, param.Count
}

var _ iface.IMaxStarChecker = (*TitleSys)(nil)

func (sys *TitleSys) IsMaxStar(itemId uint32) bool {
	titleConf := jsondata.GetTitleConfByItem(itemId)
	if titleConf == nil {
		sys.LogError("TitleConf not found")
		return false
	}
	id := titleConf.Id
	if !sys.IsActive(id) {
		sys.LogDebug("Title is not active")
		return false
	}
	titleInfo, _ := sys.GetTitleInfo(id)

	// 计算最大可升级等级
	maxLv := uint32(len(titleConf.LvTitle))
	if maxLv == 0 {
		sys.LogDebug("Title can't uplevel")
		return true
	}
	return titleInfo.Lv >= maxLv

}
func init() {
	RegisterSysClass(sysdef.SiTitle, func() iface.ISystem {
		return &TitleSys{}
	})

	engine.RegisterSysCall(sysfuncid.F2GOfflineTitle, f2gOfflineTitle)

	engine.RegisterMessage(gshare.OfflineActiveTitle, func() pb3.Message {
		return &pb3.OfflineTitle{}
	}, activeTitle)

	engine.RegAttrCalcFn(attrdef.SaTitle, titleProperty)
	net.RegisterSysProto(9, 2, sysdef.SiTitle, (*TitleSys).c2sTitleOption)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemTitle, useItemTitle)

	engine.RegWordMonitorOpCodeHandler(wordmonitor.ChangeTitleCustom, func(word *wordmonitor.Word) error {
		player := manager.GetPlayerPtrById(word.PlayerId)
		if nil == player {
			return nil
		}
		if word.Ret != wordmonitor2.Success {
			player.SendTipMsg(tipmsgid.TpSensitiveWord)
			return nil
		}
		titleId, ok := word.Data.(uint32)
		if !ok {
			return errors.New("title id cant be assert uint32")
		}

		sys, ok := player.GetSysObj(sysdef.SiTitle).(*TitleSys)
		if !ok || !sys.IsOpen() {
			return errors.New("sys not open")
		}
		err := sys.custom(titleId, word.Content)
		if err != nil {
			return err
		}
		return nil
	})

	gmevent.Register("title.active", GmTitle, 1)
	gmevent.Register("title.cancel", GmTitleCancel, 1)
}
