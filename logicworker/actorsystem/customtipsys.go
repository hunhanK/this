/**
 * @Author: lzp
 * @Date: 2025/3/31
 * @Desc:
**/

package actorsystem

import (
	wordmonitor2 "github.com/gzjjyz/wordmonitor"
	"github.com/pkg/errors"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"unicode/utf8"
)

type CustomTipSys struct {
	Base
}

func (s *CustomTipSys) OnReconnect() {
	s.s2cInfo()
}

func (s *CustomTipSys) OnLogin() {
	s.s2cInfo()
	if s.GetCustomTip(uint32(tipmsgid.TpCustomLocalFairyPlaceMasterOnline)) != nil {
		s.owner.BroadcastCustomTipMsgById(tipmsgid.TpCustomLocalFairyPlaceMasterOnline, s.GetOwner().GetName())
	}
}

func (s *CustomTipSys) GetData() map[uint32]*pb3.CustomTip {
	binary := s.GetBinaryData()
	if binary.CustomTipData == nil {
		binary.CustomTipData = make(map[uint32]*pb3.CustomTip)
	}
	return binary.CustomTipData
}

func (s *CustomTipSys) GetCustomTip(tipMsgId uint32) *pb3.CustomTip {
	data := s.GetData()

	var customType uint32
	switch tipMsgId {
	case tipmsgid.TpCustomLocalFairyPlaceMasterOnline:
		customType = custom_id.CustomTipType2
	case tipmsgid.TpCustomVocalize:
		customType = custom_id.CustomTipType6
	default:
	}

	for _, v := range data {
		if v.Type == customType && v.IsUsed {
			return v
		}
	}
	return nil
}

func (s *CustomTipSys) PackFightCustomTip(createData *pb3.CreateActorData) {
	if nil == createData {
		return
	}
	createData.CustomTipData = make(map[uint32]*pb3.CustomTip)
	if !s.IsOpen() {
		return
	}

	data := s.GetData()
	for _, v := range data {
		if !v.IsUsed {
			continue
		}
		createData.CustomTipData[v.Type] = v
	}
}

func (s *CustomTipSys) s2cInfo() {
	s.SendProto3(2, 204, &pb3.S2C_2_204{Data: s.GetData()})
}

func (s *CustomTipSys) s2cTipInfo(id uint32) {
	data := s.GetData()
	if _, ok := data[id]; !ok {
		s.SendProto3(2, 207, &pb3.S2C_2_207{CustomTip: nil})
		return
	}
	s.SendProto3(2, 207, &pb3.S2C_2_207{CustomTip: data[id]})
}

func (s *CustomTipSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_2_202
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetCustomTipConf(req.Id)
	if conf == nil {
		return neterror.ConfNotFoundError("customTip=%d not found", req.Id)
	}

	starConf := jsondata.GetCustomTipStarConf(req.Id, 0)
	if starConf == nil {
		return neterror.ConfNotFoundError("customTip=%d not found", req.Id)
	}

	data := s.GetData()
	_, ok := data[req.Id]
	if ok {
		return neterror.ParamsInvalidError("customTip=%d has activated", req.Id)
	}

	if !s.GetOwner().ConsumeByConf(starConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogCustomTipUpStar}) {
		return neterror.ParamsInvalidError("customTip active consume not enough: %v", req.Id)
	}

	data[req.Id] = &pb3.CustomTip{Id: req.Id, Type: conf.Type, Star: 0}
	s.onUse(req.Id)
	s.onStarChange(req.Id)
	s.SendProto3(2, 202, &pb3.S2C_2_202{Id: req.Id})
	s.s2cInfo()
	return nil
}

func (s *CustomTipSys) onStarChange(id uint32) {
	data := s.GetData()
	iData, ok := data[id]
	if !ok {
		return
	}
	starConf := jsondata.GetCustomTipStarConf(id, iData.Star)
	if starConf == nil {
		return
	}
	s.ResetSysAttr(attrdef.SaCustomTip)
}

func (s *CustomTipSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_2_203
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.GetData()
	iData, ok := data[req.Id]
	if !ok {
		return neterror.ParamsInvalidError("customTip not activated: %d", req.Id)
	}

	nextStar := iData.Star + 1
	starConf := jsondata.GetCustomTipStarConf(req.Id, nextStar)
	if starConf == nil {
		return neterror.ConfNotFoundError("customTip star conf not found: %d, %d", req.Id, nextStar)
	}

	if !s.GetOwner().ConsumeByConf(starConf.Consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogCustomTipUpStar}) {
		return neterror.ParamsInvalidError("customTip star consume not enough: %v", req.Id)
	}

	iData.Star = nextStar
	s.onStarChange(req.Id)
	s.SendProto3(2, 203, &pb3.S2C_2_203{Id: req.Id, Star: nextStar})
	s.s2cTipInfo(req.Id)
	return nil
}

func (s *CustomTipSys) c2sCustomContent(msg *base.Message) error {
	var req pb3.C2S_2_205
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetCustomTipConf(req.Id)
	if conf == nil || conf.Type == custom_id.CustomTipType6 {
		return neterror.ParamsInvalidError("customTip id:%d type err", req.Id)
	}

	data := s.GetData()
	iData, ok := data[req.Id]
	if !ok {
		return neterror.ParamsInvalidError("customTip not activated: %d", req.Id)
	}

	starConf := jsondata.GetCustomTipStarConf(req.Id, iData.Star)
	if starConf.CustomTimes-iData.CustomTimes <= 0 {
		return neterror.ParamsInvalidError("customTip customTimes limit %d", req.Id)
	}

	maxLen := int(jsondata.GlobalUint("customTipContentLen"))
	contentLen := utf8.RuneCountInString(req.Content)
	if contentLen > maxLen || contentLen <= 0 {
		return neterror.ParamsInvalidError("customTip content len limit %d", req.Id)
	}

	if !engine.CheckNameSpecialCharacter(req.Content) {
		return neterror.ParamsInvalidError("customTip content character limit %d", req.Id)
	}

	engine.SendWordMonitor(wordmonitor.TipCustom, wordmonitor.ChangeTipCustom, req.Content,
		wordmonitoroption.WithPlayerId(s.GetOwner().GetId()),
		wordmonitoroption.WithRawData(req.Id),
		wordmonitoroption.WithCommonData(s.GetOwner().BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(s.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
	)
	return nil
}

func (s *CustomTipSys) c2sCustomTipUse(msg *base.Message) error {
	var req pb3.C2S_2_206
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetCustomTipConf(req.Id)
	if conf == nil {
		return neterror.ConfNotFoundError("customTip=%d not found", req.Id)
	}

	data := s.GetData()
	_, ok := data[req.Id]
	if !ok {
		return neterror.ParamsInvalidError("customTip not activated: %d", req.Id)
	}

	s.onUse(req.Id)
	s.SendProto3(2, 206, &pb3.S2C_2_206{Id: req.Id})
	s.s2cInfo()
	return nil
}

func (s *CustomTipSys) setCustomTip(id uint32, isUse bool) {
	data := s.GetData()
	if _, ok := data[id]; !ok {
		return
	}
	data[id].IsUsed = isUse

	cType := data[id].Type
	var err error
	if isUse {
		err = s.owner.CallActorFunc(actorfuncid.G2FAddCustomTip, &pb3.CustomTipAdd{Id: cType, CustomTip: data[id]})
	} else {
		err = s.owner.CallActorFunc(actorfuncid.G2FRemoveCustomTip, &pb3.CustomTipRemove{Id: cType})
	}
	if err != nil {
		s.owner.LogError("err: %v", err)
	}
}

func (s *CustomTipSys) onUse(id uint32) {
	conf := jsondata.GetCustomTipConf(id)
	if conf == nil {
		return
	}
	data := s.GetData()
	if _, ok := data[id]; !ok {
		return
	}

	if data[id].IsUsed {
		s.setCustomTip(id, false)
		return
	}

	s.setCustomTip(id, true)
	s.owner.SendTipMsg(tipmsgid.TakeAppearChangeSuccess)
	for _, iData := range data {
		tmpConf := jsondata.GetCustomTipConf(iData.Id)
		if tmpConf == nil {
			continue
		}
		if conf.Type != tmpConf.Type {
			continue
		}
		if iData.IsUsed && iData.Id != id {
			s.setCustomTip(iData.Id, false)
		}
	}
}

func (s *CustomTipSys) onCustom(id uint32, content string) {
	data := s.GetData()
	custom, ok := data[id]
	if !ok {
		return
	}
	custom.CustomTimes += 1
	custom.Content = content
	if custom.IsUsed {
		err := s.owner.CallActorFunc(actorfuncid.G2FChangeCustomTip, &pb3.CustomTipChange{Id: custom.Type, CustomTip: custom})
		if err != nil {
			s.owner.LogError("err: %v", err)
		}
	}
	s.SendProto3(2, 205, &pb3.S2C_2_205{Id: id, Content: content})
	s.s2cTipInfo(id)
}

func (s *CustomTipSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	setMap := make(map[uint32]uint32) //k:setId, v:count
	for _, v := range data {
		conf := jsondata.GetCustomTipConf(v.Id)
		if conf == nil {
			continue
		}
		starConf := jsondata.GetCustomTipStarConf(v.Id, v.Star)
		if starConf == nil {
			continue
		}

		setMap[conf.SetId] += 1
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, starConf.Attrs)
	}

	// 套装属性
	for setId, num := range setMap {
		setConf := jsondata.GetCustomTipSetConf(setId)
		if setConf == nil || len(setConf.Sets) == 0 {
			continue
		}
		for _, v := range setConf.Sets {
			if num < v.SuitNum {
				continue
			}
			engine.CheckAddAttrsToCalc(s.GetOwner(), calc, v.Attrs)
		}
	}
}

func customTipProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiCustomTip).(*CustomTipSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func handleChangeTipCustom(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}
	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.TpSensitiveWord)
		return nil
	}
	id, ok := word.Data.(uint32)
	if !ok {
		return errors.New("customTip id cant be assert uint32")
	}

	sys, ok := player.GetSysObj(sysdef.SiCustomTip).(*CustomTipSys)
	if !ok || !sys.IsOpen() {
		return errors.New("sys not open")
	}
	sys.onCustom(id, word.Content)
	return nil
}

var _ iface.IMaxStarChecker = (*CustomTipSys)(nil)

func (s *CustomTipSys) IsMaxStar(relateItem uint32) bool {
	tipId := jsondata.GetCustomTipIdByRelateItem(relateItem)
	if tipId == 0 {
		s.LogError("CustomTip don't exist")
		return false
	}
	data := s.GetData()
	customTip, ok := data[tipId]
	if !ok {
		s.LogDebug("CustomTip %d is not activated", tipId)
		return false
	}

	nextStar := customTip.Star + 1
	nextStarConf := jsondata.GetCustomTipStarConf(tipId, nextStar)

	return nextStarConf == nil
}

func init() {
	RegisterSysClass(sysdef.SiCustomTip, func() iface.ISystem {
		return &CustomTipSys{}
	})
	net.RegisterSysProto(2, 202, sysdef.SiCustomTip, (*CustomTipSys).c2sActivate)
	net.RegisterSysProto(2, 203, sysdef.SiCustomTip, (*CustomTipSys).c2sUpStar)
	net.RegisterSysProto(2, 205, sysdef.SiCustomTip, (*CustomTipSys).c2sCustomContent)
	net.RegisterSysProto(2, 206, sysdef.SiCustomTip, (*CustomTipSys).c2sCustomTipUse)
	engine.RegAttrCalcFn(attrdef.SaCustomTip, customTipProperty)
	engine.RegWordMonitorOpCodeHandler(wordmonitor.ChangeTipCustom, handleChangeTipCustom)
}
