/**
 * @Author: zjj
 * @Date: 2025年2月17日
 * @Desc: 等级投资
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type LevelInvestSys struct {
	Base
}

func (s *LevelInvestSys) s2cInfo() {
	s.SendProto3(8, 70, &pb3.S2C_8_70{
		Data: s.getData(),
	})
}

func (s *LevelInvestSys) getData() *pb3.LevelInvestData {
	data := s.GetBinaryData().LevelInvestData
	if data == nil {
		s.GetBinaryData().LevelInvestData = &pb3.LevelInvestData{}
		data = s.GetBinaryData().LevelInvestData
	}
	if data.LevelAwards == nil {
		data.LevelAwards = make(map[uint32]uint32)
	}
	return data
}

func (s *LevelInvestSys) OnReconnect() {
	s.s2cInfo()
}

func (s *LevelInvestSys) OnLogin() {
	s.s2cInfo()
}

func (s *LevelInvestSys) OnOpen() {
	s.s2cInfo()
}

func (s *LevelInvestSys) getConf() (*jsondata.LevelInvestConfig, error) {
	conf := jsondata.GetLevelInvestConf()
	if conf == nil {
		return nil, neterror.ConfNotFoundError("conf not found")
	}
	return conf, nil
}

func (s *LevelInvestSys) C2SBuyLayer(msg *base.Message) error {
	var req pb3.C2S_8_71
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	owner := s.GetOwner()
	layer := req.Layer
	buyLayerBit := data.BuyLayerBit
	if utils.IsSetBit(buyLayerBit, layer) {
		return neterror.ParamsInvalidError("%d already buy", layer)
	}

	var layerConfMap = make(map[uint32]*jsondata.LevelInvestLayer)
	for _, layerConf := range conf.Layer {
		layerConfMap[layerConf.Idx] = layerConf
	}

	var notBuyLayer []uint32
	for i := uint32(1); i <= layer; i++ {
		if utils.IsSetBit(buyLayerBit, i) {
			continue
		}
		_, ok := layerConfMap[i]
		if !ok {
			continue
		}
		notBuyLayer = append(notBuyLayer, i)
	}

	if len(notBuyLayer) == 0 {
		return neterror.ParamsInvalidError("not can buy layer")
	}

	var totalConsume jsondata.ConsumeVec
	var totalAwards jsondata.StdRewardVec
	for _, layer := range notBuyLayer {
		layerConf, ok := layerConfMap[layer]
		if !ok {
			continue
		}
		totalConsume = append(totalConsume, layerConf.Consume...)
		totalAwards = append(totalAwards, layerConf.Awards...)
	}

	if len(totalConsume) == 0 || !owner.ConsumeByConf(totalConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogLevelInvestBuyLayer}) {
		return neterror.ConsumeFailedError("consume not enough")
	}

	for _, layer := range notBuyLayer {
		data.BuyLayerBit = utils.SetBit(data.BuyLayerBit, layer)
	}

	if len(totalAwards) > 0 {
		engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLevelInvestBuyLayer})
	}
	s.SendProto3(8, 71, &pb3.S2C_8_71{
		BuyLayerBit: data.BuyLayerBit,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogLevelInvestBuyLayer, &pb3.LogPlayerCounter{
		NumArgs: uint64(layer),
		StrArgs: fmt.Sprintf("%d_%d", buyLayerBit, data.BuyLayerBit),
	})
	return nil
}

func (s *LevelInvestSys) C2SRecLevelAwards(msg *base.Message) error {
	var req pb3.C2S_8_72
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	data := s.getData()
	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	if data.BuyLayerBit == 0 {
		return neterror.ParamsInvalidError("not buy layer")
	}

	level := req.Level
	recFlag := data.LevelAwards[level]
	if recFlag != 0 && recFlag == data.BuyLayerBit {
		return neterror.ParamsInvalidError("awards not can rec")
	}

	var levelConfig *jsondata.LevelInvestLevelConf
	for _, levelConf := range conf.LevelConf {
		if levelConf.Level != level {
			continue
		}
		levelConfig = levelConf
		break
	}
	if levelConfig == nil {
		return neterror.ParamsInvalidError("not found %d level config", level)
	}

	var totalAwards jsondata.StdRewardVec
	for _, money := range levelConfig.ReturnMoney {
		if !utils.IsSetBit(data.BuyLayerBit, money.Idx) {
			continue
		}
		if utils.IsSetBit(recFlag, money.Idx) {
			continue
		}
		totalAwards = append(totalAwards, money.Awards...)
	}
	if len(totalAwards) == 0 {
		return neterror.ParamsInvalidError("not can rec awards %d %d", data.BuyLayerBit, recFlag)
	}

	newFlag := data.LevelAwards[level]
	for _, money := range levelConfig.ReturnMoney {
		if !utils.IsSetBit(data.BuyLayerBit, money.Idx) {
			continue
		}

		if utils.IsSetBit(newFlag, money.Idx) {
			continue
		}

		newFlag = utils.SetBit(newFlag, money.Idx)
	}
	data.LevelAwards[level] = newFlag
	owner := s.GetOwner()
	engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLevelInvestBuyLayer})
	s.SendProto3(8, 72, &pb3.S2C_8_72{
		Level:    level,
		LayerBit: newFlag,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogLevelInvestLevelAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(level),
		StrArgs: fmt.Sprintf("%d_%d", recFlag, newFlag),
	})
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiLevelInvest, func() iface.ISystem {
		return &LevelInvestSys{}
	})
	net.RegisterSysProtoV2(8, 71, sysdef.SiLevelInvest, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LevelInvestSys).C2SBuyLayer
	})
	net.RegisterSysProtoV2(8, 72, sysdef.SiLevelInvest, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LevelInvestSys).C2SRecLevelAwards
	})
}
