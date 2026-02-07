/**
 * @Author: zjj
 * @Date: 2025年8月6日
 * @Desc: 定制名帖
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	wordmonitor2 "github.com/gzjjyz/wordmonitor"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
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
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"unicode/utf8"
)

type CustomCardSys struct {
	Base
}

func (s *CustomCardSys) s2cInfo() {
	s.SendProto3(9, 80, &pb3.S2C_9_80{
		Data: s.getData(),
	})
}

func (s *CustomCardSys) getData() *pb3.CustomCardData {
	data := s.GetBinaryData().CustomCardData
	if data == nil {
		s.GetBinaryData().CustomCardData = &pb3.CustomCardData{}
		data = s.GetBinaryData().CustomCardData
	}
	if data.CustomCards == nil {
		data.CustomCards = make(map[uint32]*pb3.CustomCardInfo)
	}
	if data.ActiveSetFlag == nil {
		data.ActiveSetFlag = make(map[uint32]uint32)
	}
	return data
}

func (s *CustomCardSys) OnReconnect() {
	s.s2cInfo()
}

func (s *CustomCardSys) OnLogin() {
	s.s2cInfo()
}

func (s *CustomCardSys) OnOpen() {
	s.s2cInfo()
}

func (s *CustomCardSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_9_81
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	id := req.Id
	data := s.getData()
	info := data.CustomCards[id]
	if info != nil {
		return neterror.ParamsInvalidError("%d already active", id)
	}

	config := jsondata.GetCustomCardConfig(id)
	if config == nil {
		return neterror.ConfNotFoundError("%d not found config", id)
	}

	starConfig := jsondata.GetCustomCardStarConfig(id, 0)
	if starConfig == nil {
		return neterror.ConfNotFoundError("%d %d not found config", id, 0)
	}

	owner := s.GetOwner()
	if len(starConfig.Consume) == 0 || !owner.ConsumeByConf(starConfig.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogCustomCardActive}) {
		return neterror.ConsumeFailedError("%d consume failed", id)
	}
	info = &pb3.CustomCardInfo{
		Id: id,
	}
	data.CustomCards[id] = info
	s.SendProto3(9, 81, &pb3.S2C_9_81{
		Info: info,
	})
	s.checkSetActive(config.SetId)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogCustomCardActive, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
	})
	s.ResetSysAttr(attrdef.SaCustomCard)
	return nil
}

func (s *CustomCardSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_9_82
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	id := req.Id
	data := s.getData()
	info := data.CustomCards[id]
	if info == nil {
		return neterror.ParamsInvalidError("%d not active", id)
	}

	config := jsondata.GetCustomCardConfig(id)
	if config == nil {
		return neterror.ConfNotFoundError("%d not found config", id)
	}
	nextStar := info.Star + 1
	starConfig := jsondata.GetCustomCardStarConfig(id, nextStar)
	if starConfig == nil {
		return neterror.ConfNotFoundError("%d %d not found config", id, nextStar)
	}

	owner := s.GetOwner()
	if len(starConfig.Consume) == 0 || !owner.ConsumeByConf(starConfig.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogCustomCardUpStar}) {
		return neterror.ConsumeFailedError("%d consume failed", id)
	}
	info.Star = nextStar
	s.SendProto3(9, 82, &pb3.S2C_9_82{
		Info: info,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogCustomCardUpStar, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d", nextStar),
	})
	s.ResetSysAttr(attrdef.SaCustomCard)
	return nil
}

func (s *CustomCardSys) c2sCustom(msg *base.Message) error {
	var req pb3.C2S_9_83
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	id := req.Id
	data := s.getData()
	info := data.CustomCards[id]
	if info == nil {
		return neterror.ParamsInvalidError("%d not active", id)
	}

	starConfig := jsondata.GetCustomCardStarConfig(id, info.Star)
	if starConfig == nil {
		return neterror.ConfNotFoundError("%d %d not found config", id, info.Star)
	}
	if starConfig.CustomTimes == 0 || info.CustomTimes >= starConfig.CustomTimes {
		return neterror.ParamsInvalidError("custom times has err %d %d", info.CustomTimes, starConfig.CustomTimes)
	}

	config := jsondata.GetCustomCardConfig(id)
	if config == nil {
		return neterror.ConfNotFoundError("%d not found config", id)
	}

	// 校验类型
	switch config.Pos {
	case appeardef.AppearPos_CustomCardDeclaration, appeardef.AppearPos_CustomCardBackground:
	default:
		return neterror.ParamsInvalidError("%d not custom", config.Pos)
	}

	if !engine.CheckNameSpecialCharacter(req.Content) {
		return neterror.ParamsInvalidError("customTip content character limit %d", req.Id)
	}

	size := uint32(utf8.RuneCountInString(req.Content))
	if config.CnSize == 0 || size > config.CnSize {
		return neterror.ParamsInvalidError("customTip content len limit %d", req.Id)
	}

	if size == 0 {
		req.Content = config.Dec
	}

	engine.SendWordMonitor(wordmonitor.CardCustom, wordmonitor.ChangeCustomCard, req.Content,
		wordmonitoroption.WithPlayerId(s.GetOwner().GetId()),
		wordmonitoroption.WithRawData(req.Id),
		wordmonitoroption.WithCommonData(s.GetOwner().BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(s.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
	)
	return nil
}

func (s *CustomCardSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_9_84
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	id := req.Id
	data := s.getData()
	info := data.CustomCards[id]
	if info == nil {
		return neterror.ParamsInvalidError("%d not active", id)
	}
	config := jsondata.GetCustomCardConfig(id)
	if config == nil {
		return neterror.ConfNotFoundError("%d not found config", id)
	}
	owner := s.GetOwner()
	if req.Dress {
		var dressSysId = uint32(appeardef.AppearSys_CustomCard)
		if config.DressSysId > 0 {
			dressSysId = config.DressSysId
		}
		owner.TakeOnAppear(config.Pos, &pb3.SysAppearSt{
			SysId:    dressSysId,
			AppearId: id,
		}, true)
	} else {
		owner.TakeOffAppear(config.Pos)
	}
	s.SendProto3(9, 84, &pb3.S2C_9_84{
		Id:    id,
		Dress: req.Dress,
	})
	return nil
}

func (s *CustomCardSys) GetCustomCardInfo(id uint32) (*pb3.CustomCardInfo, bool) {
	data := s.getData()
	info := data.CustomCards[id]
	if info == nil {
		return nil, false
	}
	return info, true
}

func (s *CustomCardSys) checkSetActive(setId uint32) {
	ids := jsondata.GetCustomCardIdsBySetId(setId)
	if ids == nil || len(ids) == 0 {
		return
	}
	config := jsondata.GetCustomCardSetConfig(setId)
	if config == nil {
		return
	}
	data := s.getData()
	flag := data.ActiveSetFlag[setId]
	var activeNum uint32
	for _, id := range ids {
		if data.CustomCards[id] == nil {
			continue
		}
		activeNum += 1
	}
	if activeNum == 0 {
		return
	}
	var newFlag = flag
	var tip bool
	for _, sets := range config.Sets {
		if utils.IsSetBit(newFlag, sets.Idx) {
			continue
		}
		if activeNum < sets.SuitNum {
			continue
		}
		newFlag = utils.SetBit(newFlag, sets.Idx)
		if !tip {
			tip = sets.Tip
		}
	}
	if newFlag == flag {
		return
	}
	data.ActiveSetFlag[setId] = newFlag
	s.SendProto3(9, 85, &pb3.S2C_9_85{
		Id:   setId,
		Flag: newFlag,
	})
	owner := s.GetOwner()
	if tip {
		engine.BroadcastTipMsgById(tipmsgid.CustomCardActiveSet, owner.GetId(), owner.GetName(), config.Name)
	}
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogCustomCardActiveSet, &pb3.LogPlayerCounter{
		NumArgs: uint64(setId),
		StrArgs: fmt.Sprintf("%d", newFlag),
	})
}

func (s *CustomCardSys) onCustom(id uint32, content string) {
	data := s.getData()
	info := data.CustomCards[id]
	if info == nil {
		return
	}
	info.CustomTimes += 1
	info.Content = content
	s.SendProto3(9, 83, &pb3.S2C_9_83{Info: info})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogCustomCardContent, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d_%s", info.CustomTimes, content),
	})
	s.GetOwner().SyncShowStr(custom_id.ShowStrCustomCardDeclaration)
	s.GetOwner().SyncShowStr(custom_id.ShowStrCustomCardBackground)
}

func (s *CustomCardSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	owner := s.GetOwner()
	for id, flag := range data.ActiveSetFlag {
		if flag == 0 {
			continue
		}
		config := jsondata.GetCustomCardSetConfig(id)
		if config == nil {
			return
		}
		for _, sets := range config.Sets {
			if !utils.IsSetBit(flag, sets.Idx) {
				continue
			}
			if len(sets.Attrs) == 0 {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, sets.Attrs)
		}
	}
	for _, customCardInfo := range data.CustomCards {
		config := jsondata.GetCustomCardConfig(customCardInfo.Id)
		if config == nil {
			continue
		}
		conf := config.StarConf[customCardInfo.Star]
		if conf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(owner, calc, conf.Attrs)
	}
}

func (s *CustomCardSys) IsMaxStar(relatedItem uint32) bool {
	config := jsondata.GetCustomCardByRelatedItemId(relatedItem)
	if config == nil {
		return false
	}
	id := config.Id
	data := s.getData()
	info := data.CustomCards[id]
	if info == nil {
		return false
	}

	nextStar := info.Star + 1
	starConfig := jsondata.GetCustomCardStarConfig(id, nextStar)
	if starConfig != nil {
		return false
	}
	return true
}

func handleChangeCustomCard(word *wordmonitor.Word) error {
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
		return neterror.ParamsInvalidError("customTip id cant be assert uint32")
	}

	sys, ok := player.GetSysObj(sysdef.SiCustomCard).(*CustomCardSys)
	if !ok || !sys.IsOpen() {
		return neterror.SysNotExistError("sys not open")
	}
	sys.onCustom(id, word.Content)
	return nil
}

func handleSaCustomCard(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiCustomCard).(*CustomCardSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiCustomCard, func() iface.ISystem {
		return &CustomCardSys{}
	})

	net.RegisterSysProtoV2(9, 81, sysdef.SiCustomCard, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CustomCardSys).c2sActive
	})
	net.RegisterSysProtoV2(9, 82, sysdef.SiCustomCard, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CustomCardSys).c2sUpStar
	})
	net.RegisterSysProtoV2(9, 83, sysdef.SiCustomCard, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CustomCardSys).c2sCustom
	})
	net.RegisterSysProtoV2(9, 84, sysdef.SiCustomCard, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CustomCardSys).c2sDress
	})
	engine.RegAttrCalcFn(attrdef.SaCustomCard, handleSaCustomCard)
	engine.RegWordMonitorOpCodeHandler(wordmonitor.ChangeCustomCard, handleChangeCustomCard)
}
