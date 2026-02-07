/**
 * @Author: yangqibin
 * @Desc: 无极仙宫玩家系统
 * @Date: 2023/12/5 10:12
 */

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/inffairyplacemgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"

	wordmonitor2 "github.com/gzjjyz/wordmonitor"
)

type InfFairyPlaceSys struct {
	Base
}

func (sys *InfFairyPlaceSys) OnLogin() {
	sys.sendLocalInfo()
	sys.sendCrossInfo()
	sys.tryBroadcastLocalFairyPlaceMasterOnline()
	sys.tryBroadcastCrossFairyPlaceMasterOnline()

}

func (sys *InfFairyPlaceSys) tryBroadcastLocalFairyPlaceMasterOnline() {
	jobId, err := sys.getMyJobId()
	if err != nil {
		return
	}

	if jobId == custom_id.LocalInfFairyPlaceJob_Master {
		engine.BroadcastTipMsgById(tipmsgid.TpLocalFairyPlaceMasterOnline, sys.GetOwner().GetName())
	}
}

func (sys *InfFairyPlaceSys) tryBroadcastCrossFairyPlaceMasterOnline() {
	if !engine.FightClientExistPredicate(base.SmallCrossServer) {
		return
	}
}

func (sys *InfFairyPlaceSys) OnReconnect() {
	sys.sendLocalInfo()
	sys.sendCrossInfo()
}

func (sys *InfFairyPlaceSys) c2sLocalSignature(msg *base.Message) error {
	var req pb3.C2S_169_2
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("c2sLocalSignature UnpackagePbmsg error:%v", err)
	}

	engine.SendWordMonitor(
		wordmonitor.InfFairyPlaceSignature,
		wordmonitor.LocalInfFairyPlaceSign,
		req.Signature,
		wordmonitoroption.WithPlayerId(sys.GetOwner().GetId()),
		wordmonitoroption.WithCommonData(sys.GetOwner().BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(sys.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
	)
	return nil
}

func (sys *InfFairyPlaceSys) c2sCrossSignature(msg *base.Message) error {
	var req pb3.C2S_169_3
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("c2sCrossSignature UnpackagePbmsg error:%v", err)
	}

	engine.SendWordMonitor(
		wordmonitor.InfFairyPlaceSignature,
		wordmonitor.CrossInfFairyPlaceSign,
		req.Signature,
		wordmonitoroption.WithPlayerId(sys.GetOwner().GetId()),
		wordmonitoroption.WithCommonData(sys.GetOwner().BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(sys.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
	)

	return nil
}

func (sys *InfFairyPlaceSys) sendLocalInfo() {
	info := inffairyplacemgr.GetLocalInfFairyPlaceMgr().PackLocalInfFairyPlaceInfoToClient()
	if info == nil {
		return
	}

	sys.SendProto3(169, 0, &pb3.S2C_169_0{
		LocalInf: info,
	})
}

func (sys *InfFairyPlaceSys) sendCrossInfo() {
	err := engine.CallFightSrvFunc(
		base.SmallCrossServer,
		sysfuncid.G2FSendCrossInfFairyPlaceInfoReq,
		&pb3.G2FSendCrossInfFairyPlaceInfoReq{
			PlayerId: sys.GetOwner().GetId(),
			PfId:     engine.GetPfId(),
			SrvId:    engine.GetServerId(),
		})

	if err != nil {
		sys.LogError("sendCrossInfo failed err: %s", err)
		return
	}

	sys.LogDebug("sendCrossInfo successed")
}

func (sys *InfFairyPlaceSys) getMyJobId() (myJobId uint32, err error) {
	info := inffairyplacemgr.GetLocalInfFairyPlaceMgr().GetLocalInfo()
	if info == nil {
		return 0, neterror.ParamsInvalidError("jobInfo is nil")
	}
	for jid, actorId := range info.JobInfo {
		if actorId == sys.GetOwner().GetId() {
			myJobId = jid
			break
		}
	}

	if myJobId == 0 {
		return 0, neterror.ParamsInvalidError("not a official")
	}

	return
}

func onLocalInfFairyPlaceSignWordCheckRet(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}

	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.TpSensitiveWord)
		return nil
	}

	info := inffairyplacemgr.GetLocalInfFairyPlaceMgr().GetLocalInfo()
	if info == nil {
		return neterror.InternalError("c2sLocalSignature jobInfo is nil")
	}

	sys, ok := player.GetSysObj(sysdef.SiInfFairyPlace).(*InfFairyPlaceSys)
	if !ok || !sys.IsOpen() {
		return nil
	}

	myJobId, err := sys.getMyJobId()
	if err != nil {
		return err
	}

	inffairyplacemgr.GetLocalInfFairyPlaceMgr().GetLocalInfo().SignatureInfo[myJobId] = word.Content

	sys.sendLocalInfo()
	return nil
}

func onCrossInfFairyPlaceSignWordCheckRet(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}

	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.TpSensitiveWord)
		return nil
	}

	engine.CallFightSrvFunc(
		base.SmallCrossServer,
		sysfuncid.G2FCrossInfFairyPlaceSign,
		&pb3.G2FCrossInfFairyPlaceSign{
			Signature: word.Content,
			PlayerId:  player.GetId(),
			PfId:      engine.GetPfId(),
			SrvId:     engine.GetServerId(),
		})

	return nil
}

func init() {
	RegisterSysClass(sysdef.SiInfFairyPlace, func() iface.ISystem {
		return &InfFairyPlaceSys{}
	})

	engine.RegWordMonitorOpCodeHandler(wordmonitor.LocalInfFairyPlaceSign, onLocalInfFairyPlaceSignWordCheckRet)
	engine.RegWordMonitorOpCodeHandler(wordmonitor.CrossInfFairyPlaceSign, onCrossInfFairyPlaceSignWordCheckRet)

	net.RegisterSysProtoV2(169, 0, sysdef.SiInfFairyPlace, func(sys iface.ISystem) func(*base.Message) error {
		return func(m *base.Message) error {
			sys.(*InfFairyPlaceSys).sendLocalInfo()
			return nil
		}
	})

	net.RegisterSysProtoV2(169, 1, sysdef.SiInfFairyPlace, func(sys iface.ISystem) func(*base.Message) error {
		return func(m *base.Message) error {
			if !engine.FightClientExistPredicate(base.SmallCrossServer) {
				return nil
			}
			sys.(*InfFairyPlaceSys).sendCrossInfo()
			return nil
		}
	})

	net.RegisterSysProtoV2(169, 2, sysdef.SiInfFairyPlace,
		func(sys iface.ISystem) func(*base.Message) error {
			return func(m *base.Message) error {
				return sys.(*InfFairyPlaceSys).c2sLocalSignature(m)
			}
		})

	net.RegisterSysProtoV2(169, 3, sysdef.SiInfFairyPlace,
		func(sys iface.ISystem) func(*base.Message) error {
			return func(m *base.Message) error {
				return sys.(*InfFairyPlaceSys).c2sCrossSignature(m)
			}
		})
}
