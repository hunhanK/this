/**
 * @Author: LvYuMeng
 * @Date: 2025/12/22
 * @Desc: 战纹
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type WarPaintSys struct {
	Base
}

var (
	_ iface.IFashionChecker = (*FairyWingFashionSys)(nil)
	_ iface.IFashionChecker = (*GodWeaponSys)(nil)
	_ iface.IFashionChecker = (*BattleShieldTransformSys)(nil)
)

func (s *WarPaintSys) GetSysData() *pb3.WarPaintSysData {
	binary := s.GetBinaryData()

	if nil == binary.WarPaintData {
		binary.WarPaintData = &pb3.WarPaintData{}
	}

	if nil == binary.WarPaintData.SysData {
		binary.WarPaintData.SysData = make(map[uint32]*pb3.WarPaintSysData)
	}

	sysId := s.GetSysId()
	sysData, ok := binary.WarPaintData.SysData[sysId]
	if !ok {
		sysData = &pb3.WarPaintSysData{}
		binary.WarPaintData.SysData[sysId] = sysData
	}

	if nil == sysData.MatrixGraph {
		sysData.MatrixGraph = make(map[uint32]*pb3.WarPaintEquipMatrixGraph)
	}

	return sysData
}

func (s *WarPaintSys) s2cInfo() {
	s.SendProto3(15, 130, &pb3.S2C_15_130{
		SysId: s.GetSysId(),
		Data:  s.GetSysData(),
	})
}

func (s *WarPaintSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *WarPaintSys) OnReconnect() {
	s.s2cInfo()
}

func (s *WarPaintSys) OnOpen() {
	s.s2cInfo()
}

func (s *WarPaintSys) CheckEquipPosTakeOn(pos uint32) bool {
	equip := s.getWarPaintBaseEquip(0)
	if equip == nil || equip.PosMap == nil {
		return false
	}
	_, ok := equip.PosMap[pos]
	if !ok {
		return false
	}
	return true
}

func (s *WarPaintSys) getConf() (*jsondata.WarPaintEquipConfig, error) {
	config := jsondata.GetWarPaintEquipMgr(s.GetSysId())
	if config == nil {
		return nil, neterror.ConfNotFoundError("conf not found")
	}
	return config, nil
}

func (s *WarPaintSys) commonCheckItem(hdl uint64, pos uint32) (*jsondata.ItemConf, *pb3.ItemSt, error) {
	ref, ok := gshare.WarPaintInstance.FindWarPaintRefByWarPaintSysId(s.GetSysId())
	if !ok {
		return nil, nil, neterror.ParamsInvalidError("not found %d ref sys", s.GetSysId())
	}
	owner := s.GetOwner()
	itemSt := owner.GetItemByHandleWithBagType(ref.BagType, hdl)
	if itemSt == nil {
		return nil, nil, neterror.ParamsInvalidError("not found %d item", hdl)
	}
	itemId := itemSt.ItemId
	config := jsondata.GetItemConfig(itemId)
	if config == nil {
		return nil, nil, neterror.ConfNotFoundError("%d not found item conf", itemId)
	}
	if config.SubType != pos {
		return nil, nil, neterror.ParamsInvalidError("%d pos %d not take on %d", itemId, pos, config.SubType)
	}
	if !owner.CheckItemCond(config) {
		return nil, nil, neterror.ParamsInvalidError("%d item cond not reach", itemId)
	}
	if config.SubType == 0 || int(config.SubType) > len(jsondata.GetWarPaintEquipMgr(s.GetSysId()).EquipPos) || !ref.EquipJudge(config.Type) {
		return nil, nil, neterror.ParamsInvalidError("%d %d not take on", config.Type, config.SubType)
	}
	return config, itemSt, nil
}

func (s *WarPaintSys) getWarPaintBaseEquip(idx uint32) *pb3.WarPaintBaseEquip {
	data := s.GetSysData()
	if idx == 0 {
		if data.Default == nil {
			data.Default = &pb3.WarPaintBaseEquip{}
		}
		if data.Default.PosMap == nil {
			data.Default.PosMap = make(map[uint32]*pb3.SimpleWarPaintEquipItem)
		}
		return data.Default
	}
	graph := s.getMatrixGraph(idx)
	if graph == nil {
		return nil
	}
	if graph.Equip == nil {
		graph.Equip = &pb3.WarPaintBaseEquip{}
	}
	if graph.Equip.PosMap == nil {
		graph.Equip.PosMap = make(map[uint32]*pb3.SimpleWarPaintEquipItem)
	}
	return graph.Equip
}

func (s *WarPaintSys) getMatrixGraph(idx uint32) *pb3.WarPaintEquipMatrixGraph {
	data := s.GetSysData()
	matrixGraph := data.MatrixGraph[idx]
	if matrixGraph == nil {
		return nil
	}
	return matrixGraph
}

func (s *WarPaintSys) c2sDefaultTakeOn(req *pb3.C2S_15_131) error {
	ref, ok := gshare.WarPaintInstance.FindWarPaintRefByWarPaintSysId(s.GetSysId())
	if !ok {
		return neterror.ParamsInvalidError("not found %d ref sys", s.GetSysId())
	}
	itemConf, itemSt, err := s.commonCheckItem(req.Hdl, req.Pos)
	if err != nil {
		return neterror.Wrap(err)
	}

	conf, err := s.getConf()
	if err != nil {
		return neterror.Wrap(err)
	}

	posConf, ok := conf.EquipPos[req.Pos]
	if !ok {
		return neterror.ConfNotFoundError("%d not found pos conf", req.Pos)
	}

	if posConf.MaxQuality != 0 && itemConf.Quality > posConf.MaxQuality {
		return neterror.ParamsInvalidError("%d quality %d over max %d", itemConf.Type, itemConf.Quality, posConf.MaxQuality)
	}

	owner := s.GetOwner()

	// 先从背包移除
	if !owner.RemoveItemByHandleWithBagType(ref.BagType, req.Hdl, ref.LogWarPaintEquipTakeOn) {
		return neterror.ParamsInvalidError("%d RemoveWarPaintEquipItemByHandle %d failed", itemConf.Id, req.Hdl)
	}

	equip := s.getWarPaintBaseEquip(0)
	equipItem, ok := equip.PosMap[req.Pos]
	// 先卸下
	if ok {
		toItemSt := equipItem.ToItemSt()
		if !owner.AddItemPtr(toItemSt, false, ref.LogWarPaintEquipTakeOff) {
			mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
				ConfId:    common.Mail_BagInsufficient,
				UserItems: []*pb3.ItemSt{toItemSt},
			})
		}
		logworker.LogPlayerBehavior(s.GetOwner(), ref.LogWarPaintEquipTakeOff, &pb3.LogPlayerCounter{
			NumArgs: uint64(req.Pos),
			StrArgs: fmt.Sprintf("%d", toItemSt.ItemId),
		})
	}

	// 穿上
	equip.PosMap[req.Pos] = itemSt.ToSimpleWarPaintEquipItem()
	s.SendProto3(15, 131, &pb3.S2C_15_131{
		SysId: s.GetSysId(),
		Pos:   req.Pos,
		Item:  equip.PosMap[req.Pos],
	})

	s.afterOpt()
	owner.TriggerEvent(ref.SeOptWarPaintEquip, &pb3.CommonSt{
		U32Param:  0,       // 操作的索引 阵图是大于0的
		U32Param2: req.Pos, // 槽位
		BParam:    true,    // true 穿上 false 卸下
	})
	return nil
}

func (s *WarPaintSys) c2sDefaultTakeOff(req *pb3.C2S_15_132) error {
	ref, ok := gshare.WarPaintInstance.FindWarPaintRefByWarPaintSysId(s.GetSysId())
	if !ok {
		return neterror.ParamsInvalidError("not found %d ref sys", s.GetSysId())
	}
	owner := s.GetOwner()

	count := owner.GetBagAvailableCountByBagType(ref.BagType)
	if count <= 0 {
		return neterror.ParamsInvalidError("bag not AvailableCount")
	}

	equip := s.getWarPaintBaseEquip(0)
	itemSt, ok := equip.PosMap[req.Pos]
	if !ok {
		return neterror.ParamsInvalidError("%d not found pos", req.Pos)
	}

	delete(equip.PosMap, req.Pos)
	toItemSt := itemSt.ToItemSt()
	if !owner.AddItemPtr(toItemSt, false, ref.LogWarPaintEquipTakeOff) {
		mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
			ConfId:    common.Mail_BagInsufficient,
			UserItems: []*pb3.ItemSt{toItemSt},
		})
	}
	s.SendProto3(15, 132, &pb3.S2C_15_132{
		SysId: s.GetSysId(),
		Pos:   req.Pos,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), ref.LogWarPaintEquipTakeOff, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Pos),
		StrArgs: fmt.Sprintf("%d", toItemSt.ItemId),
	})
	s.afterOpt()
	owner.TriggerEvent(ref.SeOptWarPaintEquip, &pb3.CommonSt{
		U32Param:  0,       // 操作的索引 阵图是大于0的
		U32Param2: req.Pos, // 槽位
		BParam:    false,   // true 穿上 false 卸下
	})
	return nil
}

func (s *WarPaintSys) c2sMatrixGraphTokeOn(req *pb3.C2S_15_133) error {
	ref, ok := gshare.WarPaintInstance.FindWarPaintRefByWarPaintSysId(s.GetSysId())
	if !ok {
		return neterror.ParamsInvalidError("not found %d ref sys", s.GetSysId())
	}
	idx := req.Idx
	if idx <= 0 || len(req.PosHdlMap) == 0 {
		return neterror.ParamsInvalidError("idx not zero")
	}

	equip := s.getWarPaintBaseEquip(idx)
	if equip == nil {
		return neterror.ParamsInvalidError("unlock %d data", idx)
	}

	conf := jsondata.GetWarPaintEquipMatrixGraph(s.GetSysId(), idx)
	if conf == nil || conf.EquipPos == nil {
		return neterror.ConfNotFoundError("%d not found conf", idx)
	}

	// 前置校验
	var bagItemMap = make(map[uint64]*pb3.ItemSt)
	for pos, hdl := range req.PosHdlMap {
		itemConf, itemSt, err := s.commonCheckItem(hdl, pos)
		if err != nil {
			return neterror.WrapMsg(err, "pos %d hdl %d has err", pos, hdl)
		}

		posConf, ok := conf.EquipPos[pos]
		if !ok {
			return neterror.ConfNotFoundError("%d not found pos conf", pos)
		}

		if posConf.MaxQuality != 0 && itemConf.Quality > posConf.MaxQuality {
			return neterror.ParamsInvalidError("%d quality %d over max %d", itemConf.Type, itemConf.Quality, posConf.MaxQuality)
		}
		bagItemMap[itemSt.Handle] = itemSt
	}

	owner := s.GetOwner()
	for _, itemSt := range bagItemMap {
		// 先从背包移除
		if !owner.RemoveItemByHandleWithBagType(ref.BagType, itemSt.Handle, ref.LogWarPaintEquipTakeOn) {
			s.LogWarn("%d RemoveWarPaintEquipItemByHandle %d failed", itemSt.ItemId, itemSt.Handle)
		}
	}

	// 把对应槽位有穿的给卸下
	for pos, hdl := range req.PosHdlMap {
		st := bagItemMap[hdl]
		equipItem, ok := equip.PosMap[pos]
		// 先卸下
		if ok {
			toItemSt := equipItem.ToItemSt()
			if !owner.AddItemPtr(toItemSt, false, ref.LogWarPaintEquipTakeOff) {
				mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
					ConfId:    common.Mail_BagInsufficient,
					UserItems: []*pb3.ItemSt{toItemSt},
				})
			}
			logworker.LogPlayerBehavior(s.GetOwner(), ref.LogWarPaintEquipTakeOff, &pb3.LogPlayerCounter{
				NumArgs: uint64(pos),
				StrArgs: fmt.Sprintf("%d", toItemSt.ItemId),
			})
		}
		// 穿上新的
		equip.PosMap[pos] = st.ToSimpleWarPaintEquipItem()
	}
	s.SendProto3(15, 133, &pb3.S2C_15_133{
		SysId: s.GetSysId(),
		Idx:   idx,
		Equip: equip.PosMap,
	})
	s.afterOpt()
	return nil
}

func (s *WarPaintSys) c2sMatrixGraphTokeOff(req *pb3.C2S_15_134) error {
	ref, ok := gshare.WarPaintInstance.FindWarPaintRefByWarPaintSysId(s.GetSysId())
	if !ok {
		return neterror.ParamsInvalidError("not found %d ref sys", s.GetSysId())
	}
	idx := req.Idx
	if idx <= 0 || len(req.PosList) == 0 {
		return neterror.ParamsInvalidError("idx not zero")
	}

	equip := s.getWarPaintBaseEquip(idx)
	if equip == nil {
		return neterror.ParamsInvalidError("unlock %d data", idx)
	}

	conf := jsondata.GetWarPaintEquipMatrixGraph(s.GetSysId(), idx)
	if conf == nil || conf.EquipPos == nil {
		return neterror.ConfNotFoundError("%d not found conf", idx)
	}

	owner := s.GetOwner()
	count := owner.GetBagAvailableCountByBagType(ref.BagType)
	if count <= 0 || count < uint32(len(req.PosList)) {
		return neterror.ParamsInvalidError("bag not AvailableCount")
	}

	var canTakeOffPosList []uint32
	for _, pos := range req.PosList {
		_, ok := equip.PosMap[pos]
		if !ok {
			continue
		}
		canTakeOffPosList = append(canTakeOffPosList, pos)
	}

	if len(canTakeOffPosList) == 0 {
		return neterror.ParamsInvalidError("not can take off pos list")
	}

	for _, pos := range canTakeOffPosList {
		itemSt, ok := equip.PosMap[pos]
		if !ok {
			continue
		}
		delete(equip.PosMap, pos)
		toItemSt := itemSt.ToItemSt()
		if !owner.AddItemPtr(toItemSt, false, ref.LogWarPaintEquipTakeOff) {
			mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
				ConfId:    common.Mail_BagInsufficient,
				UserItems: []*pb3.ItemSt{toItemSt},
			})
		}
	}
	s.SendProto3(15, 134, &pb3.S2C_15_134{
		SysId:   s.GetSysId(),
		Idx:     idx,
		PosList: canTakeOffPosList,
	})
	s.afterOpt()
	return nil
}

func (s *WarPaintSys) c2sTakeOnAppear(req *pb3.C2S_15_135) error {
	ref, ok := gshare.WarPaintInstance.FindWarPaintRefByWarPaintSysId(s.GetSysId())
	if !ok {
		return neterror.ParamsInvalidError("not found %d ref sys", s.GetSysId())
	}
	idx := req.Idx
	if idx <= 0 {
		return neterror.ParamsInvalidError("idx not zero")
	}

	matrixGraph := s.getMatrixGraph(idx)
	if matrixGraph == nil {
		return neterror.ParamsInvalidError("unlock %d data", idx)
	}

	graphConf := jsondata.GetWarPaintEquipMatrixGraph(s.GetSysId(), idx)
	if graphConf == nil {
		return neterror.ConfNotFoundError("%d not found conf", idx)
	}

	owner := s.GetOwner()
	obj := owner.GetSysObj(ref.BindSysId)
	if obj == nil || !obj.IsOpen() {
		return neterror.ParamsInvalidError("riderFashion not open")
	}

	sysCheck, ok := obj.(iface.IFashionChecker)
	if !ok {
		return neterror.ParamsInvalidError("riderFashion not sysCheck")
	}

	if !sysCheck.CheckFashionActive(req.AppearId) {
		return neterror.ParamsInvalidError("riderFashion not %d appear", req.AppearId)
	}

	if sysCheck.GetFashionQuality(req.AppearId) < graphConf.MinAppearQuality {
		return neterror.ParamsInvalidError("riderFashion not %d appear quality %d", req.AppearId, graphConf.MinAppearQuality)
	}

	matrixGraph.AppearId = req.AppearId
	s.SendProto3(15, 135, &pb3.S2C_15_135{
		SysId:    s.GetSysId(),
		Idx:      idx,
		AppearId: matrixGraph.AppearId,
	})
	s.ResetSysAttr(ref.CalAttrWarPaintEqDef)
	return nil
}

func (s *WarPaintSys) afterOpt() {
	ref, ok := gshare.WarPaintInstance.FindWarPaintRefByWarPaintSysId(s.GetSysId())
	if !ok {
		return
	}
	var checkChangeMatrixGraph = func(swordBaseEquip *pb3.WarPaintBaseEquip, matrixGraphIdx uint32) {
		data := s.GetSysData()
		matrixGraphConf := jsondata.GetWarPaintEquipMatrixGraph(s.GetSysId(), matrixGraphIdx)
		if matrixGraphConf == nil {
			return
		}
		if swordBaseEquip == nil {
			return
		}
		// 检查是否可以激活
		var qualityNumMap = make(map[uint32]uint32)
		for _, item := range swordBaseEquip.PosMap {
			config := jsondata.GetItemConfig(item.ItemId)
			if config == nil {
				continue
			}
			qualityNumMap[config.Quality] += 1
		}
		var totalNum uint32
		for q, n := range qualityNumMap {
			if matrixGraphConf.MinQuality > q {
				continue
			}
			totalNum += n
		}
		graph := data.MatrixGraph[matrixGraphIdx]
		// 不满足激活条件
		if matrixGraphConf.Num > totalNum {
			// 曾经激活过 变更状态
			if graph != nil {
				graph.IsLock = true
				s.SendProto3(15, 136, &pb3.S2C_15_136{
					SysId: s.GetSysId(),
					Data:  graph,
				})
			}
			return
		}

		// 判断前置激活条件
		if matrixGraphConf.ParentIdx != 0 {
			parentGraph := data.MatrixGraph[matrixGraphConf.ParentIdx]
			if parentGraph == nil || parentGraph.IsLock {
				// 曾经激活过 变更状态
				if graph != nil {
					graph.IsLock = true
					s.SendProto3(15, 136, &pb3.S2C_15_136{
						SysId: s.GetSysId(),
						Data:  graph,
					})
				}
				return
			}
		}

		// 激活
		if graph == nil {
			data.MatrixGraph[matrixGraphIdx] = &pb3.WarPaintEquipMatrixGraph{}
			graph = data.MatrixGraph[matrixGraphIdx]
		}
		graph.Idx = matrixGraphIdx
		graph.IsLock = false
		s.SendProto3(15, 136, &pb3.S2C_15_136{
			SysId: s.GetSysId(),
			Data:  graph,
		})
	}

	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
	// 暴力点 全量检查阵图的开启
	for _, graph := range conf.MatrixGraph {
		swordBaseEquip := s.getWarPaintBaseEquip(graph.ParentIdx)
		if swordBaseEquip == nil {
			continue
		}
		checkChangeMatrixGraph(swordBaseEquip, graph.Idx)
	}
	s.ResetSysAttr(ref.CalAttrWarPaintEqDef)
}

func (s *WarPaintSys) calAttr(calc *attrcalc.FightAttrCalc) {
	owner := s.GetOwner()
	data := s.GetSysData()
	var checkAddAttr = func(posMap map[uint32]*pb3.SimpleWarPaintEquipItem) {
		if posMap == nil {
			return
		}
		for _, item := range posMap {
			config := jsondata.GetItemConfig(item.ItemId)
			if config == nil {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, config.StaticAttrs)
			engine.CheckAddAttrsToCalc(owner, calc, config.PremiumAttrs)
			engine.CheckAddAttrsToCalc(owner, calc, config.SuperAttrs)
		}
	}

	var checkAddSuitAttr = func(posMap map[uint32]*pb3.SimpleWarPaintEquipItem, suitAttrConf []*jsondata.WarPaintSuitAttrs) {
		if posMap == nil {
			return
		}
		var qualityNumMap = make(map[uint32]uint32)
		for _, item := range posMap {
			config := jsondata.GetItemConfig(item.ItemId)
			if config == nil {
				continue
			}
			qualityNumMap[config.Quality] += 1
		}
		for _, attr := range suitAttrConf {
			var totalNum uint32
			for q, n := range qualityNumMap {
				if attr.MinQuality > q {
					continue
				}
				totalNum += n
			}
			if attr.Num > totalNum {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, attr.Attrs)
		}
	}

	// 默认装备
	if data.Default != nil {
		checkAddAttr(data.Default.PosMap)
		config, _ := s.getConf()
		if config != nil {
			checkAddSuitAttr(data.Default.PosMap, config.SuitAttrs)
		}
	}

	// 阵图装备
	for _, graph := range data.MatrixGraph {
		if graph == nil || graph.Equip == nil {
			continue
		}
		if graph.IsLock {
			continue
		}
		checkAddAttr(graph.Equip.PosMap)
		matrixGraph := jsondata.GetWarPaintEquipMatrixGraph(s.GetSysId(), graph.Idx)
		if matrixGraph == nil {
			continue
		}
		checkAddSuitAttr(graph.Equip.PosMap, matrixGraph.SuitAttrs)
		// 如果上阵了时装
		if graph.AppearId > 0 {
			engine.CheckAddAttrsToCalc(owner, calc, matrixGraph.Attrs)
		}
	}
}

func (s *WarPaintSys) calcAttrAddRate(totalCalc *attrcalc.FightAttrCalc, calc *attrcalc.FightAttrCalc) {
	ref, ok := gshare.WarPaintInstance.FindWarPaintRefByWarPaintSysId(s.GetSysId())
	if !ok {
		return
	}
	owner := s.GetOwner()
	obj := owner.GetSysObj(ref.BindSysId)
	if obj == nil || !obj.IsOpen() {
		return
	}
	checker, ok := obj.(iface.IFashionChecker)
	if !ok {
		return
	}
	data := s.GetSysData()
	baseAddRate := totalCalc.GetValue(ref.WarPaintEquipBaseAddRate)
	qualityAddRate := totalCalc.GetValue(ref.WarPaintEquipQualityAddRate)
	var checkAddAttr = func(posMap map[uint32]*pb3.SimpleWarPaintEquipItem) {
		if posMap == nil {
			return
		}
		for pos, item := range posMap {
			config := jsondata.GetItemConfig(item.ItemId)
			if config == nil {
				continue
			}
			refineConfig := jsondata.GetWarPaintEquipRefineConfig(s.GetSysId(), pos)
			var refineAddRate int64
			if refineConfig != nil {
				refineAddRate = totalCalc.GetValue(refineConfig.AttrId)
			}
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, config.StaticAttrs, uint32(baseAddRate)+uint32(refineAddRate))
			engine.CheckAddAttrsRateRoundingUp(owner, calc, config.SuperAttrs, uint32(qualityAddRate))
		}
	}

	// 默认装备
	if data.Default != nil {
		checkAddAttr(data.Default.PosMap)
	}

	// 阵图装备
	for _, graph := range data.MatrixGraph {
		if graph == nil || graph.Equip == nil {
			continue
		}
		if graph.IsLock {
			continue
		}
		checkAddAttr(graph.Equip.PosMap)
		matrixGraph := jsondata.GetWarPaintEquipMatrixGraph(s.GetSysId(), graph.Idx)
		// 上阵时装
		if matrixGraph != nil && graph.AppearId > 0 && len(matrixGraph.AddRateAttr) > 0 {
			attrVec := checker.GetFashionBaseAttr(graph.AppearId)
			engine.CheckAddAttrsRateRoundingUp(owner, calc, attrVec, matrixGraph.AddRateAttr[0].Value)
		}
	}
}

func GetWarPaintSys(player iface.IPlayer, sysId uint32) (*WarPaintSys, error) {
	sys := player.GetSysObj(sysId).(*WarPaintSys)
	if sys == nil || !sys.IsOpen() {
		return nil, neterror.SysNotExistError("WarPaintSys %d err is nil", sysId)
	}
	return sys, nil
}

func warPaintSysDo(player iface.IPlayer, sysId uint32, fn func(sys *WarPaintSys)) {
	if sys, err := GetWarPaintSys(player, sysId); err == nil && sys.IsOpen() {
		fn(sys)
	}
	return
}

func regWarPaintSys() {
	gshare.WarPaintInstance.EachWarPaintRefDo(func(ref *gshare.WarPaintRef) {
		RegisterSysClass(ref.WarPaintSysId, func() iface.ISystem {
			return &WarPaintSys{}
		})

		engine.RegAttrCalcFn(ref.CalAttrWarPaintEqDef, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
			warPaintSysDo(player, ref.WarPaintSysId, func(sys *WarPaintSys) {
				sys.calAttr(calc)
			})
		})

		engine.RegAttrAddRateCalcFn(ref.CalAttrWarPaintEqDef, func(player iface.IPlayer, totalCalc *attrcalc.FightAttrCalc, calc *attrcalc.FightAttrCalc) {
			warPaintSysDo(player, ref.WarPaintSysId, func(sys *WarPaintSys) {
				sys.calcAttrAddRate(totalCalc, calc)
			})
		})
	})

	net.RegisterProto(15, 131, func(player iface.IPlayer, msg *base.Message) error {
		var req pb3.C2S_15_131
		err := msg.UnPackPb3Msg(&req)
		if err != nil {
			return err
		}
		sys, err := GetWarPaintSys(player, req.GetSysId())
		if err != nil {
			return err
		}
		return sys.c2sDefaultTakeOn(&req)
	})

	net.RegisterProto(15, 132, func(player iface.IPlayer, msg *base.Message) error {
		var req pb3.C2S_15_132
		err := msg.UnPackPb3Msg(&req)
		if err != nil {
			return err
		}
		sys, err := GetWarPaintSys(player, req.GetSysId())
		if err != nil {
			return err
		}
		return sys.c2sDefaultTakeOff(&req)
	})

	net.RegisterProto(15, 133, func(player iface.IPlayer, msg *base.Message) error {
		var req pb3.C2S_15_133
		err := msg.UnPackPb3Msg(&req)
		if err != nil {
			return err
		}
		sys, err := GetWarPaintSys(player, req.GetSysId())
		if err != nil {
			return err
		}
		return sys.c2sMatrixGraphTokeOn(&req)
	})

	net.RegisterProto(15, 134, func(player iface.IPlayer, msg *base.Message) error {
		var req pb3.C2S_15_134
		err := msg.UnPackPb3Msg(&req)
		if err != nil {
			return err
		}
		sys, err := GetWarPaintSys(player, req.GetSysId())
		if err != nil {
			return err
		}
		return sys.c2sMatrixGraphTokeOff(&req)
	})

	net.RegisterProto(15, 135, func(player iface.IPlayer, msg *base.Message) error {
		var req pb3.C2S_15_135
		err := msg.UnPackPb3Msg(&req)
		if err != nil {
			return err
		}
		sys, err := GetWarPaintSys(player, req.GetSysId())
		if err != nil {
			return err
		}
		return sys.c2sTakeOnAppear(&req)
	})
}

func init() {
	regWarPaintSys()
}
