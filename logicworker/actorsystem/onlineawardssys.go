/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type OnlineAwardsSys struct {
	Base
}

func (s *OnlineAwardsSys) s2cInfo() {
	s.SendProto3(10, 20, &pb3.S2C_10_20{
		Data: s.getData(),
	})
}

func (s *OnlineAwardsSys) getData() *pb3.OnlineAwardsData {
	data := s.GetBinaryData().OnlineAwardsData
	if data == nil {
		s.GetBinaryData().OnlineAwardsData = &pb3.OnlineAwardsData{}
		data = s.GetBinaryData().OnlineAwardsData
	}
	if data.NextIndex == 0 {
		data.NextIndex = 1
	}
	if data.LastStartOnlineTime == 0 {
		data.LastStartOnlineTime = time_util.NowSec()
	}
	return data
}

func (s *OnlineAwardsSys) OnReconnect() {
	s.s2cInfo()
}

func (s *OnlineAwardsSys) OnLogin() {
	s.getData().LastStartOnlineTime = time_util.NowSec()
	s.s2cInfo()
}

func (s *OnlineAwardsSys) OnOpen() {
	s.s2cInfo()
}

func (s *OnlineAwardsSys) OnLogout() {
	s.updateOnlineTime()
}

func (s *OnlineAwardsSys) c2sRecAwards(msg *base.Message) error {
	var req pb3.C2S_10_21
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	config := jsondata.GetOnlineAwardsByIdx(data.NextIndex)
	if config == nil {
		return neterror.ConfNotFoundError("onlineAwards config not found %d", data.NextIndex)
	}
	s.updateOnlineTime()
	if data.TotalOnlineSec < config.OnlineTime {
		return neterror.ParamsInvalidError("onlineAwards not enough %d %d", data.TotalOnlineSec, config.OnlineTime)
	}
	data.TotalOnlineSec = 0
	data.NextIndex += 1
	owner := s.GetOwner()
	engine.GiveRewards(owner, config.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogOnlineAwards})
	s.SendProto3(10, 21, &pb3.S2C_10_21{
		Data: data,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogOnlineAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(config.Idx),
	})
	return nil
}

// 内部调用：更新到当前时间的在线时长
func (s *OnlineAwardsSys) updateOnlineTime() {
	data := s.getData()
	config := jsondata.GetOnlineAwardsByIdx(data.NextIndex)
	if config == nil {
		return
	}
	now := time_util.NowSec()
	onlineSeconds := now - data.LastStartOnlineTime
	data.TotalOnlineSec += onlineSeconds
	data.LastStartOnlineTime = now
}

func getOnlineAwardsSys(player iface.IPlayer) *OnlineAwardsSys {
	obj := player.GetSysObj(sysdef.SiOnlineAwards)
	if obj == nil || !obj.IsOpen() {
		return nil
	}
	sys, ok := obj.(*OnlineAwardsSys)
	if !ok {
		return nil
	}
	return sys
}

func init() {
	RegisterSysClass(sysdef.SiOnlineAwards, func() iface.ISystem {
		return &OnlineAwardsSys{}
	})

	net.RegisterSysProtoV2(10, 21, sysdef.SiOnlineAwards, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*OnlineAwardsSys).c2sRecAwards
	})
}
