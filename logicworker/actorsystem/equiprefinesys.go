/**
 * @Author: zjj
 * @Date: 2024/10/30
 * @Desc: 装备洗炼
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
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
)

type EquipRefineSys struct {
	Base
}

func (s *EquipRefineSys) s2cInfo() {
	s.SendProto3(11, 120, &pb3.S2C_11_120{
		Data: s.getData(),
	})
}

func (s *EquipRefineSys) getData() *pb3.EquipRefineData {
	data := s.GetBinaryData().EquipRefineData
	if data == nil {
		s.GetBinaryData().EquipRefineData = &pb3.EquipRefineData{}
		data = s.GetBinaryData().EquipRefineData
	}
	if data.GridMap == nil {
		data.GridMap = make(map[uint32]*pb3.EquipRefineGrid)
	}
	return data
}

func (s *EquipRefineSys) OnReconnect() {
	s.s2cInfo()
}

func (s *EquipRefineSys) OnLogin() {
	s.s2cInfo()
}

func (s *EquipRefineSys) OnOpen() {
	s.ResetSysAttr(attrdef.SaEquipRefine)
	s.s2cInfo()
}

func (s *EquipRefineSys) c2sActiveFreeGridPos(msg *base.Message) error {
	var req pb3.C2S_11_121
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	grid, err := s.unLock(req.Grid, 0, pb3.LogId_LogEquipRefineActiveFreePos)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}
	s.ResetSysAttr(attrdef.SaEquipRefine)
	s.SendProto3(11, 121, &pb3.S2C_11_121{
		Grid: grid,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogEquipRefineActiveFreePos, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Grid),
	})
	return nil
}

func (s *EquipRefineSys) c2sUnlockPos(msg *base.Message) error {
	var req pb3.C2S_11_122
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	grid, err := s.unLock(req.Grid, req.Pos, pb3.LogId_LogEquipRefineUnlockPos)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}
	s.ResetSysAttr(attrdef.SaEquipRefine)
	s.SendProto3(11, 122, &pb3.S2C_11_122{
		Grid: grid,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogEquipRefineUnlockPos, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Grid),
		StrArgs: fmt.Sprintf("%d", req.Pos),
	})
	return nil
}

func (s *EquipRefineSys) c2sRefine(msg *base.Message) error {
	var req pb3.C2S_11_123
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	grid := req.Grid
	gridConf := jsondata.GetEquipRefineConf(grid)
	if gridConf == nil {
		return neterror.ConfNotFoundError("%d not found grid conf", grid)
	}

	gridData := s.getGrid(grid)
	if len(gridData.PosMap) == 0 {
		return neterror.ParamsInvalidError("%d not lock pos", grid)
	}

	var consumeList jsondata.ConsumeVec

	// 锁定槽位
	var lockPosList []uint32
	for _, pos := range gridData.PosMap {
		if !pos.Lock {
			continue
		}
		lockPosList = append(lockPosList, pos.Pos)
	}
	lockPoss := pie.Uint32s(lockPosList)
	lockTypeSize := lockPoss.Len()
	if lockTypeSize != 0 {
		var lockConsumeConf *jsondata.EquipRefineLockConsumeConf
		for _, lockAttrConf := range gridConf.LockConsume {
			if lockAttrConf.Number != uint32(lockTypeSize) {
				continue
			}
			lockConsumeConf = lockAttrConf
			break
		}
		if lockConsumeConf == nil {
			return neterror.ConfNotFoundError("%d %d not found lock type consume", grid, lockTypeSize)
		}
		consumeList = append(consumeList, lockConsumeConf.Consume...)
	} else {
		// 洗炼消耗
		consumeList = append(consumeList, gridConf.RefreshConsume...)
	}

	// 保底
	useMinQuality := req.UseMinQuality
	if useMinQuality != 0 {
		var consumeConf *jsondata.EquipRefineMinQualityConsumeConf
		for _, c := range gridConf.MinQualityConsume {
			if c.Quality != useMinQuality {
				continue
			}
			consumeConf = c
			break
		}
		consumeList = append(consumeList, consumeConf.Consume...)
	}

	owner := s.GetOwner()
	if len(consumeList) != 0 && !owner.ConsumeByConf(consumeList, false, common.ConsumeParams{LogId: pb3.LogId_LogEquipRefine}) {
		return neterror.ConfNotFoundError("%d refine failed", grid)
	}

	minQuality := useMinQuality
	var skipAttr = make(map[uint32]struct{})
	for _, poss := range lockPoss {
		skipAttr[gridData.PosMap[poss].Attr.Type] = struct{}{}
	}

	for _, posData := range gridData.PosMap {
		if lockPoss.Contains(posData.Pos) {
			posData.Lock = true
			continue
		}
		posData.Lock = false
		err := s.refine(gridData, posData, minQuality, skipAttr)
		if err != nil {
			owner.LogError("err:%v", err)
			continue
		}
		skipAttr[posData.Attr.Type] = struct{}{}
		minQuality = 0
	}

	var qualityMap = make(map[uint32]uint32)
	for _, conf := range gridConf.ExtAttr {
		for _, pos := range gridData.PosMap {
			if pos.Attr.Quality < conf.MinQuality {
				continue
			}
			qualityMap[conf.MinQuality] += 1
		}
	}

	var extIdx uint32
	for _, conf := range gridConf.ExtAttr {
		if count, ok := qualityMap[conf.MinQuality]; ok {
			if count < conf.Num {
				continue
			}
			extIdx = conf.Idx
		}
	}
	gridData.ExtAttrIdx = extIdx
	gridData.RefineTimes += 1
	s.ResetSysAttr(attrdef.SaEquipRefine)
	s.SendProto3(11, 123, &pb3.S2C_11_123{
		Grid: gridData,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogEquipRefine, &pb3.LogPlayerCounter{
		NumArgs: uint64(grid),
		StrArgs: fmt.Sprintf("%d_%d", extIdx, minQuality),
	})
	return nil
}

// 洗炼
func (s *EquipRefineSys) refine(gridData *pb3.EquipRefineGrid, posData *pb3.EquipRefinePos, useMinQuality uint32, skipAttr map[uint32]struct{}) error {
	grid := gridData.Grid
	equipRefineConf := jsondata.GetEquipRefineConf(grid)
	if equipRefineConf == nil {
		return neterror.ConfNotFoundError("%d not found conf", grid)
	}

	if skipAttr == nil {
		skipAttr = make(map[uint32]struct{})
	}

	var equipRefinePosConf *jsondata.EquipRefineRefinePosConf
	for _, posConf := range equipRefineConf.RefinePos {
		if posConf.Pos != posData.Pos {
			continue
		}
		equipRefinePosConf = posConf
		break
	}
	if equipRefinePosConf == nil {
		return neterror.ConfNotFoundError("%d %d not found conf", grid, posData.Pos)
	}

	var randOneAttrPool = func(attrPool []*jsondata.EquipRefineAttrPoolConf) *jsondata.EquipRefineAttrPoolConf {
		var randPool = new(random.Pool)
		for _, pool := range attrPool {
			if _, ok := skipAttr[pool.Type]; ok {
				continue
			}
			randPool.AddItem(pool, pool.Weight)
		}
		return randPool.RandomOne().(*jsondata.EquipRefineAttrPoolConf)
	}

	// 随机属性
	var randOnePosAttr = func(attrPool *jsondata.EquipRefineAttrPoolConf) *pb3.EquipRefineAttrSt {
		var randPool = new(random.Pool)
		for _, refineAttrQualityConf := range attrPool.AttrQuality {
			if useMinQuality != 0 && useMinQuality > refineAttrQualityConf.Quality {
				continue
			}
			weight := refineAttrQualityConf.Weight + utils.MinUInt32(gridData.RefineTimes*refineAttrQualityConf.AddWeight, refineAttrQualityConf.WeightLimit)
			randPool.AddItem(refineAttrQualityConf, weight)
		}
		if randPool.Size() == 0 {
			return nil
		}
		qualityConf := randPool.RandomOne().(*jsondata.EquipRefineAttrQualityConf)
		uu := random.IntervalUU(qualityConf.Min, qualityConf.Max)
		return &pb3.EquipRefineAttrSt{
			Type:    attrPool.Type,
			Value:   uu,
			Quality: qualityConf.Quality,
		}
	}

	var oneAttrPool = randOneAttrPool(equipRefineConf.AttrPool)

	randomAttrSt := randOnePosAttr(oneAttrPool)
	if randomAttrSt == nil {
		return neterror.InternalError("%d %d random one attr failed", grid, posData.Pos)
	}
	posData.Attr = randomAttrSt
	return nil
}

func (s *EquipRefineSys) getGrid(grid uint32) *pb3.EquipRefineGrid {
	data := s.getData()
	refineGrid, ok := data.GridMap[grid]
	if !ok {
		data.GridMap[grid] = &pb3.EquipRefineGrid{
			Grid: grid,
		}
		refineGrid = data.GridMap[grid]
	}
	if refineGrid.PosMap == nil {
		refineGrid.PosMap = make(map[uint32]*pb3.EquipRefinePos)
	}
	return refineGrid
}

func (s *EquipRefineSys) checkGridCond(grid uint32) bool {
	conf := jsondata.GetEquipRefineConf(grid)
	if conf == nil {
		return false
	}
	if conf.OpenCond == nil {
		return false
	}
	cond := conf.OpenCond
	owner := s.GetOwner()
	if cond.Pos != 0 {
		equips := owner.GetMainData().ItemPool.Equips
		var posItemId uint32
		for _, equip := range equips {
			if equip.Pos != cond.Pos {
				continue
			}
			posItemId = equip.ItemId
			break
		}
		if posItemId == 0 {
			return false
		}
		itemConf := jsondata.GetItemConfig(posItemId)
		if itemConf == nil {
			return false
		}
		if cond.Quality != 0 && cond.Quality > itemConf.Quality {
			return false
		}
		if cond.Stage != 0 && cond.Stage > itemConf.Stage {
			return false
		}
		if cond.Star != 0 && cond.Star > itemConf.Star {
			return false
		}
	}
	if cond.ActorLv != 0 && cond.ActorLv > owner.GetLevel() {
		return false
	}
	return true
}

func (s *EquipRefineSys) unLock(grid uint32, pos uint32, logId pb3.LogId) (*pb3.EquipRefineGrid, error) {
	if !s.checkGridCond(grid) {
		return nil, neterror.ParamsInvalidError("grid %d not reach open cand", grid)
	}
	owner := s.GetOwner()
	gridData := s.getGrid(grid)
	gridConf := jsondata.GetEquipRefineConf(grid)
	if gridConf == nil {
		return nil, neterror.ConfNotFoundError("%d not found grid conf", grid)
	}

	if pos != 0 && gridData.PosMap[pos] != nil {
		return nil, neterror.ParamsInvalidError("%d %d already unlock", grid, pos)
	}

	var unLockConfList []*jsondata.EquipRefineRefinePosConf
	for _, posConf := range gridConf.RefinePos {
		if pos == 0 && len(posConf.Consume) == 0 {
			unLockConfList = append(unLockConfList, posConf)
			continue
		}
		if pos != 0 && pos == posConf.Pos {
			unLockConfList = append(unLockConfList, posConf)
		}
	}

	var skipSet = make(map[uint32]struct{})
	for _, posData := range gridData.PosMap {
		skipSet[posData.Attr.Type] = struct{}{}
	}

	for _, posConf := range unLockConfList {
		if gridData.PosMap[posConf.Pos] != nil {
			continue
		}
		if len(posConf.Consume) > 0 && !owner.ConsumeByConf(posConf.Consume, false, common.ConsumeParams{LogId: logId}) {
			owner.LogError("%d %d consume failed", grid, pos)
			continue
		}
		posData := &pb3.EquipRefinePos{Pos: posConf.Pos}
		err := s.refine(gridData, posData, 0, skipSet)
		if err != nil {
			owner.LogError("err:%v", err)
			continue
		}
		gridData.PosMap[posConf.Pos] = posData
		skipSet[posData.Attr.Type] = struct{}{}
	}

	return gridData, nil
}

func (s *EquipRefineSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	for _, grid := range data.GridMap {
		getEquipRefineConf := jsondata.GetEquipRefineConf(grid.Grid)
		if getEquipRefineConf == nil {
			continue
		}
		if grid.ExtAttrIdx > 0 {
			for _, conf := range getEquipRefineConf.ExtAttr {
				if conf.Idx != grid.ExtAttrIdx {
					continue
				}
				engine.AddAttrsToCalc(s.GetOwner(), calc, conf.Attrs)
			}
		}
		for _, pos := range grid.PosMap {
			if pos.Attr == nil {
				continue
			}
			calc.AddValue(pos.Attr.Type, attrdef.AttrValueAlias(pos.Attr.Value))
		}
	}
}

func (s *EquipRefineSys) c2sLock(msg *base.Message) error {
	var req pb3.C2S_11_124
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	s.getGrid(req.Grid)
	grid := s.getData().GridMap[req.Grid]
	if grid == nil {
		return neterror.ParamsInvalidError("not found %d grid data", req.Grid)
	}
	pos := grid.PosMap[req.Pos]
	if pos == nil {
		return neterror.ParamsInvalidError("not found %d grid %d pos data", req.Grid, req.Pos)
	}
	pos.Lock = !pos.Lock
	s.SendProto3(11, 124, &pb3.S2C_11_124{
		Grid: req.Grid,
		Pos:  req.Pos,
		Lock: pos.Lock,
	})
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogEquipRefineLockPos, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Grid),
		StrArgs: fmt.Sprintf("%d_%v", req.Pos, pos.Lock),
	})
	return nil
}

func calcSaEquipRefineAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiEquipRefine)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*EquipRefineSys)
	if !ok {
		return
	}
	sys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiEquipRefine, func() iface.ISystem {
		return &EquipRefineSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaEquipRefine, calcSaEquipRefineAttr)
	net.RegisterSysProtoV2(11, 121, sysdef.SiEquipRefine, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EquipRefineSys).c2sActiveFreeGridPos
	})
	net.RegisterSysProtoV2(11, 122, sysdef.SiEquipRefine, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EquipRefineSys).c2sUnlockPos
	})
	net.RegisterSysProtoV2(11, 123, sysdef.SiEquipRefine, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EquipRefineSys).c2sRefine
	})
	net.RegisterSysProtoV2(11, 124, sysdef.SiEquipRefine, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EquipRefineSys).c2sLock
	})
}
