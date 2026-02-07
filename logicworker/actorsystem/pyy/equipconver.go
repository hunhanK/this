/**
 * @Author: lzp
 * @Date: 2024/11/21
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type EquipConverSys struct {
	PlayerYYBase
}

func (s *EquipConverSys) Login() {
	s.s2cInfo()
}

func (s *EquipConverSys) OnReconnect() {
	s.s2cInfo()
}

func (s *EquipConverSys) OnOpen() {
	s.s2cInfo()
}

func (s *EquipConverSys) NewDay() {
	data := s.GetData()
	data.UsedTimes = 0
	s.s2cInfo()
}

func (s *EquipConverSys) GetData() *pb3.PYY_EquipConverData {
	state := s.GetYYData()
	if state.EquipConver == nil {
		state.EquipConver = make(map[uint32]*pb3.PYY_EquipConverData)
	}
	if state.EquipConver[s.Id] == nil {
		state.EquipConver[s.Id] = &pb3.PYY_EquipConverData{}
	}
	return state.EquipConver[s.Id]
}

func (s *EquipConverSys) ResetData() {
	state := s.GetYYData()
	if state.EquipConver == nil {
		return
	}
	delete(state.EquipConver, s.Id)
}

func (s *EquipConverSys) s2cInfo() {
	s.SendProto3(127, 110, &pb3.S2C_127_110{
		ActId: s.Id,
		Data:  s.GetData(),
	})
}

func (s *EquipConverSys) c2sConver(msg *base.Message) error {
	var req pb3.C2S_127_111
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYEquipConverConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not found")
	}

	converL := req.DataL
	data := s.GetData()

	if data.UsedTimes+uint32(len(converL)) > conf.DayLimit {
		return neterror.ParamsInvalidError("times limit")
	}

	isCheck := true
	for _, value := range converL {
		if !s.checkConver(value.EquipHdl, value.EquipId) {
			isCheck = false
			break
		}
	}

	if !isCheck {
		return neterror.ParamsInvalidError("conver data incorrect")
	}

	// 记录次数
	data.UsedTimes += uint32(len(req.DataL))

	// 装换消耗
	var consumes jsondata.ConsumeVec
	for _, value := range converL {
		consumes = append(consumes, s.getConverConsume(value.EquipHdl)...)
	}
	if !s.player.ConsumeByConf(consumes, false, common.ConsumeParams{
		LogId: pb3.LogId_LogPYYEquipConverConsumes,
	}) {
		return neterror.ParamsInvalidError("consume not enough")
	}

	var hdlL []uint64
	var rewards jsondata.StdRewardVec
	for _, value := range converL {
		hdlL = append(hdlL, value.EquipHdl)
		rewards = append(rewards, &jsondata.StdReward{
			Id:    value.EquipId,
			Count: 1,
		})

	}

	// 删除装备
	for _, hdl := range hdlL {
		s.player.DelItemByHand(hdl, pb3.LogId_LogPYYEquipConverConsumes)
	}

	// 获得装备
	engine.GiveRewards(s.player, rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPYYEquipConverAwards,
	})

	s.SendProto3(127, 111, &pb3.S2C_127_111{
		ActId: s.Id,
		Data:  s.GetData(),
	})
	return nil
}

func (s *EquipConverSys) checkConver(itemHdl uint64, itemId uint32) bool {
	equip := s.player.GetItemByHandle(itemHdl)
	if equip == nil {
		return false
	}

	itemConf := jsondata.GetItemConfig(equip.ItemId)
	tarItemConf := jsondata.GetItemConfig(itemId)

	if itemConf.Type != tarItemConf.Type {
		return false
	}
	if itemConf.SubType != itemConf.SubType {
		return false
	}

	if itemConf.Quality != tarItemConf.Quality {
		return false
	}
	if itemConf.Stage != tarItemConf.Stage {
		return false
	}
	if itemConf.Star != tarItemConf.Star {
		return false
	}
	return true
}

func (s *EquipConverSys) getConverConsume(itemHdl uint64) jsondata.ConsumeVec {
	equip := s.player.GetItemByHandle(itemHdl)
	conf := jsondata.GetPYYEquipConverConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}
	itemConf := jsondata.GetItemConfig(equip.ItemId)

	for _, subConf := range conf.ConverConf {
		if subConf.Quality != itemConf.Quality {
			continue
		}
		if subConf.Stage != itemConf.Stage {
			continue
		}
		if subConf.Star != itemConf.Star {
			continue
		}
		return subConf.Consume
	}

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYEquipConver, func() iface.IPlayerYY {
		return &EquipConverSys{}
	})

	net.RegisterYYSysProtoV2(127, 111, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*EquipConverSys).c2sConver
	})
}
