package manager

import (
	"github.com/gzjjyz/srvlib/utils"
	"google.golang.org/protobuf/proto"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

func isActorInThisServer(actorId uint64) bool {
	return GetPlayerPtrById(actorId) != nil || HasOfflineData(actorId)
}

func SendOtherPlayerInfo(actor iface.IPlayer, req *pb3.C2S_2_27) {
	rsp := &pb3.S2C_2_27{}
	if engine.IsRobot(req.Id) {
		viewRobot(actor, req.Id, req.ViewType, req.Param)
		return
	}
	GetPlayerInfo(rsp, req.GetId(), req.GetViewType(), req.GetParam())

	actor.SendProto3(2, 27, rsp)
	// logger.LogDebug("协议2_27返回: %v", rsp)
}

func GetPlayerInfo(rsp *pb3.S2C_2_27, targetId uint64, viewType uint32, param string) {
	rsp.Id = targetId
	rsp.ViewType = viewType
	rsp.Param = param
	rsp.Info = &pb3.DetailedRoleInfo{}
	if targetPlayer := GetPlayerPtrById(targetId); nil != targetPlayer {
		var idx uint32
		for idx = common.ViewTypeBegin; idx <= common.ViewTypeEnd; idx++ {
			if utils.IsSetBit(viewType, idx) {
				TriggerViewFunc(idx, targetPlayer, rsp.Info)
			}
		}
		rsp.IsOnline = true
	} else {
		if data, ok := GetData(targetId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
			rsp.Info.Basic = data
			rsp.IsOnline = false
		}
		if property, ok := GetData(targetId, gshare.ActorDataProperty).(*pb3.OfflineProperty); ok {
			if nil != property.FightAttr {
				rsp.Info.FightProp = property.FightAttr
			}
		}
		if equipSt, ok := GetData(targetId, gshare.ActorDataEquip).(*pb3.OfflineEquipData); ok {
			rsp.Info.EquipDeatil = &pb3.EquipDetail{
				Equip:                   equipSt.Equip,
				Intensify:               equipSt.Intensify,
				EquipSuit:               equipSt.EquipSuit,
				EquipAwaken:             equipSt.EquipAwaken,
				KillDragonEquipSuitData: equipSt.KillDragonEquipSuitData,
				ExclusiveSign:           equipSt.ExclusiveSign,
			}
		}
		if data, ok := GetData(targetId, gshare.ActorShowStr).(*pb3.PlayerShowStrAttr); ok {
			rsp.Info.ShowStrAttr = proto.Clone(data).(*pb3.PlayerShowStrAttr)
		}
	}
	return
}

// todo 查看玩家信息 (要改通用)
func c2sViewOtherPlayer(player iface.IPlayer, msg *base.Message) error {
	req := &pb3.C2S_2_27{}
	if err := pb3.Unmarshal(msg.Data, req); nil != err {
		return err
	}
	if req.GetId() == 0 {
		return nil
	}
	targetId := req.GetId()
	viewType := req.GetViewType()
	if isActorInThisServer(targetId) || engine.IsRobot(targetId) {
		SendOtherPlayerInfo(player, req)
	} else {
		var crossServerTypeList = []base.ServerType{base.SmallCrossServer, base.MediumCrossServer}
		for _, serverType := range crossServerTypeList {
			err := engine.CallFightSrvFunc(serverType, sysfuncid.G2CViewPlayer, &pb3.G2CViewPlayer{
				ActorId:       player.GetId(),
				TargetActorId: targetId,
				ViewType:      viewType,
				PfId:          engine.GetPfId(),
				SrvId:         engine.GetServerId(),
				Param:         req.GetParam(),
				ServerType:    uint32(serverType),
			})
			if err != nil {
				player.LogError("err:%v", err)
				continue
			}
			break
		}
	}
	return nil
}

func crossViewOtherPlayer(buf []byte) {
	msg := pb3.C2GViewPlayer{}
	if err := pb3.Unmarshal(buf, &msg); err != nil {
		return
	}

	rsp := &pb3.S2C_2_27{}

	if !isActorInThisServer(msg.TargetActorId) {
		return
	}

	GetPlayerInfo(rsp, msg.TargetActorId, msg.ViewType, msg.Param)

	if buff, err := pb3.Marshal(rsp); nil != err {
		return
	} else {
		// 发回跨服
		engine.CallFightSrvFunc(base.ServerType(msg.ServerType), sysfuncid.G2CRetPlayerInfo, &pb3.G2CRetViewPlayer{
			PfId:    msg.RetPfId,
			SrvId:   msg.RetSrvId,
			ActorId: msg.RetActorId,
			Pb2:     buff,
		})
	}
}

func viewPlayerBasic(targetPlayer iface.IPlayer, info *pb3.DetailedRoleInfo) {
	if nil == info.Basic {
		info.Basic = &pb3.PlayerDataBase{}
	}
	info.Basic = targetPlayer.ToPlayerDataBase()
}

func viewPlayerEquip(targetPlayer iface.IPlayer, info *pb3.DetailedRoleInfo) {
	if nil == info.EquipDeatil {
		info.EquipDeatil = &pb3.EquipDetail{}
	}
	itemPool := targetPlayer.GetMainData().ItemPool
	if nil != itemPool {
		info.EquipDeatil.Equip = itemPool.Equips
	}
	binary := targetPlayer.GetBinaryData()
	info.EquipDeatil.Intensify = binary.Intensify
	info.EquipDeatil.AllGemData = binary.GetAllGemData()
	info.EquipDeatil.EquipSuit = binary.GetEquipSuitStrong()
	info.EquipDeatil.EquipAwaken = binary.GetEquipAwaken()
	info.EquipDeatil.KillDragonEquipSuitData = binary.GetKillDragonEquipSuitData()
	info.EquipDeatil.ExclusiveSign = binary.GetExclusiveSign().GetLv()
}

func viewPlayerFightAttr(targetPlayer iface.IPlayer, info *pb3.DetailedRoleInfo) {
	if sys := targetPlayer.GetAttrSys(); nil != sys {
		sys.PackFightPropertyData(info)
	}
}

func crossRetOtherPlayer(buf []byte) {
	msg := pb3.G2CRetViewPlayer{}
	if err := pb3.Unmarshal(buf, &msg); err != nil {
		return
	}

	player := GetPlayerPtrById(msg.ActorId)
	if nil == player {
		return
	}

	rsp := &pb3.S2C_2_27{}

	if err := pb3.Unmarshal(msg.Pb2, rsp); nil != err {
		return
	}

	player.SendProto3(2, 27, rsp)
}

func viewPlayerShowStr(player iface.IPlayer, info *pb3.DetailedRoleInfo) {
	info.ShowStrAttr = &pb3.PlayerShowStrAttr{
		ShowStr: player.PackageShowStr(),
	}
}

func init() {
	gshare.IsActorInThisServer = isActorInThisServer

	net.RegisterProto(2, 27, c2sViewOtherPlayer)

	RegisterViewFunc(common.ViewPlayerBasic, viewPlayerBasic)
	RegisterViewFunc(common.ViewPlayerEquip, viewPlayerEquip)
	RegisterViewFunc(common.ViewPlayerFightAttr, viewPlayerFightAttr)
	RegisterViewFunc(common.ViewPlayerShowStr, viewPlayerShowStr)

	engine.RegisterSysCall(sysfuncid.C2GViewPlayer, crossViewOtherPlayer)
	engine.RegisterSysCall(sysfuncid.C2GRetPlayerInfo, crossRetOtherPlayer)

}
