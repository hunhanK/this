/**
 * @Author: zjj
 * @Date: 2024/11/22
 * @Desc: 战盾化形系统
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/fashion"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type BattleShieldTransformSys struct {
	Base
	upStar fashion.UpStar
}

func (s *BattleShieldTransformSys) CheckFashionActive(fashionId uint32) bool {
	_, ok := s.upStar.Fashions[fashionId]
	if !ok {
		return false
	}
	return true
}

func (s *BattleShieldTransformSys) GetFashionQuality(fashionId uint32) uint32 {
	conf := jsondata.GetBattleShieldTransformConf(fashionId)
	if conf == nil {
		return 0
	}
	return conf.Quality
}

func (s *BattleShieldTransformSys) GetFashionBaseAttr(fashionId uint32) jsondata.AttrVec {
	data := s.getData()
	dressData, ok := data.Fashions[fashionId]
	if !ok {
		return nil
	}
	shieldTransformConf := jsondata.GetBattleShieldTransformConf(dressData.GetId())
	if shieldTransformConf == nil {
		return nil
	}
	var attrs jsondata.AttrVec
	for _, star := range shieldTransformConf.StarConf {
		if star.Star != dressData.Star {
			continue
		}
		attrs = append(attrs, star.Attrs...)
		break
	}

	stage := data.FashionStageMap[dressData.Id]
	for _, transformStage := range shieldTransformConf.StageConf {
		if transformStage.Stage != stage {
			continue
		}
		attrs = append(attrs, transformStage.Attrs...)
		break
	}
	return attrs
}

func (s *BattleShieldTransformSys) init() bool {
	data := s.getData()
	s.upStar = fashion.UpStar{
		Fashions:  data.Fashions,
		LogId:     pb3.LogId_LogBattleShieldTransformFashionUpStar,
		CheckJob:  false,
		AttrSysId: attrdef.SaBattleShieldTransform,
		GetLvConfHandler: func(fashionId, lv uint32) *jsondata.FashionStarConf {
			conf := jsondata.BattleShieldTransformStarConf(fashionId, lv)
			if conf != nil {
				return &conf.FashionStarConf
			}
			return nil
		},
		GetFashionConfHandler: func(fashionId uint32) *jsondata.FashionMeta {
			conf := jsondata.GetBattleShieldTransformConf(fashionId)
			if conf != nil {
				return &conf.FashionMeta
			}
			return nil
		},
		AfterUpstarCb:   s.onUpStar,
		AfterActivateCb: s.onActivated,
	}

	if err := s.upStar.Init(); err != nil {
		s.LogError("BattleShieldTransformSys upStar init failed, err: %v", err)
		return false
	}
	return true
}

func (s *BattleShieldTransformSys) s2cInfo() {
	s.SendProto3(22, 20, &pb3.S2C_22_20{
		Data: s.getData(),
	})
}

func (s *BattleShieldTransformSys) getData() *pb3.BattleShieldTransformData {
	data := s.GetBinaryData().BattleShieldTransformData
	if data == nil {
		s.GetBinaryData().BattleShieldTransformData = &pb3.BattleShieldTransformData{}
		data = s.GetBinaryData().BattleShieldTransformData
	}
	if data.Fashions == nil {
		data.Fashions = make(map[uint32]*pb3.DressData)
	}
	if data.FashionStageMap == nil {
		data.FashionStageMap = make(map[uint32]uint32)
	}
	return data
}

func (s *BattleShieldTransformSys) OnReconnect() {
	s.s2cInfo()
}

func (s *BattleShieldTransformSys) OnInit() {
	if !s.IsOpen() {
		return
	}
	s.init()
}

func (s *BattleShieldTransformSys) OnLogin() {
	s.s2cInfo()
}

func (s *BattleShieldTransformSys) OnOpen() {
	if !s.init() {
		return
	}
	s.ResetSysAttr(attrdef.SaBattleShieldTransform)
	s.s2cInfo()
}

func (s *BattleShieldTransformSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_22_21
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	owner := s.GetOwner()
	err = s.upStar.Activate(owner, req.Id, false, false)
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (s *BattleShieldTransformSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_22_22
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	owner := s.GetOwner()
	err = s.upStar.Upstar(owner, req.Id, false)
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (s *BattleShieldTransformSys) c2sUpStage(msg *base.Message) error {
	var req pb3.C2S_22_23
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	data := s.getData()
	owner := s.GetOwner()
	id := req.Id
	fashionData, ok := data.Fashions[id]
	if !ok {
		return neterror.ParamsInvalidError("not active %d data", id)
	}

	stage := data.FashionStageMap[id]
	nextStage := stage + 1
	stageConf := jsondata.BattleShieldTransformStageConf(id, nextStage)
	if stageConf.Star > fashionData.Star {
		return neterror.ParamsInvalidError("not enough star %d %d", stageConf.Star, fashionData.Star)
	}

	if len(stageConf.Consumes) != 0 && !owner.ConsumeByConf(stageConf.Consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogBattleShieldTransformFashionUpStage}) {
		return neterror.ConsumeFailedError("%d %d consume failed", id, nextStage)
	}

	data.FashionStageMap[id] = nextStage
	s.SendProto3(22, 23, &pb3.S2C_22_23{
		Id:    req.Id,
		Stage: nextStage,
	})
	s.ResetSysAttr(attrdef.SaBattleShieldTransform)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogBattleShieldTransformFashionUpStage, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d", nextStage),
	})
	return nil
}

func (s *BattleShieldTransformSys) c2sAppear(msg *base.Message) error {
	var req pb3.C2S_22_24
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	id := req.Id
	data := s.getData()
	_, ok := data.Fashions[id]
	if !ok {
		return neterror.ParamsInvalidError("not active %d", id)
	}
	owner := s.GetOwner()
	owner.TakeOnAppear(appeardef.AppearPos_BattleShield, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_BattleShieldTransfiguration,
		AppearId: id,
	}, true)
	s.SendProto3(22, 24, &pb3.S2C_22_24{
		Id: id,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogBattleShieldTransformFashionAppear, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
	})
	return nil
}

func (s *BattleShieldTransformSys) onUpStar(fashionId uint32) {
	s.SendProto3(22, 22, &pb3.S2C_22_22{
		Id:   fashionId,
		Star: s.getData().Fashions[fashionId].Star,
	})
}

func (s *BattleShieldTransformSys) onActivated(fashionId uint32) {
	s.getData().FashionStageMap[fashionId] = 0
	s.SendProto3(22, 21, &pb3.S2C_22_21{
		Data:  s.getData().Fashions[fashionId],
		Stage: s.getData().FashionStageMap[fashionId],
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogBattleShieldTransformFashionActivated, &pb3.LogPlayerCounter{
		NumArgs: uint64(fashionId),
	})
}

func (s *BattleShieldTransformSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	owner := s.GetOwner()
	for _, dressData := range data.Fashions {
		shieldTransformConf := jsondata.GetBattleShieldTransformConf(dressData.GetId())
		if shieldTransformConf == nil {
			continue
		}

		for _, star := range shieldTransformConf.StarConf {
			if star.Star != dressData.Star {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, star.Attrs)
			break
		}

		stage := data.FashionStageMap[dressData.Id]
		for _, transformStage := range shieldTransformConf.StageConf {
			if transformStage.Stage != stage {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, transformStage.Attrs)
			break
		}
	}
}

func handleSaBattleShieldTransform(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiBattleShieldTransform)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*BattleShieldTransformSys)
	if !ok {
		return
	}
	sys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiBattleShieldTransform, func() iface.ISystem {
		return &BattleShieldTransformSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaBattleShieldTransform, handleSaBattleShieldTransform)
	net.RegisterSysProtoV2(22, 21, sysdef.SiBattleShieldTransform, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BattleShieldTransformSys).c2sActive
	})
	net.RegisterSysProtoV2(22, 22, sysdef.SiBattleShieldTransform, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BattleShieldTransformSys).c2sUpStar
	})
	net.RegisterSysProtoV2(22, 23, sysdef.SiBattleShieldTransform, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BattleShieldTransformSys).c2sUpStage
	})
	net.RegisterSysProtoV2(22, 24, sysdef.SiBattleShieldTransform, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BattleShieldTransformSys).c2sAppear
	})
	initBattleShieldTransformGm()
}

func initBattleShieldTransformGm() {
	gmevent.Register("battleShieldTransformSys.changeStar", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiBattleShieldTransform)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys, ok := obj.(*BattleShieldTransformSys)
		if !ok {
			return false
		}
		sys.getData().Fashions[utils.AtoUint32(args[0])].Star = utils.AtoUint32(args[1])
		sys.ResetSysAttr(attrdef.SaBattleShieldTransform)
		sys.s2cInfo()
		return true
	}, 1)
}
