/**
 * @Author: zjj
 * @Date: 2023/12/13
 * @Desc: 首次体验管理器
**/

package actorsystem

import (
	"encoding/json"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/common"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
)

const (
	FirstExperienceStateEnd = 1
)

type FirstExperienceSys struct {
	Base
}

func (s *FirstExperienceSys) GetData() *pb3.FirstExperienceMgr {
	if s.GetBinaryData().FirstExperienceMgr == nil {
		s.GetBinaryData().FirstExperienceMgr = &pb3.FirstExperienceMgr{}
	}
	state := s.GetBinaryData().FirstExperienceMgr
	if state.FreeMgr == nil {
		state.FreeMgr = make(map[uint32]*pb3.FirstExperience)
	}
	return state
}

func (s *FirstExperienceSys) OneSecLoop() {
	s.checkExpired()
}

func (s *FirstExperienceSys) OnLogin() {
	s.checkExpired()
	s.s2cInfo()
}

func (s *FirstExperienceSys) OnReconnect() {
	s.checkExpired()
	s.s2cInfo()
}

func (s *FirstExperienceSys) OnOpen() {
	s.s2cInfo()
}

func (s *FirstExperienceSys) s2cInfo() {
	s.SendProto3(55, 0, &pb3.S2C_55_0{
		Mgr: s.GetData(),
	})
}

func (s *FirstExperienceSys) s2cStartExperience(typ uint32) {
	s.SendProto3(55, 2, &pb3.S2C_55_2{
		Val: s.GetData().FreeMgr[typ],
	})
}

func (s *FirstExperienceSys) s2cEndExperience(typ uint32) {
	s.SendProto3(55, 1, &pb3.S2C_55_1{
		Val: s.GetData().FreeMgr[typ],
	})
}

// CheckExperience
// true 体验过
// false 未体验
func (s *FirstExperienceSys) CheckExperience(typ pb3.ExperienceType) (bool, error) {
	_, ok := pb3.ExperienceType_name[int32(typ)]
	if !ok {
		return false, neterror.ConfNotFoundError("not found enum , val is %d", int32(typ))
	}
	v, ok := s.GetData().FreeMgr[uint32(typ)]
	if !ok {
		return false, nil
	}

	if v == nil {
		return false, nil
	}

	// 从未体验过
	if v.Count == 0 && v.ExperienceExpiredAt == 0 {
		return false, nil
	}

	// 体验过
	if v.ExperienceExpiredAt != 0 && v.ExperienceExpiredAt < time_util.NowSec() {
		return true, nil
	}

	return true, nil
}

func (s *FirstExperienceSys) checkExpired() {
	data := s.GetData()
	nowSec := time_util.NowSec()
	for typ, experience := range data.FreeMgr {
		// 已经结束
		if experience.State == FirstExperienceStateEnd {
			continue
		}

		// 到达结束时间
		if nowSec >= experience.ExperienceExpiredAt {
			s.EndFirstExperience(typ, nil)
			continue
		}
	}
}

func (s *FirstExperienceSys) IsInExperience(typ pb3.ExperienceType, ext uint32) (bool, error) {
	_, ok := pb3.ExperienceType_name[int32(typ)]
	if !ok {
		return false, neterror.ConfNotFoundError("not found enum , val is %d", int32(typ))
	}
	v, ok := s.GetData().FreeMgr[uint32(typ)]
	if !ok {
		return false, nil
	}

	if v == nil {
		return false, nil
	}

	if v.ExtU32Param != ext {
		return false, nil
	}

	// 从未体验过
	if v.Count == 0 && v.ExperienceExpiredAt == 0 {
		return false, nil
	}

	// 体验未结束
	if v.ExperienceExpiredAt != 0 && v.ExperienceExpiredAt >= time_util.NowSec() {
		return true, nil
	}

	return false, nil
}

func (s *FirstExperienceSys) StartFirstExperience(typ uint32) {
	player := s.GetOwner()
	experienceConf, err := jsondata.GetFirstExperienceConf(typ)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	var giveAwardsByFirstExperienceCompensate = func() {
		if len(experienceConf.Compensate) == 0 {
			return
		}
		engine.GiveRewards(player, experienceConf.Compensate, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogGiveAwardsByFirstExperienceCompensate,
		})
	}

	experience, err := s.CheckExperience(pb3.ExperienceType(typ))
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	// 已经体验过 给补偿
	if experience {
		giveAwardsByFirstExperienceCompensate()
		return
	}

	data := s.GetData()
	experienceData, ok := data.FreeMgr[typ]
	if !ok {
		data.FreeMgr[typ] = &pb3.FirstExperience{
			Id: typ,
		}
		experienceData = data.FreeMgr[typ]
	}

	var timeout uint32
	var canStart = true
	utils.ProtectRun(func() {
		f, ok := beforeStartFirstExperienceMgr[typ]
		if !ok {
			return
		}
		canStart, timeout = f(player, experienceData, experienceConf)
	})

	// 不能激活 给补偿 标记已经结束体验
	if !canStart {
		experienceData.State = FirstExperienceStateEnd
		giveAwardsByFirstExperienceCompensate()
		return
	}

	nowSec := time_util.NowSec()
	experienceData.StartExperienceAt = nowSec
	experienceData.ExperienceExpiredAt = nowSec + timeout
	s.s2cStartExperience(typ)

	utils.ProtectRun(func() {
		f, ok := startFirstExperienceMgr[typ]
		if !ok {
			return
		}
		f(player, experienceData, experienceConf)
	})

	// 日志打点
	bytes, _ := json.Marshal(experienceData)
	logworker.LogPlayerBehavior(player, pb3.LogId_LogFirstExperienceSysStartExperience, &pb3.LogPlayerCounter{
		NumArgs: uint64(typ),
		StrArgs: string(bytes),
	})

	// 直接结束
	if experienceData.ExperienceExpiredAt != 0 && experienceData.ExperienceExpiredAt == experienceData.StartExperienceAt {
		s.EndFirstExperience(typ, nil)
	}
}

// 处理使用体验道具
func handleUseItemFirstExperience(player iface.IPlayer, _ *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) == 0 {
		return
	}
	typ := conf.Param[0]

	obj := player.GetSysObj(sysdef.SiFirstExperience)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*FirstExperienceSys)
	if !ok {
		return
	}

	sys.StartFirstExperience(typ)
	return true, true, 1
}

func (s *FirstExperienceSys) EndFirstExperience(typ uint32, st *pb3.CommonSt) {
	experienceData, ok := s.GetData().FreeMgr[typ]
	if !ok || experienceData.State == FirstExperienceStateEnd {
		return
	}

	// 结束
	var canEndRet = true
	utils.ProtectRun(func() {
		f, ok := endExperienceMgr[typ]
		if !ok {
			return
		}
		canEndRet = f(s.GetOwner(), st)
	})

	// 不能结束
	if !canEndRet {
		s.GetOwner().LogWarn("not can stop %d", typ)
		return
	}

	experienceData.Count += 1
	experienceData.State = FirstExperienceStateEnd

	s.s2cEndExperience(typ)
	bytes, _ := json.Marshal(experienceData)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFirstExperienceSysEndExperience, &pb3.LogPlayerCounter{
		NumArgs: uint64(typ),
		StrArgs: string(bytes),
	})
}

func init() {
	RegisterSysClass(sysdef.SiFirstExperience, func() iface.ISystem {
		return &FirstExperienceSys{}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemFirstExperience, handleUseItemFirstExperience)
}
