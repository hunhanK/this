/**
 * @Author: LvYuMeng
 * @Date: 2023/12/11
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logicworker/weddingbanquetmgr"
	"jjyz/gameserver/net"
	"time"
)

type WeddingBanquetSys struct {
	Base
}

func (s *WeddingBanquetSys) OnInit() {
	binary := s.GetBinaryData()
	if nil == binary.WeddingBanquetData {
		binary.WeddingBanquetData = &pb3.WeddingBanquetData{}
	}
}

func (s *WeddingBanquetSys) OnLogin() {
	s.SetBanquetAttr()
}

func (s *WeddingBanquetSys) SetBanquetAttr() {
	data := s.GetData()
	s.owner.SetExtraAttr(attrdef.WeddingBanquetCandy, int64(data.Candy))
	s.owner.SetExtraAttr(attrdef.WeddingBanquetDelicious, int64(data.Deliciouts))
	s.owner.SetExtraAttr(attrdef.WeddingBanquetExpGet, int64(data.ExpGet))
	s.owner.SetExtraAttr(attrdef.WeddingBanquetExpId, int64(data.ExpId))
}

func (s *WeddingBanquetSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *WeddingBanquetSys) OnReconnect() {
	s.s2cInfo()
}

func (s *WeddingBanquetSys) GetData() *pb3.WeddingBanquetData {
	binary := s.GetBinaryData()
	if nil == binary.WeddingBanquetData {
		binary.WeddingBanquetData = &pb3.WeddingBanquetData{}
	}
	return binary.WeddingBanquetData
}

func (s *WeddingBanquetSys) s2cInfo() {
	weddingbanquetmgr.SendWeddingBanquetInfo(s.owner, utils.SetBit(0, custom_id.WeddingBanquetEnd)-1)
	weddingbanquetmgr.SendWeddingBeInvited(s.owner)
	weddingbanquetmgr.SendWeddingReserveList(s.owner)
}

func (s *WeddingBanquetSys) c2sReserve(msg *base.Message) error {
	var req pb3.C2S_53_11
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	marrySys, ok := s.owner.GetSysObj(sysdef.SiMarry).(*MarrySys)
	if !ok || !marrySys.IsOpen() {
		return neterror.ParamsInvalidError("marry sys not open")
	}

	data := marrySys.GetData()
	if !friendmgr.IsExistStatus(data.CommonId, custom_id.FsMarry) {
		return neterror.ParamsInvalidError("not marry")
	}

	fd, ok := friendmgr.GetFriendCommonDataById(data.CommonId)
	if !ok || nil == fd.MarryInfo {
		return neterror.InternalError("marry common data(%d) get err", data.CommonId)
	}

	if fd.MarryInfo.WeddingBanquetTimes[req.Grade] <= 0 {
		return neterror.ParamsInvalidError("wedding banquet grade(%d) time not enough", req.Grade)
	}

	conf := jsondata.GetWeddingBanquetConf()
	if nil == conf.Reserve[req.Id] {
		return neterror.ConfNotFoundError("not exist wedding reservation(%d)", req.Id)
	}

	startTime := time_util.ToTodayTime(conf.Reserve[req.Id].Start)
	if !req.IsToday {
		startTime = uint32((time.Unix(int64(startTime), 0).AddDate(0, 0, 1)).Unix())
	}
	checkTime := uint32((time_util.Now().AddDate(0, 0, 1)).Unix())
	if startTime > checkTime || startTime <= time_util.NowSec() {
		return neterror.ParamsInvalidError("not in reserve range")
	}

	key := weddingbanquetmgr.GetWeddingBanquetKey(req.Id, startTime)

	if !weddingbanquetmgr.CanReservedWeddingBanquet(s.owner.GetId(), key) {
		return neterror.ParamsInvalidError("has a wedding banquet")
	}

	fd.MarryInfo.WeddingBanquetTimes[req.Grade]--
	weddingbanquetmgr.ReservedWeddingBanquet(key, req.Grade, fd.ActorId1, fd.ActorId2, data.CommonId)

	endTime := weddingbanquetmgr.GetWeddingBanquetEndTime(key)
	weddingbanquetmgr.BroadProto3ToReservePlayer(key, 53, 20, &pb3.S2C_53_20{Times: fd.MarryInfo.WeddingBanquetTimes})
	weddingbanquetmgr.BroadProto3ToReservePlayer(key, 53, 23, &pb3.S2C_53_23{Id: req.Id, StartTime: startTime, Grade: req.Grade, EndTime: endTime})

	engine.Broadcast(chatdef.CIWorld, 0, 53, 11, &pb3.S2C_53_11{
		Id:        req.Id,
		Actor1:    manager.GetSimplyData(fd.ActorId1),
		Actor2:    manager.GetSimplyData(fd.ActorId2),
		Grade:     req.Grade,
		StartTime: startTime,
	}, 0)

	return nil
}

func (s *WeddingBanquetSys) c2sEnterFb(msg *base.Message) error {
	var req pb3.C2S_53_24
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	data := s.GetData()
	conf := jsondata.GetWeddingBanquetConf()

	banquet := weddingbanquetmgr.GetWBanquetInfo()
	if banquet.Id == 0 {
		return neterror.ParamsInvalidError("no banquet running")
	}
	key := weddingbanquetmgr.GetWeddingBanquetKey(banquet.Id, banquet.StartTime)
	if !weddingbanquetmgr.CheckWeddingBanquetLicense(key, s.owner.GetId(), custom_id.WbLicenseCanEnter) {
		return neterror.ParamsInvalidError("no license CanEnter to banquet(%d)", key)
	}
	lv := s.owner.GetLevel()
	if lv < conf.Level {
		s.owner.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}
	if data.ExpId == 0 {
		for expId, v := range conf.ExpLimit {
			if lv >= v.LvMin && lv <= v.LvMax {
				data.ExpId = expId
				s.owner.SetExtraAttr(attrdef.WeddingBanquetExpId, int64(data.ExpId))
				break
			}
		}
	}
	if data.ParticipateKey != key {
		data.ParticipateKey = key
		data.Deliciouts = 0
		s.owner.SetExtraAttr(attrdef.WeddingBanquetDelicious, 0)
	}
	return s.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterWeddingBanquet, &pb3.CommonSt{
		U32Param: banquet.Id,
	})
}

func (s *WeddingBanquetSys) c2sReqInvitation(msg *base.Message) error {
	var req pb3.C2S_53_12
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	conf := jsondata.GetWeddingBanquetConf()
	if s.owner.GetLevel() < conf.Level {
		s.owner.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}

	reserveInfo := weddingbanquetmgr.GetWeddingBanquetReserveInfo(req.GetId(), req.GetStartTime())
	if nil == reserveInfo {
		return neterror.ParamsInvalidError("wedding banquet id(%d),startTime(%d) not exist", req.Id, req.StartTime)
	}
	if weddingbanquetmgr.CheckWeddingBanquetLicense(reserveInfo.Id, s.owner.GetId(), custom_id.WbLicenseInvitationCanGetTime) {
		return neterror.ParamsInvalidError("banquet license invitation getTime end")
	}
	if reserveInfo.InviteIds[s.owner.GetId()] { //已经被邀请
		s.owner.SendTipMsg(tipmsgid.WeddingBanquetHasBuyInvitation)
		return nil
	}
	if reserveInfo.SelfBuyIds[s.owner.GetId()] { //已经购买
		return neterror.ParamsInvalidError("wedding banquet id(%d),startTime(%d) invitation has buy", req.Id, req.StartTime)
	}
	if reserveInfo.ReqEnter[s.owner.GetId()] { //申请列表中
		return nil
	}
	reserveInfo.ReqEnter[s.owner.GetId()] = true

	weddingbanquetmgr.BroadProto3ToReservePlayer(reserveInfo.Id, 53, 12, &pb3.S2C_53_12{
		Id:        req.Id,
		StartTime: req.StartTime,
		Actor:     manager.GetSimplyData(s.owner.GetId()),
	})
	return nil
}

func (s *WeddingBanquetSys) c2sInviteList(msg *base.Message) error {
	var req pb3.C2S_53_13
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	if req.Type <= 0 || req.Type >= custom_id.WeddingBanquetEnd {
		return neterror.ParamsInvalidError("not define wedding banquet info type")
	}

	reserveInfo := weddingbanquetmgr.GetWeddingBanquetReserveInfo(req.Id, req.StartTime)
	if nil == reserveInfo {
		return neterror.ParamsInvalidError("wedding banquet id(%d),startTime(%d) not exist", req.Id, req.StartTime)
	}

	weddingbanquetmgr.SendBanquetInfoByReserve(s.owner, reserveInfo, utils.SetBit(0, req.Type))
	return nil
}

func (s *WeddingBanquetSys) c2sHandleInvite(msg *base.Message) error {
	var req pb3.C2S_53_14
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	return weddingbanquetmgr.HandleInviteReq(s.owner, &req)
}

func (s *WeddingBanquetSys) c2sSendInvite(msg *base.Message) error {
	var req pb3.C2S_53_15
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	return weddingbanquetmgr.SendInvite(s.owner, req.Id, req.StartTime, req.ActorId)
}

func (s *WeddingBanquetSys) c2sGetReserveInfo(msg *base.Message) error {
	var req pb3.C2S_53_18
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	return weddingbanquetmgr.SendReserveInfo(s.owner, req.Id, req.StartTime)
}

func (s *WeddingBanquetSys) c2sReserveList(msg *base.Message) error {
	var req pb3.C2S_53_19
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	weddingbanquetmgr.SendWeddingReserveList(s.owner)
	return nil
}

const (
	wbBuyInviteTimes = 1
	wbBuyInvitation  = 2
)

func (s *WeddingBanquetSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_53_21
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	switch req.Type {
	case wbBuyInviteTimes:
		return weddingbanquetmgr.BuyInviteTimes(s.owner, req.Id, req.StartTime)
	case wbBuyInvitation:
		return weddingbanquetmgr.BuyInvitation(s.owner, req.Id, req.StartTime)
	}
	return nil
}

func onGatherBanquetCandy(player iface.IPlayer, buf []byte) {
	var st pb3.CommonSt
	if nil != pb3.Unmarshal(buf, &st) {
		return
	}
	wbData := player.GetBinaryData().WeddingBanquetData
	wbData.Candy++
	player.SetExtraAttr(attrdef.WeddingBanquetCandy, int64(wbData.Candy))
}

func onGatherBanquetDelicious(player iface.IPlayer, buf []byte) {
	var st pb3.CommonSt
	if nil != pb3.Unmarshal(buf, &st) {
		return
	}
	wbData := player.GetBinaryData().WeddingBanquetData
	wbData.Deliciouts++
	player.SetExtraAttr(attrdef.WeddingBanquetDelicious, int64(wbData.Deliciouts))
}

func onBanquetAddExp(player iface.IPlayer, buf []byte) {
	var st pb3.CommonSt
	if nil != pb3.Unmarshal(buf, &st) {
		return
	}

	lvSys, ok := player.GetSysObj(sysdef.SiLevel).(*LevelSys)
	if !ok {
		return
	}

	factor := st.U32Param
	actorRate := uint32(player.GetFightAttr(attrdef.ExpAddRate))
	expId := uint32(player.GetExtraAttr(attrdef.WeddingBanquetExpId))

	exp := int64(player.GetLevel() / 60 * factor)
	finalExp, _ := lvSys.CalcFinalExp(exp, true, actorRate)
	wbData := player.GetBinaryData().WeddingBanquetData

	conf := jsondata.GetWeddingBanquetConf()
	expConf, ok := conf.ExpLimit[expId]
	if !ok {
		return
	}
	if finalExp+int64(wbData.ExpGet) > int64(expConf.ExpLimit) {
		finalExp = int64(expConf.ExpLimit) - int64(wbData.ExpGet)
	}
	if finalExp <= 0 {
		return
	}
	finalExp = player.AddExp(finalExp, pb3.LogId_LogWeddingBanquetExp, false)
	wbData.ExpGet += uint32(finalExp)

	player.SetExtraAttr(attrdef.WeddingBanquetExpGet, int64(wbData.ExpGet))
}

func useItemAddWeddingBanquetDegree(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 2 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}
	s, ok := player.GetSysObj(sysdef.SiWeddingBanquet).(*WeddingBanquetSys)
	if !ok || !s.IsOpen() {
		player.SendTipMsg(tipmsgid.TpSySNotOpen)
		return false, false, 0
	}
	wbConf := jsondata.GetWeddingBanquetConf()
	if player.GetFbId() != wbConf.FbId {
		return false, false, 0
	}

	wbInfo := weddingbanquetmgr.GetWBanquetInfo()
	progressConf := jsondata.GetWeddingBanquetProgressConf(wbInfo.Progress)
	if nil == progressConf || progressConf.Type == custom_id.WbProgressPre {
		return false, false, 0
	}
	degree := conf.Param[0] * uint32(param.Count)
	err := player.CallActorFunc(actorfuncid.AddWeddingBanquetDegree, &pb3.CommonSt{U32Param: degree})
	if err != nil {
		return false, false, 0
	}
	player.AddExp(int64(conf.Param[1]), pb3.LogId_LogUseWeddingBanquetDegreeItem, false)
	return true, true, param.Count
}

func onBanquetNewDay(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiWeddingBanquet).(*WeddingBanquetSys)
	if !ok || !sys.IsOpen() {
		return
	}
	data := sys.GetData()
	data.ExpGet = 0
	data.ExpId = 0
	data.Candy = 0
	sys.SetBanquetAttr()
}

func gmEnterBanquet(player iface.IPlayer, args ...string) bool {
	banquet := weddingbanquetmgr.GetWBanquetInfo()
	if banquet.Id == 0 {
		return false
	}
	s, ok := player.GetSysObj(sysdef.SiWeddingBanquet).(*WeddingBanquetSys)
	if !ok || !s.IsOpen() {
		return false
	}
	conf := jsondata.GetWeddingBanquetConf()
	data := s.GetData()
	lv := player.GetLevel()
	if data.ExpId == 0 {
		for expId, v := range conf.ExpLimit {
			if lv >= v.LvMin && lv <= v.LvMax {
				data.ExpId = expId
				s.owner.SetExtraAttr(attrdef.WeddingBanquetExpId, int64(data.ExpId))
			}
		}
	}
	if data.ParticipateKey != weddingbanquetmgr.GetWeddingBanquetKey(banquet.Id, banquet.StartTime) {
		data.ParticipateKey = uint64(banquet.Id)
		data.Deliciouts = 0
		player.SetExtraAttr(attrdef.WeddingBanquetDelicious, 0)
	}
	player.EnterFightSrv(base.LocalFightServer, fubendef.EnterWeddingBanquet, &pb3.CommonSt{
		U32Param: banquet.Id,
	})
	return true
}

func init() {
	RegisterSysClass(sysdef.SiWeddingBanquet, func() iface.ISystem {
		return &WeddingBanquetSys{}
	})

	event.RegActorEvent(custom_id.AeNewDay, onBanquetNewDay)

	engine.RegisterActorCallFunc(playerfuncid.GatherBanquetCandy, onGatherBanquetCandy)
	engine.RegisterActorCallFunc(playerfuncid.GatherBanquetDelicious, onGatherBanquetDelicious)
	engine.RegisterActorCallFunc(playerfuncid.BanquetAddExp, onBanquetAddExp)

	net.RegisterSysProtoV2(53, 11, sysdef.SiWeddingBanquet, func(s iface.ISystem) func(*base.Message) error {
		return s.(*WeddingBanquetSys).c2sReserve
	})
	net.RegisterSysProtoV2(53, 12, sysdef.SiWeddingBanquet, func(s iface.ISystem) func(*base.Message) error {
		return s.(*WeddingBanquetSys).c2sReqInvitation
	})
	net.RegisterSysProtoV2(53, 13, sysdef.SiWeddingBanquet, func(s iface.ISystem) func(*base.Message) error {
		return s.(*WeddingBanquetSys).c2sInviteList
	})
	net.RegisterSysProtoV2(53, 14, sysdef.SiWeddingBanquet, func(s iface.ISystem) func(*base.Message) error {
		return s.(*WeddingBanquetSys).c2sHandleInvite
	})
	net.RegisterSysProtoV2(53, 15, sysdef.SiWeddingBanquet, func(s iface.ISystem) func(*base.Message) error {
		return s.(*WeddingBanquetSys).c2sSendInvite
	})
	net.RegisterSysProtoV2(53, 18, sysdef.SiWeddingBanquet, func(s iface.ISystem) func(*base.Message) error {
		return s.(*WeddingBanquetSys).c2sGetReserveInfo
	})
	net.RegisterSysProtoV2(53, 19, sysdef.SiWeddingBanquet, func(s iface.ISystem) func(*base.Message) error {
		return s.(*WeddingBanquetSys).c2sReserveList
	})
	net.RegisterSysProtoV2(53, 21, sysdef.SiWeddingBanquet, func(s iface.ISystem) func(*base.Message) error {
		return s.(*WeddingBanquetSys).c2sBuy
	})
	net.RegisterSysProtoV2(53, 24, sysdef.SiWeddingBanquet, func(s iface.ISystem) func(*base.Message) error {
		return s.(*WeddingBanquetSys).c2sEnterFb
	})
	gmevent.Register("enterBanquet", gmEnterBanquet, 1)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddWeddingBanquetDegree, useItemAddWeddingBanquetDegree)
}
