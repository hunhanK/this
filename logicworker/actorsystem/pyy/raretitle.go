/**
 * @Author: lzp
 * @Date: 2025/6/16
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type RareTitle struct {
	PlayerYYBase
}

const (
	RareTitleMissionType1 = 1 // 时装任务
)

func (s *RareTitle) Login() {
	s.s2cInfo()
}

func (s *RareTitle) OnOpen() {
	s.s2cInfo()
}

func (s *RareTitle) OnEnd() {
	conf := jsondata.GetPYYRareTitleConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.GetData()
	if s.checkCanFetch() {
		data.IsFetched = true
		s.GetPlayer().SendMail(&mailargs.SendMailSt{
			ConfId:  common.Mail_PYYRareTitleAwards,
			Rewards: conf.Rewards,
		})
	}
}

func (s *RareTitle) ResetData() {
	state := s.GetYYData()
	if nil == state.RareTitle {
		return
	}
	delete(state.RareTitle, s.Id)
}

func (s *RareTitle) OnReconnect() {
	s.s2cInfo()
}

func (s *RareTitle) GetData() *pb3.PYY_RareTitle {
	state := s.GetYYData()
	if state.RareTitle == nil {
		state.RareTitle = make(map[uint32]*pb3.PYY_RareTitle)
	}
	if state.RareTitle[s.Id] == nil {
		state.RareTitle[s.Id] = &pb3.PYY_RareTitle{}
	}
	return state.RareTitle[s.Id]
}

func (s *RareTitle) s2cInfo() {
	s.SendProto3(127, 165, &pb3.S2C_127_165{ActId: s.Id, Data: s.GetData()})
}

func (s *RareTitle) c2sFetch(msg *base.Message) error {
	var req pb3.C2S_127_166
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	conf := jsondata.GetPYYRareTitleConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("config not found")
	}

	if s.checkCanFetch() {
		data := s.GetData()
		data.IsFetched = true
		engine.GiveRewards(s.GetPlayer(), conf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYRareTitleReward,
		})

		player := s.GetPlayer()
		engine.BroadcastTipMsgById(conf.Broadcast, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(s.GetPlayer(), conf.Rewards))
		s.SendProto3(127, 166, &pb3.S2C_127_166{ActId: s.Id, IsSuccess: true})
		return nil
	}

	s.SendProto3(127, 166, &pb3.S2C_127_166{ActId: s.Id, IsSuccess: false})
	return nil
}

func (s *RareTitle) checkCanFetch() bool {
	conf := jsondata.GetPYYRareTitleConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return false
	}

	data := s.GetData()
	if data.IsFetched {
		return false
	}

	canFetch := true
	for mId, mConf := range conf.Missions {
		if mConf.Job > 0 && mConf.Job != s.GetPlayer().GetJob() {
			continue
		}
		if !utils.SliceContainsUint32(data.MIds, mId) {
			canFetch = false
		}
	}

	return canFetch
}

func (s *RareTitle) handleActiveFashion(fType, fId uint32) {
	conf := jsondata.GetPYYRareTitleConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.GetData()
	for _, mConf := range conf.Missions {
		if mConf.Type != RareTitleMissionType1 {
			continue
		}
		if mConf.Job > 0 && mConf.Job != s.GetPlayer().GetJob() {
			continue
		}

		if len(mConf.TargetVal) >= 2 {
			p1, p2 := mConf.TargetVal[0], mConf.TargetVal[1]
			if p1 == fType && p2 == fId {
				data.MIds = pie.Uint32s(data.MIds).Append(mConf.Id).Unique()
			}
		}
	}
	s.s2cInfo()
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYRareTitle, func() iface.IPlayerYY {
		return &RareTitle{}
	})

	net.RegisterYYSysProto(127, 166, (*RareTitle).c2sFetch)

	event.RegActorEvent(custom_id.AeRareTitleActiveFashion, func(player iface.IPlayer, args ...interface{}) {
		yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYRareTitle)
		for _, obj := range yyList {
			if len(args) < 1 {
				return
			}
			data, ok := args[0].(*custom_id.FashionSetEvent)
			if !ok {
				return
			}
			s, ok := obj.(*RareTitle)
			if !ok || !s.IsOpen() {
				continue
			}
			s.handleActiveFashion(data.FType, data.FashionId)
		}
	})
}
