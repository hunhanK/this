/**
 * @Author:
 * @Date:
 * @Desc: 飞剑玉符
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type FlyingSwordEquipSys struct {
	Base
}

func (s *FlyingSwordEquipSys) s2cInfo() {
	s.SendProto3(8, 130, &pb3.S2C_8_130{
		Data: s.getData(),
	})
}

func (s *FlyingSwordEquipSys) getData() *pb3.FlyingSwordEquipData {
	data := s.GetBinaryData().FlyingSwordEquipData
	if data == nil {
		s.GetBinaryData().FlyingSwordEquipData = &pb3.FlyingSwordEquipData{}
		data = s.GetBinaryData().FlyingSwordEquipData
	}
	if data.MatrixGraph == nil {
		data.MatrixGraph = make(map[uint32]*pb3.FlyingSwordEquipMatrixGraph)
	}
	return data
}

func (s *FlyingSwordEquipSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FlyingSwordEquipSys) OnLogin() {
	s.s2cInfo()
}

func (s *FlyingSwordEquipSys) OnOpen() {
	s.s2cInfo()
}

func (s *FlyingSwordEquipSys) CheckEquipPosTakeOn(pos uint32) bool {
	equip := s.getFlyingSwordBaseEquip(0)
	if equip == nil || equip.PosMap == nil {
		return false
	}
	_, ok := equip.PosMap[pos]
	if !ok {
		return false
	}
	return true
}

func (s *FlyingSwordEquipSys) getConf() (*jsondata.FlyingSwordEquipConfig, error) {
	config := jsondata.GetFlyingSwordEquipConfig()
	if config == nil {
		return nil, neterror.ConfNotFoundError("conf not found")
	}
	return config, nil
}

func (s *FlyingSwordEquipSys) commonCheckItem(hdl uint64, pos uint32) (*jsondata.ItemConf, *pb3.ItemSt, error) {
	owner := s.GetOwner()
	itemSt := owner.GetFlyingSwordEquipItemByHandle(hdl)
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
	if !itemdef.IsFlyingSwordEquipItem(config.Type) || !itemdef.IsIstFlyingSwordEquipPos(config.SubType) {
		return nil, nil, neterror.ParamsInvalidError("%d %d not take on", config.Type, config.SubType)
	}
	return config, itemSt, nil
}

func (s *FlyingSwordEquipSys) getFlyingSwordBaseEquip(idx uint32) *pb3.FlyingSwordBaseEquip {
	data := s.getData()
	if idx == 0 {
		if data.Default == nil {
			data.Default = &pb3.FlyingSwordBaseEquip{}
		}
		if data.Default.PosMap == nil {
			data.Default.PosMap = make(map[uint32]*pb3.SimpleFlyingSwordEquipItem)
		}
		return data.Default
	}
	graph := s.getMatrixGraph(idx)
	if graph == nil {
		return nil
	}
	if graph.Equip == nil {
		graph.Equip = &pb3.FlyingSwordBaseEquip{}
	}
	if graph.Equip.PosMap == nil {
		graph.Equip.PosMap = make(map[uint32]*pb3.SimpleFlyingSwordEquipItem)
	}
	return graph.Equip
}

func (s *FlyingSwordEquipSys) getMatrixGraph(idx uint32) *pb3.FlyingSwordEquipMatrixGraph {
	data := s.getData()
	matrixGraph := data.MatrixGraph[idx]
	if matrixGraph == nil {
		return nil
	}
	return matrixGraph
}

func (s *FlyingSwordEquipSys) c2sDefaultTakeOn(msg *base.Message) error {
	var req pb3.C2S_8_131
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
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
	if !owner.RemoveFlyingSwordEquipItemByHandle(req.Hdl, pb3.LogId_LogFlyingSwordEquipTakeOn) {
		return neterror.ParamsInvalidError("%d RemoveFlyingSwordEquipItemByHandle %d failed", itemConf.Id, req.Hdl)
	}

	equip := s.getFlyingSwordBaseEquip(0)
	equipItem, ok := equip.PosMap[req.Pos]
	// 先卸下
	if ok {
		toItemSt := equipItem.ToItemSt()
		if !owner.AddItemPtr(toItemSt, false, pb3.LogId_LogFlyingSwordEquipTakeOff) {
			mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
				ConfId:    common.Mail_BagInsufficient,
				UserItems: []*pb3.ItemSt{toItemSt},
				Content: &mailargs.CommonMailArgs{
					Str1: owner.GetName(),
					Str2: itemConf.Name,
				},
			})
		}
		logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFlyingSwordEquipTakeOff, &pb3.LogPlayerCounter{
			NumArgs: uint64(req.Pos),
			StrArgs: fmt.Sprintf("%d", toItemSt.ItemId),
		})
	}

	// 穿上
	equip.PosMap[req.Pos] = itemSt.ToSimpleFlyingSwordEquipItem()
	s.SendProto3(8, 131, &pb3.S2C_8_131{
		Pos:  req.Pos,
		Item: equip.PosMap[req.Pos],
	})

	s.afterOpt()
	owner.TriggerEvent(custom_id.SeOptFlyingSwordEquip, &pb3.CommonSt{
		U32Param:  0,       // 操作的索引 阵图是大于0的
		U32Param2: req.Pos, // 槽位
		BParam:    true,    // true 穿上 false 卸下
	})
	owner.TriggerQuestEventRange(custom_id.QttTakeOnDefaultFlyingSwordEquip)
	return nil
}

func (s *FlyingSwordEquipSys) c2sDefaultTakeOff(msg *base.Message) error {
	var req pb3.C2S_8_132
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	owner := s.GetOwner()

	count := owner.GetFlyingSwordEquipBagAvailableCount()
	if count <= 0 {
		return neterror.ParamsInvalidError("bag not AvailableCount")
	}

	equip := s.getFlyingSwordBaseEquip(0)
	itemSt, ok := equip.PosMap[req.Pos]
	if !ok {
		return neterror.ParamsInvalidError("%d not found pos", req.Pos)
	}

	delete(equip.PosMap, req.Pos)
	toItemSt := itemSt.ToItemSt()
	if !owner.AddItemPtr(toItemSt, false, pb3.LogId_LogFlyingSwordEquipTakeOff) {
		conf := jsondata.GetItemConfig(toItemSt.ItemId)
		var confName = "玉符"
		if conf != nil {
			confName = conf.Name
		}
		mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
			ConfId:    common.Mail_BagInsufficient,
			UserItems: []*pb3.ItemSt{toItemSt},
			Content: &mailargs.CommonMailArgs{
				Str1: owner.GetName(),
				Str2: confName,
			},
		})
	}
	s.SendProto3(8, 132, &pb3.S2C_8_132{
		Pos: req.Pos,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFlyingSwordEquipTakeOff, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Pos),
		StrArgs: fmt.Sprintf("%d", toItemSt.ItemId),
	})
	s.afterOpt()
	owner.TriggerEvent(custom_id.SeOptFlyingSwordEquip, &pb3.CommonSt{
		U32Param:  0,       // 操作的索引 阵图是大于0的
		U32Param2: req.Pos, // 槽位
		BParam:    false,   // true 穿上 false 卸下
	})
	owner.TriggerQuestEventRange(custom_id.QttTakeOnDefaultFlyingSwordEquip)
	return nil
}
func (s *FlyingSwordEquipSys) c2sMatrixGraphTokeOn(msg *base.Message) error {
	var req pb3.C2S_8_133
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	idx := req.Idx
	if idx <= 0 || len(req.PosHdlMap) == 0 {
		return neterror.ParamsInvalidError("idx not zero")
	}

	equip := s.getFlyingSwordBaseEquip(idx)
	if equip == nil {
		return neterror.ParamsInvalidError("unlock %d data", idx)
	}

	conf := jsondata.GetFlyingSwordEquipMatrixGraph(idx)
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
		if !owner.RemoveFlyingSwordEquipItemByHandle(itemSt.Handle, pb3.LogId_LogFlyingSwordEquipTakeOn) {
			s.LogWarn("%d RemoveFlyingSwordEquipItemByHandle %d failed", itemSt.ItemId, itemSt.Handle)
		}
	}

	// 把对应槽位有穿的给卸下
	for pos, hdl := range req.PosHdlMap {
		st := bagItemMap[hdl]
		equipItem, ok := equip.PosMap[pos]
		// 先卸下
		if ok {
			toItemSt := equipItem.ToItemSt()
			if !owner.AddItemPtr(toItemSt, false, pb3.LogId_LogFlyingSwordEquipTakeOff) {
				conf := jsondata.GetItemConfig(toItemSt.ItemId)
				var confName = "玉符"
				if conf != nil {
					confName = conf.Name
				}
				mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
					ConfId:    common.Mail_BagInsufficient,
					UserItems: []*pb3.ItemSt{toItemSt},
					Content: &mailargs.CommonMailArgs{
						Str1: owner.GetName(),
						Str2: confName,
					},
				})
			}
			logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFlyingSwordEquipTakeOff, &pb3.LogPlayerCounter{
				NumArgs: uint64(pos),
				StrArgs: fmt.Sprintf("%d", toItemSt.ItemId),
			})
		}
		// 穿上新的
		equip.PosMap[pos] = st.ToSimpleFlyingSwordEquipItem()
	}
	s.SendProto3(8, 133, &pb3.S2C_8_133{
		Idx:   idx,
		Equip: equip.PosMap,
	})
	s.afterOpt()
	return nil
}
func (s *FlyingSwordEquipSys) c2sMatrixGraphTokeOff(msg *base.Message) error {
	var req pb3.C2S_8_134
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	idx := req.Idx
	if idx <= 0 || len(req.PosList) == 0 {
		return neterror.ParamsInvalidError("idx not zero")
	}

	equip := s.getFlyingSwordBaseEquip(idx)
	if equip == nil {
		return neterror.ParamsInvalidError("unlock %d data", idx)
	}

	conf := jsondata.GetFlyingSwordEquipMatrixGraph(idx)
	if conf == nil || conf.EquipPos == nil {
		return neterror.ConfNotFoundError("%d not found conf", idx)
	}

	owner := s.GetOwner()
	count := owner.GetFlyingSwordEquipBagAvailableCount()
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
		if !owner.AddItemPtr(toItemSt, false, pb3.LogId_LogFlyingSwordEquipTakeOff) {
			conf := jsondata.GetItemConfig(toItemSt.ItemId)
			var confName = "玉符"
			if conf != nil {
				confName = conf.Name
			}
			mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
				ConfId:    common.Mail_BagInsufficient,
				UserItems: []*pb3.ItemSt{toItemSt},
				Content: &mailargs.CommonMailArgs{
					Str1: owner.GetName(),
					Str2: confName,
				},
			})
		}
	}
	s.SendProto3(8, 134, &pb3.S2C_8_134{
		Idx:     idx,
		PosList: canTakeOffPosList,
	})
	s.afterOpt()
	return nil
}
func (s *FlyingSwordEquipSys) c2sTakeOnAppear(msg *base.Message) error {
	var req pb3.C2S_8_135
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	idx := req.Idx
	if idx <= 0 {
		return neterror.ParamsInvalidError("idx not zero")
	}

	matrixGraph := s.getMatrixGraph(idx)
	if matrixGraph == nil {
		return neterror.ParamsInvalidError("unlock %d data", idx)
	}

	graphConf := jsondata.GetFlyingSwordEquipMatrixGraph(idx)
	if graphConf == nil {
		return neterror.ConfNotFoundError("%d not found conf", idx)
	}

	owner := s.GetOwner()
	obj := owner.GetSysObj(sysdef.SiRiderFashion)
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
	s.SendProto3(8, 135, &pb3.S2C_8_135{
		Idx:      idx,
		AppearId: matrixGraph.AppearId,
	})
	s.ResetSysAttr(attrdef.SaFlyingSwordEquip)
	return nil
}

func (s *FlyingSwordEquipSys) afterOpt() {
	var checkChangeMatrixGraph = func(swordBaseEquip *pb3.FlyingSwordBaseEquip, matrixGraphIdx uint32) {
		data := s.getData()
		matrixGraphConf := jsondata.GetFlyingSwordEquipMatrixGraph(matrixGraphIdx)
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
				s.SendProto3(8, 136, &pb3.S2C_8_136{
					Data: graph,
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
					s.SendProto3(8, 136, &pb3.S2C_8_136{
						Data: graph,
					})
				}
				return
			}
		}

		// 激活
		if graph == nil {
			data.MatrixGraph[matrixGraphIdx] = &pb3.FlyingSwordEquipMatrixGraph{}
			graph = data.MatrixGraph[matrixGraphIdx]
		}
		graph.Idx = matrixGraphIdx
		graph.IsLock = false
		s.SendProto3(8, 136, &pb3.S2C_8_136{
			Data: graph,
		})
	}

	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
	// 暴力点 全量检查阵图的开启
	for _, graph := range conf.MatrixGraph {
		swordBaseEquip := s.getFlyingSwordBaseEquip(graph.ParentIdx)
		if swordBaseEquip == nil {
			continue
		}
		checkChangeMatrixGraph(swordBaseEquip, graph.Idx)
	}
	s.ResetSysAttr(attrdef.SaFlyingSwordEquip)
}

func (s *FlyingSwordEquipSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	owner := s.GetOwner()
	data := s.getData()
	var checkAddAttr = func(posMap map[uint32]*pb3.SimpleFlyingSwordEquipItem) {
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

	var checkAddSuitAttr = func(posMap map[uint32]*pb3.SimpleFlyingSwordEquipItem, suitAttrConf []*jsondata.FlyingSwordSuitAttrs) {
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
		matrixGraph := jsondata.GetFlyingSwordEquipMatrixGraph(graph.Idx)
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

func (s *FlyingSwordEquipSys) calcAttrAddRate(totalCalc *attrcalc.FightAttrCalc, calc *attrcalc.FightAttrCalc) {
	owner := s.GetOwner()
	obj := owner.GetSysObj(sysdef.SiRiderFashion)
	if obj == nil || !obj.IsOpen() {
		return
	}
	checker, ok := obj.(iface.IFashionChecker)
	if !ok {
		return
	}

	data := s.getData()
	baseAddRate := totalCalc.GetValue(attrdef.FlyingSwordEquipBaseAddRate)
	qualityAddRate := totalCalc.GetValue(attrdef.FlyingSwordEquipQualityAddRate)
	var checkAddAttr = func(posMap map[uint32]*pb3.SimpleFlyingSwordEquipItem) {
		if posMap == nil {
			return
		}
		for pos, item := range posMap {
			config := jsondata.GetItemConfig(item.ItemId)
			if config == nil {
				continue
			}
			refineConfig := jsondata.GetFlyingSwordEquipRefineConfig(pos)
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
		matrixGraph := jsondata.GetFlyingSwordEquipMatrixGraph(graph.Idx)
		// 上阵时装
		if matrixGraph != nil && graph.AppearId > 0 && len(matrixGraph.AddRateAttr) > 0 {
			attrVec := checker.GetFashionBaseAttr(graph.AppearId)
			engine.CheckAddAttrsRateRoundingUp(owner, calc, attrVec, matrixGraph.AddRateAttr[0].Value)
		}
	}
}

func flyingSwordEquipProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiFlyingSwordEquip)
	if obj == nil || !obj.IsOpen() {
		return
	}

	sys := obj.(*FlyingSwordEquipSys)
	if sys == nil {
		return
	}

	sys.calcAttr(calc)
}

func flyingSwordEquipPropertyAddRate(player iface.IPlayer, totalCalc *attrcalc.FightAttrCalc, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiFlyingSwordEquip)
	if obj == nil || !obj.IsOpen() {
		return
	}

	sys := obj.(*FlyingSwordEquipSys)
	if sys == nil {
		return
	}

	sys.calcAttrAddRate(totalCalc, calc)
}

func init() {
	RegisterSysClass(sysdef.SiFlyingSwordEquip, func() iface.ISystem {
		return &FlyingSwordEquipSys{}
	})

	net.RegisterSysProtoV2(8, 131, sysdef.SiFlyingSwordEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyingSwordEquipSys).c2sDefaultTakeOn
	})
	net.RegisterSysProtoV2(8, 132, sysdef.SiFlyingSwordEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyingSwordEquipSys).c2sDefaultTakeOff
	})
	net.RegisterSysProtoV2(8, 133, sysdef.SiFlyingSwordEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyingSwordEquipSys).c2sMatrixGraphTokeOn
	})
	net.RegisterSysProtoV2(8, 134, sysdef.SiFlyingSwordEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyingSwordEquipSys).c2sMatrixGraphTokeOff
	})
	net.RegisterSysProtoV2(8, 135, sysdef.SiFlyingSwordEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyingSwordEquipSys).c2sTakeOnAppear
	})
	engine.RegAttrCalcFn(attrdef.SaFlyingSwordEquip, flyingSwordEquipProperty)
	engine.RegAttrAddRateCalcFn(attrdef.SaFlyingSwordEquip, flyingSwordEquipPropertyAddRate)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnDefaultFlyingSwordEquip, func(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(ids) < 1 {
			return 0
		}
		obj := player.GetSysObj(sysdef.SiFlyingSwordEquip)
		if obj == nil || !obj.IsOpen() {
			return 0
		}
		sys := obj.(*FlyingSwordEquipSys)
		if sys == nil {
			return 0
		}
		var needQuality = ids[0]
		equip := sys.getFlyingSwordBaseEquip(0)
		var count uint32
		for _, item := range equip.PosMap {
			config := jsondata.GetItemConfig(item.ItemId)
			if config == nil {
				continue
			}
			if config.Quality < needQuality {
				continue
			}
			count += 1
		}
		return count
	})
}
