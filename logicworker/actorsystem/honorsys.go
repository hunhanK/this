/**
 * @Author: lzp
 * @Date: 2026/1/5
 * @Desc: 头衔系统
**/

package actorsystem

import (
	"errors"
	"github.com/gzjjyz/srvlib/utils"
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
)

type HonorSys struct {
	Base
	timer *time_util.Timer
}

func (sys *HonorSys) OnInit() {
	role := sys.GetBinaryData()
	if nil == role.Honors {
		role.Honors = make(map[uint32]*pb3.HonorInfo)
	}
}

// OnLogin 玩家登陆
func (sys *HonorSys) OnLogin() {
	role := sys.GetBinaryData()
	sys.checkSetTimer()

	if takeOnId := role.GetCurHonorId(); takeOnId > 0 {
		if sys.IsActive(takeOnId) {
			sys.SetTakeOnId(takeOnId)
		}
	}
}

func (sys *HonorSys) OnOpen() {
	role := sys.GetBinaryData()
	sys.checkSetTimer()

	if takeOnId := role.GetCurHonorId(); takeOnId > 0 {
		if sys.IsActive(takeOnId) {
			sys.SetTakeOnId(takeOnId)
		}
	}
	sys.ResetSysAttr(attrdef.SaHonor)
}

func (sys *HonorSys) OnAfterLogin() {
	sys.s2cInfo()
	curHonorId := sys.GetBinaryData().CurHonorId
	sys.owner.SetExtraAttr(attrdef.HonorId, attrdef.AttrValueAlias(curHonorId))
	sys.owner.SyncShowStr(custom_id.ShowStrCustomHonor)
}

func (sys *HonorSys) OnReconnect() {
	sys.s2cInfo()
}

// OnHonorOverdue 头衔过期
func (sys *HonorSys) OnHonorOverdue(id uint32) {
	sys.TakeOff(id, 0)
	sys.SendProto3(9, 7, &pb3.S2C_9_7{HonorId: id})
}

// 设置下一次超时的定时器
func (sys *HonorSys) checkSetTimer() {
	role := sys.GetBinaryData()
	if nil == role {
		return
	}
	// 遍历; 把超时过期的称号移除掉
	honors := role.Honors
	nowSec := time_util.NowSec()

	var next uint32
	for honorId, honorInfo := range honors {
		if honorInfo.EndTime <= 0 || nowSec < honorInfo.EndTime {
			if next == 0 || (honorInfo.EndTime != 0 && next > honorInfo.EndTime) {
				next = honorInfo.EndTime
			}
			continue //未过期
		}
		// 过期了
		sys.OnHonorOverdue(honorId)
		delete(honors, honorId)
	}

	if nil != sys.timer {
		sys.timer.Stop()
	}
	if next > 0 && nowSec < next {
		sys.timer = sys.owner.SetTimeout(time.Duration(next-nowSec)*time.Second, func() {
			sys.checkSetTimer()
		})
	}
}

// GetTakeOnId 当前穿戴头衔id
func (sys *HonorSys) GetTakeOnId() uint32 {
	role := sys.GetBinaryData()
	return role.GetCurHonorId()
}

// SetTakeOnId 设置当前穿戴头衔id
func (sys *HonorSys) SetTakeOnId(curHonorId uint32) {
	role := sys.GetBinaryData()
	role.CurHonorId = curHonorId

	sys.owner.SetExtraAttr(attrdef.HonorId, attrdef.AttrValueAlias(curHonorId))
	sys.owner.SyncShowStr(custom_id.ShowStrCustomHonor)
}

// 更新称号
func (sys *HonorSys) honorUpdate(honorId, option uint32) {
	honorInfo := sys.GetBinaryData().Honors[honorId]
	curHonorId := sys.GetBinaryData().GetCurHonorId()
	var rsp = &pb3.S2C_9_6{
		HonorId:    honorId,
		CurHonorId: curHonorId,
		Option:     option,
		Info:       honorInfo,
	}
	sys.SendProto3(9, 6, rsp)
}

func (sys *HonorSys) PileUp(honorId, num, option uint32, checkConsume bool) bool {
	conf := jsondata.GetHonorConfig(honorId)
	if nil == conf {
		return false
	}

	if conf.TimeOut == 0 {
		return false
	}

	if num <= 0 {
		return false
	}

	if checkConsume && !sys.owner.ConsumeByConf([]*jsondata.Consume{{Id: conf.ItemId, Count: num}}, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveHonor}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return false
	}

	role := sys.GetBinaryData()
	honorInfo, ok := role.Honors[honorId]
	if !ok {
		return false
	}

	if honorInfo.EndTime == 0 {
		return false
	}

	nowSec := time_util.NowSec()
	startTime := honorInfo.EndTime
	if startTime < nowSec {
		startTime = nowSec
	}

	honorInfo.EndTime = startTime + conf.TimeOut*num

	if conf.SpecialType == custom_id.HonorSpecialTypeCustom {
		honorInfo.ChangeTimes++
	}

	sys.honorUpdate(honorId, option)

	if honorInfo.EndTime > 0 {
		sys.checkSetTimer()
	}
	sys.owner.GetAttrSys().ResetSysAttr(attrdef.SaHonor)

	return true
}

// Active 激活称号
func (sys *HonorSys) Active(honorId, timeout, option uint32, isFormSys bool) {
	conf := jsondata.GetHonorConfig(honorId)
	if nil == conf {
		return
	}

	// 检查消耗
	if !isFormSys && !sys.owner.ConsumeByConf([]*jsondata.Consume{{Id: conf.ItemId, Count: 1}}, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveHonor}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return
	}

	// 执行激活
	role := sys.GetBinaryData()
	if nil != role.Honors[honorId] {
		return
	}
	honorInfo := &pb3.HonorInfo{
		EndTime: timeout,
		Lv:      1,
	}

	if conf.SpecialType == custom_id.HonorSpecialTypeCustom {
		honorInfo.ChangeTimes = 1
	}

	role.Honors[honorId] = honorInfo

	sys.honorUpdate(honorId, option)
	sys.GetOwner().TriggerEvent(custom_id.AeActiveFashion, &custom_id.FashionSetEvent{
		SetId:     conf.SetId,
		FType:     conf.FType,
		FashionId: conf.Id,
	})
	// 判断是否是第一个
	if len(role.Honors) == 1 {
		// 第一个称号直接穿戴
		sys.TakeOn(honorId, 0)
	}
	if honorInfo.EndTime > 0 {
		sys.checkSetTimer()
	}
	sys.owner.GetAttrSys().ResetSysAttr(attrdef.SaHonor)
}

// DisActive 回收称号
func (sys *HonorSys) DisActive(honorId uint32, needReCalc bool) {
	if honorId == 0 {
		return
	}
	sys.TakeOff(honorId, 0) // 先脱下 再去删除
	role := sys.GetBinaryData()
	delete(role.Honors, honorId)
	if needReCalc { // 需要重算属性
		sys.owner.GetAttrSys().ResetSysAttr(attrdef.SaHonor)
	}
}

// TakeOn 请求穿戴称号
func (sys *HonorSys) TakeOn(honorId, option uint32) {
	if sys.GetTakeOnId() == honorId {
		return
	}
	if !sys.IsActive(honorId) {
		return
	}

	sys.SetTakeOnId(honorId)
	sys.honorUpdate(honorId, option)
}

// TakeOff 脱称号
func (sys *HonorSys) TakeOff(honorId, option uint32) {
	if sys.GetTakeOnId() != honorId {
		return
	}
	sys.SetTakeOnId(0)
	sys.honorUpdate(honorId, option)
}

// IsActive 称号是否已激活
func (sys *HonorSys) IsActive(honorId uint32) bool {
	role := sys.GetBinaryData()
	_, isAct := role.Honors[honorId]
	return isAct // 能找到就是已激活就是true
}

// GetHonorInfo 获取称号实体
func (sys *HonorSys) GetHonorInfo(honorId uint32) (*pb3.HonorInfo, bool) {
	role := sys.GetBinaryData()
	honor, isAct := role.Honors[honorId]
	return honor, isAct
}

// 升级称号
func (sys *HonorSys) upgrade(honorId, option uint32) {
	role := sys.GetBinaryData()
	conf := jsondata.GetHonorConfig(honorId)
	honorInfo := role.Honors[honorId]
	if honorInfo == nil {
		return
	}
	//检查下一级
	if honorInfo.Lv >= uint32(len(conf.LvHonor)) { // 已满级
		return
	}
	// 检查消耗
	lvHonor := conf.LvHonor[honorInfo.Lv-1]
	if !sys.owner.ConsumeByConf([]*jsondata.Consume{{Id: conf.ItemId, Count: lvHonor.Count}}, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveHonor}) {
		return
	}
	//升级
	honorInfo.Lv += 1
	if conf.SpecialType == custom_id.HonorSpecialTypeCustom {
		honorInfo.ChangeTimes++
	}
	role.Honors[honorId] = honorInfo
	sys.honorUpdate(honorId, option)
	sys.owner.GetAttrSys().ResetSysAttr(attrdef.SaHonor)
}

func (sys *HonorSys) s2cInfo() {
	if role := sys.GetBinaryData(); nil != role {
		sys.SendProto3(9, 5, &pb3.S2C_9_5{
			Honors:     role.Honors,
			CurHonorId: sys.GetBinaryData().GetCurHonorId()})
	}
}

// 称号操作
func (sys *HonorSys) c2sHonorOption(msg *base.Message) error {
	var req pb3.C2S_9_6
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return err
	}
	id := req.GetHonorId()
	option := req.GetOption()
	conf := jsondata.GetHonorConfig(id)
	if nil == conf {
		return neterror.ParamsInvalidError("id:%d honor config nil", id)
	}

	if !sys.judgeOpen(conf.SpecialType) {
		return neterror.SysNotExistError("sys not open")
	}

	switch option {
	case custom_id.Honor_option_act:
		if !sys.IsActive(id) {
			timeOut := conf.TimeOut
			if timeOut > 0 {
				timeOut = time_util.NowSec() + timeOut
			}
			sys.Active(conf.Id, timeOut, custom_id.Honor_option_act, false)
		} else {
			sys.PileUp(id, 1, custom_id.Honor_option_act, true)
		}
	case custom_id.Honor_option_wear:
		if sys.IsActive(id) {
			sys.TakeOn(id, custom_id.Honor_option_wear)
		}
	case custom_id.Honor_option_cast:
		if sys.IsActive(id) {
			sys.TakeOff(id, custom_id.Honor_option_cast)
		}
	case custom_id.Honor_option_up: // 升级称号
		if sys.IsActive(id) {
			sys.upgrade(id, custom_id.Honor_option_up)
		}
	case custom_id.Honor_option_custom: // 升级称号
		if sys.IsActive(id) {
			return sys.opCustom(id, req.Name)
		}
	}
	return nil
}

func (sys *HonorSys) canCustom(honorId uint32, name string) (bool, error) {
	conf := jsondata.GetHonorConfig(honorId)
	if nil == conf {
		return false, neterror.ConfNotFoundError("honor %d conf is nil", honorId)
	}

	if conf.SpecialType != custom_id.HonorSpecialTypeCustom {
		return false, neterror.ConfNotFoundError("honor %d cant custom", honorId)
	}

	honorInfo := sys.GetBinaryData().Honors[honorId]
	if honorInfo == nil {
		return false, neterror.ConfNotFoundError("honor %d not active", honorId)
	}

	if honorInfo.ChangeTimes <= 0 {
		return false, neterror.ConfNotFoundError("honor %d change time not enough", honorId)
	}

	length := uint32(utf8.RuneCountInString(name))
	if conf.CustomMaxLength < length || length == 0 {
		return false, neterror.ConfNotFoundError("length is invalid")
	}

	return true, nil
}

func (sys *HonorSys) opCustom(honorId uint32, name string) error {
	if ok, err := sys.canCustom(honorId, name); !ok {
		return err
	}

	engine.SendWordMonitor(
		wordmonitor.HonorCustom,
		wordmonitor.ChangeHonorCustom,
		name,
		wordmonitoroption.WithPlayerId(sys.GetOwner().GetId()),
		wordmonitoroption.WithRawData(honorId),
		wordmonitoroption.WithCommonData(sys.GetOwner().BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(sys.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
	)
	return nil
}

func (sys *HonorSys) custom(honorId uint32, name string) error {
	if ok, err := sys.canCustom(honorId, name); !ok {
		return err
	}

	honorInfo := sys.GetBinaryData().Honors[honorId]
	honorInfo.Name = name
	honorInfo.ChangeTimes--
	sys.honorUpdate(honorId, custom_id.Honor_option_custom)
	sys.owner.SyncShowStr(custom_id.ShowStrCustomHonor)
	return nil
}

func (sys *HonorSys) judgeOpen(specialType uint32) bool {
	if specialType == custom_id.HonorSpecialTypeCustom {
		sysMgr := sys.GetOwner().GetSysMgr().(*Mgr)
		open := sysMgr.canOpenSys(sysdef.SiCustomHonor, nil)
		return open
	}
	return true
}

func GmHonor(actor iface.IPlayer, args ...string) bool {
	sys, ok := actor.GetSysObj(sysdef.SiHonor).(*HonorSys)
	if !ok {
		return false
	}
	if len(args) < 2 {
		return false
	}
	sys.Active(utils.AtoUint32(args[0]), utils.AtoUint32(args[1]), 0, true)
	return true
}

func GmHonorCancel(actor iface.IPlayer, args ...string) bool {
	sys, ok := actor.GetSysObj(sysdef.SiHonor).(*HonorSys)
	if !ok {
		return false
	}
	if len(args) < 1 {
		return false
	}
	role := sys.GetBinaryData()

	id := utils.AtoUint32(args[0])
	delete(role.Honors, id)

	sys.ResetSysAttr(attrdef.SaHonor)
	return true
}

// 计算称号属性
func honorProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	honors := player.GetBinaryData().Honors
	now := time_util.NowSec()
	for id, honorInfo := range honors {
		hConf := jsondata.GetHonorConfig(id)
		// 过期的不算
		if nil == hConf || (now > honorInfo.EndTime && honorInfo.EndTime > 0) {
			continue
		}
		// 基础属性
		engine.CheckAddAttrsToCalc(player, calc, hConf.Attr)
		// 等级属性
		if honorInfo.Lv > 0 {
			lvIndex := honorInfo.Lv - 1
			if len(hConf.LvHonor) == 0 {
				continue
			}
			engine.CheckAddAttrsToCalc(player, calc, hConf.LvHonor[lvIndex].LvAttrs)
		}
	}
}

// 激活称号
func activeHonor(actor iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.OfflineHonor)
	if !ok {
		return
	}

	nowSec := time_util.NowSec()
	// 已过期
	if st.ExpireTime > 0 && st.ExpireTime <= nowSec {
		return
	}
	if sys, ok := actor.GetSysObj(sysdef.SiHonor).(*HonorSys); ok {
		sys.Active(st.HonorId, st.ExpireTime, custom_id.Honor_option_act, true)
	}
}

func f2gOfflineHonor(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	actorId, honorId, expiredTime := msg.U64Param, msg.U32Param, msg.U32Param2
	engine.SendPlayerMessage(actorId, gshare.OfflineActiveHonor, &pb3.OfflineHonor{
		HonorId:    honorId,
		ExpireTime: expiredTime,
	})
}

func useItemHonor(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys, ok := player.GetSysObj(sysdef.SiHonor).(*HonorSys)
	if !ok || !sys.IsOpen() {
		return false, false, 0
	}

	honorConf := jsondata.GetHonorConfByItem(param.ItemId)
	if nil == honorConf {
		return false, false, 0
	}

	if !sys.judgeOpen(honorConf.SpecialType) {
		return false, false, 0
	}

	if !sys.IsActive(honorConf.Id) {
		return false, false, 0
	}

	if !sys.PileUp(honorConf.Id, uint32(param.Count), custom_id.Honor_option_act, false) {
		return false, false, 0
	}

	return true, true, param.Count
}

var _ iface.IMaxStarChecker = (*HonorSys)(nil)

func (sys *HonorSys) IsMaxStar(itemId uint32) bool {
	honorConf := jsondata.GetHonorConfByItem(itemId)
	if honorConf == nil {
		sys.LogError("honorConf not found")
		return false
	}
	id := honorConf.Id
	if !sys.IsActive(id) {
		sys.LogDebug("honor is not active")
		return false
	}
	honorInfo, _ := sys.GetHonorInfo(id)

	// 计算最大可升级等级
	maxLv := uint32(len(honorConf.LvHonor))
	if maxLv == 0 {
		sys.LogDebug("honor can't uplevel")
		return true
	}
	return honorInfo.Lv >= maxLv

}
func init() {
	RegisterSysClass(sysdef.SiHonor, func() iface.ISystem {
		return &HonorSys{}
	})

	engine.RegisterSysCall(sysfuncid.F2GOfflineHonor, f2gOfflineHonor)

	engine.RegisterMessage(gshare.OfflineActiveHonor, func() pb3.Message {
		return &pb3.OfflineHonor{}
	}, activeHonor)

	engine.RegAttrCalcFn(attrdef.SaHonor, honorProperty)
	net.RegisterSysProto(9, 6, sysdef.SiHonor, (*HonorSys).c2sHonorOption)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemHonor, useItemHonor)

	engine.RegWordMonitorOpCodeHandler(wordmonitor.ChangeHonorCustom, func(word *wordmonitor.Word) error {
		player := manager.GetPlayerPtrById(word.PlayerId)
		if nil == player {
			return nil
		}
		if word.Ret != wordmonitor2.Success {
			player.SendTipMsg(tipmsgid.TpSensitiveWord)
			return nil
		}
		honorId, ok := word.Data.(uint32)
		if !ok {
			return errors.New("honor id cant be assert uint32")
		}

		sys, ok := player.GetSysObj(sysdef.SiHonor).(*HonorSys)
		if !ok || !sys.IsOpen() {
			return errors.New("sys not open")
		}
		err := sys.custom(honorId, word.Content)
		if err != nil {
			return err
		}
		return nil
	})

	gmevent.Register("honor.active", GmHonor, 1)
	gmevent.Register("honor.cancel", GmHonorCancel, 1)
}
