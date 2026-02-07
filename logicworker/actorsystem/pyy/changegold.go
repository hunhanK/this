/**
 * @Author: LvYuMeng
 * @Date: 2023/11/14
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type ChangeGoldSys struct {
	PlayerYYBase
}

func (s *ChangeGoldSys) Login() {
	s.s2cInfo()
}

func (s *ChangeGoldSys) OnReconnect() {
	s.s2cInfo()
}

func (s *ChangeGoldSys) OnEnd() {
	conf := jsondata.GetChangeGoldConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	var rewards jsondata.StdRewardVec
	for classId, classConf := range conf.ClassList {
		awardConf := classConf.ItemList
		for k, v := range awardConf {
			formalId := utils.Make64(k, classId)
			if data.ChangeGold[formalId] != ChangeGoldActive {
				continue
			}
			rewards = jsondata.MergeStdReward(rewards, v.Awards)
			data.ChangeGold[formalId] = ChangeGoldReceive
		}
	}
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_ChangeGold,
			Rewards: rewards,
		})
	}
	return
}

func (s *ChangeGoldSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.ChangeGold {
		return
	}
	delete(yyData.ChangeGold, s.Id)
}

func (s *ChangeGoldSys) GetData() *pb3.PYYChangeGold {
	yyData := s.GetYYData()
	if nil == yyData.ChangeGold {
		yyData.ChangeGold = make(map[uint32]*pb3.PYYChangeGold)
	}
	if nil == yyData.ChangeGold[s.Id] {
		yyData.ChangeGold[s.Id] = &pb3.PYYChangeGold{}
	}
	if nil == yyData.ChangeGold[s.Id].ChangeGold {
		yyData.ChangeGold[s.Id].ChangeGold = make(map[uint64]uint32)
	}
	return yyData.ChangeGold[s.Id]
}

func (s *ChangeGoldSys) OnOpen() {
	s.clear()
	s.s2cInfo()
}

func (s *ChangeGoldSys) clear() {
	if data := s.GetData(); nil != data {
		s.GetYYData().ChangeGold[s.Id] = nil
	}
}

func (s *ChangeGoldSys) GetRecentRecord() *pb3.RecentChangGoldPlayer {
	staticVar := gshare.GetStaticVar()
	if nil == staticVar.ChangGoldRecord {
		staticVar.ChangGoldRecord = make(map[uint32]*pb3.RecentChangGoldPlayer)
	}
	if nil == staticVar.ChangGoldRecord[s.Id] {
		staticVar.ChangGoldRecord[s.Id] = &pb3.RecentChangGoldPlayer{}
	}
	if nil == staticVar.ChangGoldRecord[s.Id].Record {
		staticVar.ChangGoldRecord[s.Id].Record = make(map[uint64]string)
	}
	return staticVar.ChangGoldRecord[s.Id]
}

func (s *ChangeGoldSys) s2cInfo() {
	rsp := &pb3.S2C_49_0{ActiveId: s.Id}
	data, recordObj := s.GetData(), s.GetRecentRecord()
	for k, v := range data.ChangeGold {
		rsp.Rewards = append(rsp.Rewards, &pb3.Key64Value{Key: k, Value: v})
	}
	for k, v := range recordObj.Record {
		rsp.Names = append(rsp.Names, &pb3.Key64Str{Key: k, Value: v})
	}
	s.SendProto3(49, 0, rsp)
}

func (s *ChangeGoldSys) c2sInfo(_ *base.Message) error {
	s.s2cInfo()
	return nil
}

func (s *ChangeGoldSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_49_1
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return err
	}
	classId, getItemId := req.GetYeqianId(), req.GetId()
	conf := jsondata.GetChangeGoldConf(s.ConfName, s.ConfIdx)
	if conf == nil || nil == conf.ClassList[classId] || nil == conf.ClassList[classId].ItemList {
		return neterror.ConfNotFoundError("yyId:%d no change gold conf(%d)", s.Id, getItemId)
	}
	awardConf := conf.ClassList[classId].ItemList
	name := s.GetPlayer().GetName()
	data := s.GetData()
	recordObj := s.GetRecentRecord()
	var getList []uint64
	var rewards jsondata.StdRewardVec
	if getItemId > 0 {
		formalId := utils.Make64(getItemId, classId)
		if data.ChangeGold[formalId] != ChangeGoldActive {
			s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
			return nil
		}
		rewards = awardConf[getItemId].Awards
		getList = append(getList, formalId)
	} else {
		for k, v := range awardConf {
			formalId := utils.Make64(k, classId)
			if data.ChangeGold[formalId] != ChangeGoldActive {
				continue
			}
			rewards = jsondata.MergeStdReward(rewards, v.Awards)
			getList = append(getList, formalId)
		}
	}

	if nil == rewards {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	if !engine.CheckRewards(s.GetPlayer(), rewards) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}
	for _, formalId := range getList {
		data.ChangeGold[formalId] = ChangeGoldReceive
		recordObj.Record[formalId] = name
	}
	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogChangeGold})
	}
	s.s2cInfo()
	return nil
}

const (
	ChangeGoldNoActive = 0
	ChangeGoldActive   = 1
	ChangeGoldReceive  = 2
)

func (s *ChangeGoldSys) onCheckFirstDrop(dropInfos *pb3.DropInfos) {
	conf := jsondata.GetChangeGoldConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		s.LogError("yyId:%d no change gold conf", s.Id)
		return
	}

	data := s.GetData()
	itemList := make([]uint64, 0, len(dropInfos.DropList))
	for _, dropInfo := range dropInfos.DropList {
		dropItemId := dropInfo.ItemId
		if dropItemId <= 0 {
			continue
		}

		itemConf := jsondata.GetItemConfig(dropItemId)
		if nil == itemConf {
			continue
		}

		if itemConf.Job > 0 && s.GetPlayer().GetJob() != uint32(itemConf.Job) {
			continue
		}

		if itemConf.Sex > 0 && s.GetPlayer().GetSex() != uint32(itemConf.Sex) {
			continue
		}

		for classId, classConf := range conf.ClassList {
			dropItemConf := classConf.ItemList[dropItemId]
			if dropItemConf == nil {
				continue
			}
			formalId := utils.Make64(dropItemId, classId)
			if data.ChangeGold[formalId] != ChangeGoldNoActive {
				continue
			}

			data.ChangeGold[formalId] = ChangeGoldActive
			itemList = append(itemList, formalId)
		}
	}

	if len(itemList) > 0 {
		s.SendProto3(49, 2, &pb3.S2C_49_2{ActiveId: s.Id, ItemList: itemList})
	}
}

func onCheckChangeGoldFirstDrop(player iface.IPlayer, dropInfos *pb3.DropInfos) {
	yyObjs := player.GetPYYObjList(yydefine.YYChangeGold)
	if len(yyObjs) <= 0 {
		return
	}
	for _, obj := range yyObjs {
		if mod, ok := obj.(*ChangeGoldSys); ok {
			mod.onCheckFirstDrop(dropInfos)
		}
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYChangeGold, func() iface.IPlayerYY {
		return &ChangeGoldSys{}
	})

	net.RegisterYYSysProtoV2(49, 0, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*ChangeGoldSys).c2sInfo
	})
	net.RegisterYYSysProtoV2(49, 1, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*ChangeGoldSys).c2sAward
	})

	event.RegActorEvent(custom_id.AeDropInfo, func(actor iface.IPlayer, args ...interface{}) {
		if len(args) <= 0 {
			return
		}
		dropItems, ok := args[0].(*pb3.DropInfos)
		if !ok {
			return
		}
		onCheckChangeGoldFirstDrop(actor, dropItems)
	})
}
