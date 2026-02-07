package actorsystem

/*
	desc:表白系统
	author: twl
	time:	2023/12/05
*/

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

// 仙缘墙
type ConfessionSys struct {
	Base
}

func (sys *ConfessionSys) OnInit() {
	sys.init()
}

func (sys *ConfessionSys) OnLogin() {
	sys.triggerQuest()
}

func (sys *ConfessionSys) OnReconnect() {
	// 初始化表白等级
	//sys.GetBinaryData().ConfessionLv = 1
	//sys.GetBinaryData().ConfessionExp = 1
	sys.triggerQuest()
}

func (sys *ConfessionSys) init() bool {
	return true
}

func (sys *ConfessionSys) OnOpen() { // 功能开启初始化等级
	sys.GetBinaryData().ConfessionLv = 1
	sys.ResetSysAttr(attrdef.SaConfessionLv)
	sys.triggerQuest()
}

// 发送界面信息
func (sys *ConfessionSys) c2sPackSend(_msg *base.Message) {
	var partnerConfessionLv uint32
	var partnerAttr map[uint32]int64
	partnerId := sys.GetBinaryData().MarryData.MarryId
	if partnerId > 0 { // 有伴侣
		data := manager.GetData(partnerId, gshare.ActorDataBase)
		if bData, ok := data.(*pb3.PlayerDataBase); ok {
			partnerConfessionLv = bData.ConfessionLv
		}
		partnerAttr = manager.GetExtraAppearAttr(partnerId)
	}
	rsp := &pb3.S2C_2_87{
		MyConfessionLv:      sys.GetBinaryData().ConfessionLv,
		Exp:                 sys.GetBinaryData().ConfessionExp,
		PartnerAttrs:        partnerAttr,
		PartnerId:           partnerId,
		PartnerConfessionLv: partnerConfessionLv,
	}
	sys.SendProto3(2, 87, rsp)
}

// 送花表白
func (sys *ConfessionSys) c2sSendConfession(msg *base.Message) error {
	var req pb3.C2S_2_88
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return neterror.ParamsInvalidError("UnpackPb3Msg SetTags :%v", err)
	}
	conf := jsondata.GetConfessionItemConf(req.ItemTid)
	if conf == nil {
		return neterror.ParamsInvalidError("GetConfessionItemConf UNKNOWN ItemTid :%v", req.ItemTid)
	}
	consume := jsondata.ConsumeVec{
		&jsondata.Consume{
			Id:    req.ItemTid,
			Count: req.ItemNum,
		},
	}

	// 判断是否是好友关系
	friendSys := sys.owner.GetSysObj(sysdef.SiFriend).(*FriendSys)
	if !friendSys.IsExistFriend(req.TargetId, custom_id.FrFriend) {
		sys.owner.SendTipMsg(tipmsgid.TpConfessionNotUFriend)
		return nil
	}
	if !sys.owner.ConsumeByConf(consume, true, common.ConsumeParams{LogId: pb3.LogId_LogConfessionConsume}) { //消耗成功 去加亲密度
		// 增加双方的亲密度
		return neterror.ConsumeFailedError("Consume failed")
	}
	friendSys.AddIntimacy(req.TargetId, conf.CloseExp*req.ItemNum)

	// 然后自己增加魅力值
	sys.GetBinaryData().ConfessionExp += conf.Exp * req.ItemNum
	newLv, newExp, isUpLv := checkUpLv(sys.GetBinaryData().ConfessionLv, sys.GetBinaryData().ConfessionExp)
	sys.GetBinaryData().ConfessionLv, sys.GetBinaryData().ConfessionExp = newLv, newExp
	sys.triggerQuest()
	//离线事件 接受者增加魅力值
	engine.SendPlayerMessage(req.TargetId, gshare.OfflineConfessMsg, &pb3.OfflineConfessMsg{
		ItemId: req.ItemTid,
		Num:    req.ItemNum,
	})
	if isUpLv {
		sys.ResetSysAttr(attrdef.SaConfessionLv)
		// 等级变动 伴侣在线通知伴侣
		sys.partnerReCalcAttr(newLv)
		sys.tellPartnerNewLv(newLv)
	}
	targetPlayer := manager.GetPlayerPtrById(req.TargetId)
	var tarName string
	if targetPlayer != nil { // 在线
		tarRsp := &pb3.S2C_2_89{
			TargetId: sys.owner.GetId(),
			NickName: sys.owner.GetName(),
			ItemTid:  req.ItemTid,
			ItemNum:  req.ItemNum,
		}
		targetPlayer.SendProto3(2, 89, tarRsp) //通知
		tarName = targetPlayer.GetMainData().ActorName
	} else {
		data := manager.GetData(req.TargetId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		tarName = data.Name
	}
	rsp := &pb3.S2C_2_88{
		MyConfessionLv: newLv,
		Exp:            newExp,
	}
	sys.SendProto3(2, 88, rsp)
	// 发送tips
	engine.BroadcastTipMsgById(conf.Tips, sys.owner.GetName(), tarName, req.ItemTid)
	effect := &pb3.S2C_12_9{EffectId: custom_id.SceneEffectConfession, Ext: conf.ItemTid}
	if conf.IsGlobal {
		engine.Broadcast(chatdef.CIWorld, 0, 12, 9, effect, 0)
	} else {
		sys.owner.SendProto3(12, 9, effect)
		if nil != targetPlayer {
			targetPlayer.SendProto3(12, 9, effect)
		}
	}
	sys.owner.TriggerEvent(custom_id.AeSendConfession, req.TargetId, tarName, uint32(conf.Exp*req.ItemNum))
	err := sys.owner.CallActorFunc(actorfuncid.SyncConfessionEvent, &pb3.CommonSt{
		U32Param:  req.ItemTid,
		U32Param2: req.ItemNum,
		U64Param:  sys.owner.GetId(),
		U64Param2: req.TargetId,
		U32Param3: time_util.NowSec(),
	})
	if err != nil {
		sys.LogError("SyncConfessionEvent err:%v", err)
		return nil
	}
	return nil
}

func checkUpLv(lv uint32, exp uint32) (uint32, uint32, bool) {
	isUpLv := false
	conf := jsondata.GetConfessionConf(lv + 1) // 找下一级
	for {
		if conf == nil { // 没有下级
			return lv, exp, isUpLv
		}
		reqExp := conf.ReqExp
		if reqExp > exp {
			break
		}
		// 有下一级
		lv++
		exp -= reqExp
		isUpLv = true
		if conf = jsondata.GetConfessionConf(lv + 1); nil == conf { // 没有下一级
			return lv, exp, isUpLv
		}
	}
	return lv, exp, isUpLv
}

// 伴侣重新计算属性
func (sys *ConfessionSys) partnerReCalcAttr(lv uint32) {
	partnerId := sys.GetBinaryData().MarryData.MarryId
	if partnerId == 0 {
		return
	}
	p := manager.GetPlayerPtrById(partnerId)
	if p == nil {
		return
	}
	sysP := p.GetSysObj(sysdef.SiConfession).(*ConfessionSys)
	sysP.ResetSysAttr(attrdef.SaConfessionLv)
	sysP.owner.SendTipMsg(tipmsgid.ConfessionLevelTips, lv)
}

// 被表白执行离线消息
func asyncBeConfession(actor iface.IPlayer, msg pb3.Message) {
	if sys, ok := actor.GetSysObj(sysdef.SiConfession).(*ConfessionSys); ok {
		sys.beConfession(msg)
	}
}

// 收到表白执行
func (sys *ConfessionSys) beConfession(msg pb3.Message) {
	st, ok := msg.(*pb3.OfflineConfessMsg)
	if !ok {
		return
	}
	// 增加魅力值 重新计算属性
	conf := jsondata.GetConfessionItemConf(st.ItemId)
	// 然后自己增加魅力值
	sys.GetBinaryData().ConfessionExp += conf.Exp * st.Num
	newLv, newExp, isUpLv := checkUpLv(sys.GetBinaryData().ConfessionLv, sys.GetBinaryData().ConfessionExp)
	sys.GetBinaryData().ConfessionLv, sys.GetBinaryData().ConfessionExp = newLv, newExp
	sys.triggerQuest()
	if isUpLv {
		sys.ResetSysAttr(attrdef.SaConfessionLv)
		sys.partnerReCalcAttr(newLv)
		sys.tellPartnerNewLv(newLv)
	}
	rsp := &pb3.S2C_2_88{
		MyConfessionLv: newLv,
		Exp:            newExp,
	}
	sys.SendProto3(2, 88, rsp)
}

// AddSelfCharm 增加自身魅力值
func (sys *ConfessionSys) AddSelfCharm(value uint32) {
	sys.GetBinaryData().ConfessionExp += value
	newLv, newExp, isUpLv := checkUpLv(sys.GetBinaryData().ConfessionLv, sys.GetBinaryData().ConfessionExp)
	if isUpLv {
		sys.ResetSysAttr(attrdef.SaConfessionLv)
		sys.tellPartnerNewLv(newLv)
	}
	sys.triggerQuest()
	rsp := &pb3.S2C_2_88{
		MyConfessionLv: newLv,
		Exp:            newExp,
	}
	sys.SendProto3(2, 88, rsp)
}

func (sys *ConfessionSys) triggerQuest() {
	sys.owner.TriggerQuestEvent(custom_id.QttConfessionLv, 0, int64(sys.GetBinaryData().ConfessionLv))
}

func (sys *ConfessionSys) tellPartnerNewLv(lv uint32) {
	partnerId := sys.GetBinaryData().MarryData.MarryId
	if partnerId == 0 {
		return
	}
	p := manager.GetPlayerPtrById(partnerId)
	if p == nil {
		return
	}

	p.SendProto3(2, 99, &pb3.S2C_2_99{ConfessionLv: lv})
}

// 表白等级属性 - 共享伴侣的一半的属性
func confessionProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	confessionLv := player.GetBinaryData().ConfessionLv
	conf := jsondata.GetConfessionConf(confessionLv)
	if nil == conf {
		return
	}
	engine.CheckAddAttrsToCalc(player, calc, conf.Attrs)
	// 共享伴侣属性
	partnerId := player.GetBinaryData().MarryData.MarryId
	if partnerId == 0 {
		return
	}
	data := manager.GetData(partnerId, gshare.ActorDataBase)
	if bData, ok := data.(*pb3.PlayerDataBase); ok {
		confP := jsondata.GetConfessionConf(bData.ConfessionLv)
		if confP == nil {
			return
		}
		// 取一半的属性值
		attrsP := getPartOfAttr(confP.Attrs, confP.Ratio)
		engine.CheckAddAttrsToCalc(player, calc, attrsP)
	}
}

// 获取部分的属性
func getPartOfAttr(attrs jsondata.AttrVec, ratio uint32) jsondata.AttrVec {
	accAttr := make(jsondata.AttrVec, 0)
	for _, attr := range attrs {
		v := attr.Value * ratio / 10000
		accAttr = append(accAttr, &jsondata.Attr{Type: attr.Type, Value: v})
	}
	return accAttr
}

// 婚姻状态变动 - 重算属性
func onMarryStateChange(actor iface.IPlayer, _args ...interface{}) {
	actor.LogDebug("婚姻状态变动 - 重算属性")
	if sys, ok := actor.GetSysObj(sysdef.SiConfession).(*ConfessionSys); ok && sys.IsOpen() {
		sys.ResetSysAttr(attrdef.SaConfessionLv)
	}
}

func useAddCharmValueItem(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	sys, ok := player.GetSysObj(sysdef.SiConfession).(*ConfessionSys)
	if !ok {
		return false, false, 0
	}

	v := conf.Param[0]
	if v <= 0 {
		player.LogError("useItem:%d err: add charm is zero!", param.ItemId)
		return false, false, 0
	}

	sys.AddSelfCharm(v * uint32(param.Count))

	return true, true, param.Count
}

func init() {
	RegisterSysClass(sysdef.SiConfession, func() iface.ISystem {
		return &ConfessionSys{}
	})
	event.RegActorEvent(custom_id.AeMarrySuccess, onMarryStateChange)
	event.RegActorEvent(custom_id.AeDivorce, onMarryStateChange)
	net.RegisterSysProto(2, 87, sysdef.SiConfession, (*ConfessionSys).c2sPackSend)
	net.RegisterSysProto(2, 88, sysdef.SiConfession, (*ConfessionSys).c2sSendConfession)
	engine.RegAttrCalcFn(attrdef.SaConfessionLv, confessionProperty)
	engine.RegisterMessage(gshare.OfflineConfessMsg, func() pb3.Message {
		return &pb3.OfflineConfessMsg{}
	}, asyncBeConfession)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemCharm, useAddCharmValueItem)
}
