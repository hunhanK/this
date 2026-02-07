/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"strings"
)

type ImmortalElixirSys struct {
	Base
}

func (s *ImmortalElixirSys) s2cInfo() {
	s.SendProto3(10, 70, &pb3.S2C_10_70{
		Data: s.getData(),
	})
}

func (s *ImmortalElixirSys) getData() *pb3.ImmortalElixirData {
	data := s.GetBinaryData().ImmortalElixirData
	if data == nil {
		s.GetBinaryData().ImmortalElixirData = &pb3.ImmortalElixirData{}
		data = s.GetBinaryData().ImmortalElixirData
	}
	if data.ImmortalElixirMap == nil {
		data.ImmortalElixirMap = make(map[uint32]*pb3.ImmortalElixirEntry)
	}
	if data.ImmortalElixirSpiritMap == nil {
		data.ImmortalElixirSpiritMap = make(map[uint32]*pb3.ImmortalElixirSpiritEntry)
	}
	return data
}

func (s *ImmortalElixirSys) OnReconnect() {
	s.s2cInfo()
}

func (s *ImmortalElixirSys) OnLogin() {
	s.s2cInfo()
}

func (s *ImmortalElixirSys) OnOpen() {
	s.s2cInfo()
}

func (s *ImmortalElixirSys) c2sUseElixir(msg *base.Message) error {
	var req pb3.C2S_10_71
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	if len(req.ItemMap) == 0 {
		return neterror.ParamsInvalidError("params invalid")
	}
	var totalConsume jsondata.ConsumeVec
	owner := s.GetOwner()
	for itemId, count := range req.ItemMap {
		config := jsondata.GetImmortalElixirConfig(itemId)
		if config == nil {
			return neterror.ConfNotFoundError("immortal elixir use, itemId: %d, not exist", itemId)
		}
		itemCount := owner.GetItemCount(itemId, -1)
		if uint32(itemCount) < count {
			return neterror.ParamsInvalidError("itemId: %d, not enough %d %d", itemId, itemId, count)
		}
		totalConsume = append(totalConsume, &jsondata.Consume{
			Id:    itemId,
			Count: count,
		})
	}
	if !owner.ConsumeByConf(totalConsume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogImmortalElixirUse,
	}) {
		return neterror.ParamsInvalidError("consume error")
	}
	var resp pb3.S2C_10_71
	resp.ImmortalElixirMap = make(map[uint32]*pb3.ImmortalElixirEntry)
	var logStr strings.Builder
	for itemId, count := range req.ItemMap {
		elixir := s.getImmortalElixir(itemId)
		elixir.Count += count
		resp.ImmortalElixirMap[itemId] = elixir
		logStr.WriteString(fmt.Sprintf("%d_%d", itemId, count))
		logStr.WriteString("|")
	}
	s.SendProto3(10, 71, &resp)
	s.ResetSysAttr(attrdef.SaImmortalElixir)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogImmortalElixirUse, &pb3.LogPlayerCounter{
		StrArgs: logStr.String(),
	})
	return nil
}
func (s *ImmortalElixirSys) c2sSpiritUpStar(msg *base.Message) error {
	var req pb3.C2S_10_72
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	id := req.Id
	config := jsondata.GetImmortalElixirSpiritConfig(id)
	if config == nil {
		return neterror.ConfNotFoundError("immortal elixir spirit up star, itemId: %d, not exist", id)
	}
	spirit := s.getImmortalElixirSpirit(id)
	nextStarConf := config.GetStarConf(spirit.Star + 1)
	if nextStarConf == nil {
		return neterror.ParamsInvalidError("immortal elixir spirit up star, itemId: %d, star: %d, not exist", id, spirit.Star+1)
	}
	owner := s.GetOwner()
	if !owner.ConsumeByConf(nextStarConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogImmortalElixirUse,
	}) {
		return neterror.ParamsInvalidError("consume error")
	}
	spirit.Star++
	s.SendProto3(10, 72, &pb3.S2C_10_72{
		Entry: spirit,
	})
	s.ResetSysAttr(attrdef.SaImmortalElixir)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogImmortalElixirSpiritUpStar, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d", spirit.Star),
	})
	return nil
}

func (s *ImmortalElixirSys) getImmortalElixir(itemId uint32) *pb3.ImmortalElixirEntry {
	data := s.getData()
	entry := data.ImmortalElixirMap[itemId]
	if entry == nil {
		entry = &pb3.ImmortalElixirEntry{
			Id:    itemId,
			Count: 0,
		}
		data.ImmortalElixirMap[itemId] = entry
	}
	return entry
}

func (s *ImmortalElixirSys) getImmortalElixirSpirit(id uint32) *pb3.ImmortalElixirSpiritEntry {
	data := s.getData()
	entry := data.ImmortalElixirSpiritMap[id]
	if entry == nil {
		entry = &pb3.ImmortalElixirSpiritEntry{
			Id:   id,
			Star: 0,
		}
		data.ImmortalElixirSpiritMap[id] = entry
	}
	return entry
}

func handleSaImmortalElixir(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys := getImmortalElixirSys(player)
	if sys == nil {
		return
	}
	data := sys.getData()
	for _, immortalElixirEntry := range data.ImmortalElixirMap {
		config := jsondata.GetImmortalElixirConfig(immortalElixirEntry.Id)
		if config == nil {
			continue
		}
		count := immortalElixirEntry.Count
		var historyEffectCount uint32
		for _, elixirStageUsage := range config.StageUsage {
			if elixirStageUsage.Limit == 0 {
				break
			}
			var effectCount = count - historyEffectCount
			if elixirStageUsage.Limit < count {
				effectCount = elixirStageUsage.Limit - historyEffectCount
			}
			engine.CheckAddAttrsToCalcTimes(player, calc, elixirStageUsage.Attrs, effectCount)
			historyEffectCount += effectCount
		}
	}
	for _, spiritEntry := range data.ImmortalElixirSpiritMap {
		config := jsondata.GetImmortalElixirSpiritConfig(spiritEntry.Id)
		if config == nil {
			continue
		}
		conf := config.GetStarConf(spiritEntry.Star)
		if conf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, conf.Attrs)
	}
}

func getImmortalElixirSys(player iface.IPlayer) *ImmortalElixirSys {
	obj := player.GetSysObj(sysdef.SiImmortalElixir)
	if obj == nil || !obj.IsOpen() {
		return nil
	}
	sys, ok := obj.(*ImmortalElixirSys)
	if !ok {
		return nil
	}
	return sys
}

func init() {
	RegisterSysClass(sysdef.SiImmortalElixir, func() iface.ISystem {
		return &ImmortalElixirSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaImmortalElixir, handleSaImmortalElixir)
	net.RegisterSysProtoV2(10, 71, sysdef.SiImmortalElixir, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalElixirSys).c2sUseElixir
	})
	net.RegisterSysProtoV2(10, 72, sysdef.SiImmortalElixir, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalElixirSys).c2sSpiritUpStar
	})
}
