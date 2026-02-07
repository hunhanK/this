package actorsystem

import (
	"errors"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/activitydef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/functional"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/activity"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"unicode/utf8"

	wordmonitor2 "github.com/gzjjyz/wordmonitor"

	"github.com/gzjjyz/srvlib/utils"
)

type GuildSys struct {
	Base
}

func (sys *GuildSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.GetGuildData() {
		binary.GuildData = &pb3.GuildData{}
	}
}

func (sys *GuildSys) OnLogin() {
	sys.checkGuild()
}

func (sys *GuildSys) OnLogout() {
	if guild := sys.GetGuild(); nil != guild {
		if member := guild.GetMember(sys.owner.GetId()); nil != member {
			member.IsOnline = false
			guild.OnMemberDataBaseChange(manager.GetSimplyData(sys.owner.GetId()))
		}
		return
	}
}

func (sys *GuildSys) checkCoolTime() {
	if cTime := guildmgr.GetExitTimeOff(sys.owner.GetId()); cTime > 0 {
		sys.owner.SetQuitGuildCd(cTime)
		guildmgr.SetExitTimeOff(sys.owner.GetId(), 0)
	}
	sys.owner.SetExtraAttr(attrdef.GuildCoolTime, int64(sys.GetData().GetCoolTime()))
}

func (sys *GuildSys) checkGuild() {
	actorId := sys.owner.GetId()
	if guild := sys.GetGuild(); nil != guild {
		if member := guild.GetMember(actorId); nil != member { //还在原来的仙盟
			sys.owner.SetGuildId(guild.GetId())
			return
		}
	}

	found := false
	for _, newGuild := range guildmgr.GuildMap {
		if member := newGuild.GetMember(actorId); member != nil {
			if !found {
				found = true
				guildData := sys.GetData()
				guildData.GuildInviteList = nil
				sys.owner.SetGuildId(newGuild.GetId()) //离线上线加入行会
			} else {
				//已经有加入,则后面的行会要移除
				newGuild.RemoveMember(actorId)
			}
		}
	}
	if !found && sys.owner.GetGuildId() > 0 {
		oldGuildId := sys.owner.GetGuildId()
		sys.owner.SetGuildId(0)
		sys.owner.TriggerEvent(custom_id.AeLeaveGuild, oldGuildId)
	}

	sys.checkCoolTime()
}

func (sys *GuildSys) OnAfterLogin() {
	if guild := sys.GetGuild(); nil != guild {
		guild.OnLogin(sys.owner)
	} else {
		sys.owner.SendProto3(29, 1, &pb3.S2C_29_1{CoolTime: sys.GetData().GetCoolTime(), GuildRule: guildmgr.GetPfRule()})
	}
	guildmgr.SendSpInviteInfo(sys.owner)
	sys.SendReceiveInviteInfo()
}

func (sys *GuildSys) SendReceiveInviteInfo() {
	data := sys.GetData()
	rsp := &pb3.S2C_29_117{}
	for _, guildId := range data.GuildInviteList {
		if g := guildmgr.GetGuildById(guildId); nil != g {
			rsp.InviteList = append(rsp.InviteList, g.BasicInfo)
		}
	}
	sys.SendProto3(29, 117, rsp)
}

func (sys *GuildSys) OnReconnect() {
	sys.OnAfterLogin()
}

func (sys *GuildSys) GetData() *pb3.GuildData {
	binary := sys.GetBinaryData()
	guildData := binary.GetGuildData()
	return guildData
}

func (sys *GuildSys) GetGuild() *guildmgr.Guild {
	guildData := sys.GetData()
	if guildData.GuildId <= 0 {
		return nil
	}
	return guildmgr.GetGuildById(guildData.GuildId)
}

func (sys *GuildSys) GetGuildBasicById(id uint64) *pb3.GuildBasicInfo {
	if guild := guildmgr.GetGuildById(id); nil != guild {
		return guild.GetBasicInfo()
	}
	return nil
}

func (sys *GuildSys) c2sCreateGuild(msg *base.Message) error {
	var req pb3.C2S_29_2
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	engine.SendWordMonitor(wordmonitor.GuildBasic, wordmonitor.CreateGuild, req.GetName(),
		wordmonitoroption.WithPlayerId(sys.GetOwner().GetId()),
		wordmonitoroption.WithRawData(&req),
		wordmonitoroption.WithCommonData(sys.GetOwner().BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(sys.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
	)
	return err
}

func (sys *GuildSys) CanGuildBannerUse(banner *pb3.GuildBanner) bool {
	if utf8.RuneCountInString(banner.BannerChar) > 1 {
		return false
	}

	if banner.BannerChar == "" {
		return false
	}

	return true
}

func (sys *GuildSys) CanGuildNoticeUse(notice string) bool {
	limit := int(jsondata.GlobalUint("announcementMaxWordage"))
	return utf8.RuneCountInString(notice) <= limit
}

func (sys *GuildSys) CreateGuild(create *pb3.C2S_29_2) (bool, error) {
	conf := jsondata.GetGuildConf()
	if nil == conf || nil == conf.Create {
		return false, neterror.InternalError("guild conf is nil")
	}
	guildName := create.GetName()
	banner := create.GetBanner()
	if nil == banner {
		return false, nil
	}
	coolTime := sys.GetData().GetCoolTime()
	if coolTime > time_util.NowSec() {
		sys.owner.SendTipMsg(tipmsgid.TpJoinGuildCoolCd)
		return false, nil
	}
	if !guildmgr.CheckGuildName(create.Name) {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNameExist)
		return false, nil
	}
	if !sys.CanGuildBannerUse(banner) {
		return false, nil
	}
	if nil != sys.GetGuild() {
		sys.owner.SendTipMsg(tipmsgid.TpGuildHasJoinOther)
		return false, nil
	}

	if sys.owner.GetVipLevel() < conf.Create.CreateNeedVipLv {
		sys.owner.SendTipMsg(tipmsgid.TpVipLvNotEnough)
		return false, nil
	}
	if !sys.owner.ConsumeByConf(conf.Create.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogCreateGuild}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return false, nil
	}
	guild := guildmgr.CreateGuild(sys.owner, guildName, banner)
	if nil == guild {
		return false, nil
	}
	return true, nil
}

func (sys *GuildSys) c2sGuildList(msg *base.Message) error {
	var req pb3.C2S_29_0
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	var res = &pb3.S2C_29_0{
		Infos:     make([]*pb3.GuildListInfo, 0, len(guildmgr.GuildMap)),
		GuildRule: guildmgr.GetPfRule(),
	}

	for _, guild := range guildmgr.GuildMap {
		res.Infos = append(res.Infos, &pb3.GuildListInfo{
			Basic:   guild.BasicInfo,
			IsApply: guild.IsApply(sys.owner.GetId()),
		})
	}
	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeGuild)
	conf := jsondata.GetRankConf(gshare.RankTypeGuild)
	if nil != conf {
		rankLine := rank.GetList(1, int(conf.ShowMaxLimit))
		res.RankInfo = rankLine
	}
	sys.SendProto3(29, 0, res)
	return nil
}

func (sys *GuildSys) c2sGuildApply(msg *base.Message) error {
	var req pb3.C2S_29_3
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guildData := sys.GetData()
	lv := sys.owner.GetLevel()
	if !guildmgr.CheckApplyLv(lv) {
		sys.owner.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}
	coolTime := guildData.GetCoolTime()
	if coolTime > time_util.NowSec() {
		sys.owner.SendTipMsg(tipmsgid.TpJoinGuildCoolCd)
		return nil
	}

	guild := guildmgr.GetGuildById(req.GetId())
	if nil == guild {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotExist)
		return nil
	}

	guild.ApplyJoin(sys.owner)
	return nil
}

func (sys *GuildSys) c2sGuildCancel(msg *base.Message) error {
	var req pb3.C2S_29_4
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := guildmgr.GetGuildById(req.GetId())
	if nil != guild {
		if guild.RemoveApply(sys.owner.GetId()) {
			sys.SendProto3(29, 4, &pb3.S2C_29_4{
				GuildId: guild.GetId(),
				ActorId: sys.owner.GetId(),
			})
		}
	}

	return nil
}

func (sys *GuildSys) c2sApplyList(msg *base.Message) error {
	var req pb3.C2S_29_5
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := sys.GetGuild()
	if nil == guild {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotContainSelf)
		return nil
	}

	myMember := guild.GetMember(sys.owner.GetId())
	if nil == myMember {
		return neterror.ParamsInvalidError("cant self(%d) find in guild(%d)", sys.GetOwner().GetId(), guild.GetId())
	}

	ApplyIds := guild.GetBinary().ApplyIds
	if !guild.CheckPermission(myMember, custom_id.GuildPermission_CanManageRequest) {
		sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
		return nil
	}

	rsp := &pb3.S2C_29_5{ApplyList: make([]*pb3.GuildApplyInfo, 0, len(ApplyIds))}
	for actorId, applyTime := range ApplyIds {
		if data := manager.GetSimplyData(actorId); nil != data {
			rsp.ApplyList = append(rsp.ApplyList, &pb3.GuildApplyInfo{
				PlayerInfo: data,
				ApplyTime:  applyTime,
			})
		}
	}
	sys.SendProto3(29, 5, rsp)
	return nil
}

func (sys *GuildSys) c2sReplyApply(msg *base.Message) error {
	var req pb3.C2S_29_6
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	if guild := sys.GetGuild(); nil != guild {
		member := guild.GetMember(sys.owner.GetId())
		if nil == member {
			sys.owner.SendTipMsg(tipmsgid.TpGuildNotContainSelf)
			return nil
		}
		if !guild.CheckPermission(member, custom_id.GuildPermission_CanManageRequest) {
			sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
			return nil
		}

		if req.GetActorId() == 0 {
			for applyId := range guild.Binary.ApplyIds {
				if guild.IsFull() {
					sys.owner.SendTipMsg(tipmsgid.TpGuildMemberMax)
					return nil
				}
				guild.ReplyApply(sys.owner, applyId, req.GetOp())
			}
			//审批全部
		} else {
			guild.ReplyApply(sys.owner, req.GetActorId(), req.GetOp())
		}

	}
	return nil
}

func (sys *GuildSys) c2sGuildExit(msg *base.Message) error {
	var req pb3.C2S_29_8
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := sys.GetGuild()
	if nil == guild {
		return nil
	}

	member := guild.GetMember(sys.owner.GetId())
	if nil == member {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotContainSelf)
		return nil
	}

	pos := member.GetPosition()
	if pos == custom_id.GuildPos_Leader { //盟主需要先移交仙盟
		sys.owner.SendTipMsg(tipmsgid.TpGuildLeaderCantExit)
		return nil
	}

	// 在仙盟战-昆仑秘境, 仙盟宴会中不能退出仙盟
	if sys.inActivity() {
		sys.owner.SendTipMsg(tipmsgid.TpCannotQuitInGuildSecretFb)
		return nil
	}

	if guild.RemoveMember(sys.owner.GetId()) {
		guild.AddEvent(custom_id.GuildEvent_LeaveGuild, member.GetPlayerInfo().GetName())
		sys.owner.SetQuitGuildCd(time_util.NowSec())
	}
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogLeaveGuild, &pb3.LogPlayerCounter{
		NumArgs: guild.GetId(),
	})
	return nil
}

func (sys *GuildSys) c2sGuildPreview(msg *base.Message) error {
	var req pb3.C2S_29_9
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := guildmgr.GetGuildById(req.GetGuildId())
	if nil == guild {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotExist)
		return nil
	}
	sys.SendProto3(29, 9, &pb3.S2C_29_9{
		GuildId:    guild.GetId(),
		Lv:         guild.GetLevel(),
		Name:       guild.GetName(),
		MemberList: functional.MapToSlice(guild.GetMembers()),
	})
	return nil
}

func (sys *GuildSys) c2sPrefixNameUse(msg *base.Message) error {
	return nil
}

func (sys *GuildSys) c2sSetApplyMode(msg *base.Message) error {
	var req pb3.C2S_29_100
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if guild := sys.GetGuild(); nil != guild {
		if !guild.CheckPermission(guild.GetMember(sys.owner.GetId()), custom_id.GuildPermission_CanAutoAgree) {
			sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
			return nil
		}
		if req.GetMode() == custom_id.GuildApplyMode_Cond {
			lvMin, lvMax := guildmgr.GetLevelLimit()
			powerMin, powerMax := guildmgr.GetPowerLimit()
			if lvMin > req.GetLevel() || lvMax < req.GetLevel() {
				return neterror.ParamsInvalidError("mode auto accept mode lv exceed")
			}
			if powerMin > req.GetPower() || powerMax < req.GetPower() {
				return neterror.ParamsInvalidError("mode auto accept mode power exceed")
			}
		}
		guild.SetMode(req.GetMode(), req.GetLevel(), req.GetPower())
		guild.SetProp(custom_id.GuildPropChangeSet, true)
	}
	return nil
}

func (sys *GuildSys) c2sRemoveMember(msg *base.Message) error {
	var req pb3.C2S_29_101
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := sys.GetGuild()
	member, tMember := guild.GetMember(sys.owner.GetId()), guild.GetMember(req.GetActorId())
	if nil == member || nil == tMember {
		sys.owner.SendTipMsg(tipmsgid.TpGuildPlayerIsntMember)
		return nil
	}

	if sys.inActivity() {
		sys.owner.SendTipMsg(tipmsgid.TpCannotQuitInGuildSecretFb)
		return nil
	}

	if req.GetActorId() == sys.owner.GetId() {
		return nil
	}

	if !guild.CheckPermission(guild.GetMember(sys.owner.GetId()), custom_id.GuildPermission_KickMember, tMember.GetPosition()) {
		sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
		return nil
	}

	if guild.RemoveMember(req.GetActorId()) {
		guild.AddEvent(custom_id.GuildEvent_KickMember, member.GetPlayerInfo().GetName(), tMember.GetPlayerInfo().GetName())
		if player := manager.GetPlayerPtrById(req.GetActorId()); nil != player {
			player.SetQuitGuildCd(time_util.NowSec())
			guildData := player.GetBinaryData().GetGuildData()
			player.SendProto3(29, 101, &pb3.S2C_29_101{CoolTime: guildData.GetCoolTime()})
		} else {
			guildmgr.SetExitTimeOff(req.GetActorId(), time_util.NowSec())
		}
		mailmgr.SendMailToActor(req.GetActorId(), &mailargs.SendMailSt{ConfId: common.Mail_GuildKick})
		logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogKickMember, &pb3.LogPlayerCounter{
			NumArgs: guild.GetId(),
			StrArgs: utils.Itoa(req.GetActorId()),
		})
	}
	return nil
}

func (sys *GuildSys) c2sTransferLeader(msg *base.Message) error {
	var req pb3.C2S_29_103
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := sys.GetGuild()
	if nil == guild {
		return nil
	}
	member, tMember := guild.GetMember(sys.owner.GetId()), guild.GetMember(req.GetActorId())
	if nil == member || nil == tMember {
		sys.owner.SendTipMsg(tipmsgid.TpGuildPlayerIsntMember)
		return nil
	}

	if sys.owner.GetId() != guild.GetLeaderId() {
		sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
		return nil
	}

	guild.SetGuildPos(member, custom_id.GuildPos_Common)
	guild.SetGuildPos(tMember, custom_id.GuildPos_Leader)

	// 保存
	guildmgr.SetSaveFlag(guild.GetId())

	if !engine.IsRobot(req.GetActorId()) {
		mailmgr.SendMailToActor(req.GetActorId(), &mailargs.SendMailSt{ConfId: common.Mail_GuildTransferLeader, Content: &mailargs.PlayerNameArgs{Name: sys.owner.GetName()}})
	}

	guild.AddEvent(custom_id.GuildEvent_Commission, sys.owner.GetName(), tMember.GetPlayerInfo().GetName(), tMember.GetPosition())
	return nil
}

func (sys *GuildSys) c2sChangeGuildName(msg *base.Message) error {
	var req pb3.C2S_29_105
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	engine.SendWordMonitor(wordmonitor.GuildName, wordmonitor.ChangeGuildName, req.GetName(),
		wordmonitoroption.WithPlayerId(sys.GetOwner().GetId()),
		wordmonitoroption.WithRawData(&req),
		wordmonitoroption.WithCommonData(sys.GetOwner().BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(sys.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
	)
	return nil
}

func (sys *GuildSys) onChangeGuildName(req *pb3.C2S_29_105) error {
	if guild := sys.GetGuild(); nil != guild {
		if !guild.CheckPermission(guild.GetMember(sys.owner.GetId()), custom_id.GuildPermission_CanModifyGuildName) {
			sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
			return nil
		}
		guildConf := jsondata.GetGuildConf()
		if nil == guildConf {
			return neterror.ParamsInvalidError("guild conf is nil")
		}
		if !guildmgr.CheckGuildName(req.Name) {
			sys.owner.SendTipMsg(tipmsgid.TpGuildNameExist)
			return nil
		}
		name := req.GetName()
		nameLen := utf8.RuneCountInString(name)
		lenLimit := jsondata.GetCommonConf("guildNameLimit").U32
		if nameLen > int(lenLimit) || nameLen <= 0 {
			sys.owner.SendTipMsg(tipmsgid.TpGuildNameLenLimit)
			return nil
		}
		if !engine.CheckNameSpecialCharacter(name) {
			sys.owner.SendTipMsg(tipmsgid.TpGuildNameLenLimit)
			return nil
		}
		if name == guild.GetName() { //重名不用改
			return nil
		}
		if !sys.owner.ConsumeByConf(guildConf.RenameConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogGuildRename}) {
			sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
		if req.IsCustom && !sys.owner.ConsumeByConf(guildConf.CustomNameConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogGuildCustomName}) {
			sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
		guild.OnUpdateGuildName(name, utils.Make64(req.GetPrefix(), req.GetSuffix()))
	}
	return nil
}

func (sys *GuildSys) c2sChangeGuildBanner(msg *base.Message) error {
	var req pb3.C2S_29_106
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	engine.SendWordMonitor(wordmonitor.GuildBanner, wordmonitor.ChangeGuildBanner, req.GetBanner().GetBannerChar(),
		wordmonitoroption.WithPlayerId(sys.GetOwner().GetId()),
		wordmonitoroption.WithRawData(&req),
		wordmonitoroption.WithCommonData(sys.GetOwner().BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(sys.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
	)
	return nil
}

func (sys *GuildSys) onChangeGuildBanner(req *pb3.C2S_29_106) error {
	if guild := sys.GetGuild(); nil != guild {
		if !guild.CheckPermission(guild.GetMember(sys.owner.GetId()), custom_id.GuildPermission_CanModifyGuildFlag) {
			sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
			return nil
		}
		if !sys.CanGuildBannerUse(req.GetBanner()) {
			return nil
		}
		guildConf := jsondata.GetGuildConf()
		if nil == guildConf {
			return neterror.ParamsInvalidError("guild conf is nil")
		}
		if !sys.owner.ConsumeByConf(guildConf.ReBannerConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogGuildReBanner}) {
			sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
		guild.Binary.Banner = req.GetBanner()
		guild.BasicInfo.Banner = guild.Binary.Banner

		guild.BroadcastProto(29, 106, &pb3.S2C_29_106{GuildId: guild.GetId(), Banner: guild.Binary.GetBanner()})

	}
	return nil
}

func (sys *GuildSys) c2sSetGuildPos(msg *base.Message) error {
	var req pb3.C2S_29_107
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := sys.GetGuild()
	if nil == guild {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotContainSelf)
		return nil
	}
	if req.GetActorId() == sys.owner.GetId() {
		return nil
	}
	member, tMember := guild.GetMember(sys.owner.GetId()), guild.GetMember(req.GetActorId())
	if nil == member || nil == tMember {
		sys.owner.SendTipMsg(tipmsgid.TpGuildPlayerIsntMember)
		return nil
	}

	if engine.IsRobot(req.GetActorId()) && req.GetPosition() != custom_id.GuildPos_Common {
		sys.owner.SendTipMsg(tipmsgid.GuildPosSetError)
		return nil
	}

	if !guild.CheckPermission(member, custom_id.GuildPermission_SetPosition, req.GetPosition()) {
		sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
		return nil
	}
	guildConf := jsondata.GetGuildConf()
	if nil == guildConf {
		return neterror.ParamsInvalidError("guild conf is nil")
	}
	if guild.IsPositionFull(req.GetPosition()) {
		sys.owner.SendTipMsg(tipmsgid.TpGuildPositonIsFull)
		return nil
	}
	if req.GetPosition() == custom_id.GuildPos_Leader {
		sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
		return nil
	}

	if guild.SetGuildPos(tMember, req.GetPosition()) {
		guild.AddEvent(custom_id.GuildEvent_Commission, sys.owner.GetName(), tMember.GetPlayerInfo().GetName(), tMember.GetPosition())
	}
	sys.SendProto3(29, 107, &pb3.S2C_29_107{
		ActorId:  req.GetActorId(),
		Position: tMember.GetPosition(),
	})
	return nil
}

func (sys *GuildSys) c2sIssueGuildRecruit(msg *base.Message) error {
	cd := jsondata.GlobalUint("guildRecruitCd")
	guild := sys.GetGuild()
	if nil == guild {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotContainSelf)
		return nil
	}
	member := guild.GetMember(sys.owner.GetId())
	if !guild.CheckPermission(member, custom_id.Guildpermission_Caninvite) {
		sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
		return nil
	}
	nowSec := time_util.NowSec()
	if guild.Binary.RecruitBroadcastTime > nowSec {
		sys.owner.SendTipMsg(tipmsgid.TpChatCd, guild.Binary.RecruitBroadcastTime-nowSec)
		return nil
	}
	guild.Binary.RecruitBroadcastTime = nowSec + cd

	engine.BroadcastTipMsgById(tipmsgid.TpGuildCreateInviteMsg, sys.owner.GetId(), guild.GetId(), guild.GetLevel(), guild.GetName())

	return nil
}

func (sys *GuildSys) c2sGuildDonate(msg *base.Message) error {
	var req pb3.C2S_29_109
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := sys.GetGuild()
	if nil == guild {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotContainSelf)
		return nil
	}
	guildConf := jsondata.GetGuildConf()
	if nil == guildConf {
		return neterror.ParamsInvalidError("guild conf is nil")
	}
	if nil == guildConf.Donate || nil == guildConf.Donate[req.GetType()] {
		return neterror.ParamsInvalidError("donate conf(%d) is nil", req.GetType())
	}
	donateConf := guildConf.Donate[req.GetType()]
	guildData := sys.GetData()
	var dt *pb3.KeyValue
	for _, donate := range guildData.DonateList {
		if donate.Key == req.GetType() {
			if donate.Value >= donateConf.Count {
				sys.owner.SendTipMsg(tipmsgid.TpTodayIsLimit)
				return nil
			}
			dt = donate
		}
	}

	if !sys.owner.ConsumeByConf(donateConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogGuildDonate}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	if nil == dt {
		dt = &pb3.KeyValue{
			Key:   req.GetType(),
			Value: 0,
		}
		guildData.DonateList = append(guildData.DonateList, dt)
	}
	dt.Value++
	sys.SendProto3(29, 109, &pb3.S2C_29_109{Donate: dt})
	engine.GiveRewards(sys.owner, donateConf.Money, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGuildDonate})
	//贡献货币
	engine.GiveRewards(sys.owner, donateConf.DonatePoint, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGuildDonate})
	for _, line := range donateConf.Consume {
		guild.AddEvent(custom_id.GuildEvent_Donate, sys.owner.GetName(), line.Id, line.Count)
	}
	sys.owner.TriggerQuestEvent(custom_id.QttGuildDonateTimes, 0, 1)
	return nil
}

func (sys *GuildSys) c2sSendGuildInvite(msg *base.Message) error {
	var req pb3.C2S_29_114
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	inviteId := req.GetActorId()
	if inviteId == sys.owner.GetId() {
		return nil
	}
	if playerData, ok := manager.GetData(inviteId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		lv := playerData.GetLv()
		if !guildmgr.CheckApplyLv(lv) {
			sys.owner.SendTipMsg(tipmsgid.TpPlayerNotUnlockGuild)
			return nil
		}
	}
	if guild := sys.GetGuild(); nil != guild {
		if !guild.CheckPermission(guild.GetMember(sys.owner.GetId()), custom_id.Guildpermission_Caninvite) {
			sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
			return nil
		}
		if player := manager.GetPlayerPtrById(inviteId); nil != player {
			if player.GetGuildId() > 0 {
				player.SendProto3(29, 114, &pb3.S2C_29_114{Basic: guild.GetBasicInfo()})
			}
			guildSys, ok := player.GetSysObj(sysdef.SiGuild).(*GuildSys)
			if !ok || !guildSys.IsOpen() {
				return nil
			}
			if nil != guildSys.GetGuild() {
				sys.owner.SendTipMsg(tipmsgid.TpGuildHasJoinOther)
				return nil
			}
			guildData := guildSys.GetData()
			if !utils.SliceContainsUint64(guildData.GuildInviteList, guild.GetId()) {
				guildData.GuildInviteList = append(guildData.GuildInviteList, guild.GetId())
				player.SendProto3(29, 114, &pb3.S2C_29_114{Basic: guild.GetBasicInfo()})
				player.SendProto3(29, 135, &pb3.S2C_29_135{ //下发邀请函
					InviteName: sys.owner.GetName(),
					GuildName:  guild.GetName(),
					GuildId:    guild.GetId(),
				})
			}
		} else {
			engine.SendPlayerMessage(req.GetActorId(), gshare.OfflineInviteGuild, &pb3.OfflineGuildInvite{Guild: guild.GetId()})
		}
	}
	return nil
}

func (sys *GuildSys) c2sAcceptGuildInvite(msg *base.Message) error {
	var req pb3.C2S_29_115
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guildData := sys.GetData()
	coolTime := guildData.GetCoolTime()
	if coolTime > time_util.NowSec() {
		sys.owner.SendTipMsg(tipmsgid.TpJoinGuildCoolCd)
		return nil
	}
	guild := guildmgr.GetGuildById(req.GetGuildId())
	if nil == guild {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotExist)
		return nil
	}
	if guild.IsFull() {
		sys.owner.SendTipMsg(tipmsgid.TpGuildMemberMax)
		return nil
	}
	if nil != sys.GetGuild() {
		sys.owner.SendTipMsg(tipmsgid.TpGuildHasJoinOther)
		return nil
	}

	if !utils.SliceContainsUint64(guildData.GuildInviteList, guild.GetId()) {
		return neterror.ParamsInvalidError("not receive guild(%d) invite", req.GetGuildId())
	}
	guildmgr.GuildAddMember(guild, manager.GetSimplyData(sys.owner.GetId()), custom_id.GuildPos_Common)
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogJoinGuild, &pb3.LogPlayerCounter{
		NumArgs: guild.GetId(),
	})
	return nil
}

func (sys *GuildSys) c2sChangeGuildNotice(msg *base.Message) error {
	var req pb3.C2S_29_116
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	engine.SendWordMonitor(wordmonitor.GuildNotice, wordmonitor.ChangeGuildNotice, req.GetNotice(),
		wordmonitoroption.WithPlayerId(sys.GetOwner().GetId()),
		wordmonitoroption.WithRawData(&req),
		wordmonitoroption.WithCommonData(sys.GetOwner().BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(sys.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
	)
	return nil
}

func (sys *GuildSys) onChangeGuildNotice(req *pb3.C2S_29_116) error {
	if guild := sys.GetGuild(); nil != guild {
		if guild.Binary.NoticeFlag { //禁止修改公告
			return nil
		}
		if !guild.CheckPermission(guild.GetMember(sys.owner.GetId()), custom_id.GuildPermission_CanModifyAnnoucement) {
			sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
			return nil
		}
		if !sys.CanGuildNoticeUse(req.GetNotice()) {
			return nil
		}
		guild.BasicInfo.Notice = req.GetNotice()
		guild.BroadcastProto(29, 116, &pb3.S2C_29_116{Notice: req.GetNotice(), ActorId: sys.owner.GetId()})
	}
	return nil
}

func (sys *GuildSys) c2sMember(msg *base.Message) error {
	if guild := sys.GetGuild(); nil != guild {
		sys.SendProto3(29, 120, &pb3.S2C_29_120{Members: functional.MapToSlice(guild.GetMembers())})
	}
	return nil
}

func (sys *GuildSys) c2sDonateToDepot(msg *base.Message) error {
	var req pb3.C2S_29_52
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := sys.GetGuild()
	if nil == guild {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotExist)
		return nil
	}
	handles := req.GetHandles()
	for _, handle := range handles {
		guild.Donate(sys.owner, handle)
	}
	return nil
}

func (sys *GuildSys) c2sExchangeDepot(msg *base.Message) error {
	var req pb3.C2S_29_53
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := sys.GetGuild()
	if nil == guild {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotExist)
		return nil
	}
	guild.Exchange(sys.owner, req.GetHandle())
	sys.owner.TriggerQuestEvent(custom_id.QttAchievementsGuildExchangeDepot, 0, 1)
	return nil
}

func (sys *GuildSys) c2sDepotDestroy(msg *base.Message) error {
	var req pb3.C2S_29_54
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	guild := sys.GetGuild()
	if nil == guild {
		sys.owner.SendTipMsg(tipmsgid.TpGuildNotExist)
		return nil
	}
	if !guild.CheckPermission(guild.GetMember(sys.owner.GetId()), custom_id.GuildPermission_CanManageStorage) {
		sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
		return nil
	}
	handles := req.GetHandle()
	for _, handle := range handles {
		guild.DestroyDepotItem(sys.owner, handle)
	}
	return nil
}

func (sys *GuildSys) c2sGuildDismiss(msg *base.Message) error {
	if guild := sys.GetGuild(); nil != guild {
		if guild.GetLeaderId() != sys.owner.GetId() {
			sys.owner.SendTipMsg(tipmsgid.TpPermissionNotEnough)
			return nil
		}
		if sys.inActivity() {
			sys.owner.SendTipMsg(tipmsgid.TpCannotQuitInGuildSecretFb)
			return nil
		}
		for memberId := range guild.Members {
			if memberId == sys.owner.GetId() {
				continue
			}
			if !engine.IsRobot(memberId) {
				return neterror.ParamsInvalidError("member has real player")
			}
		}
		guildId := guild.GetId()
		guildmgr.DelGuild(guildId)
		sys.owner.SetGuildId(0)
		sys.owner.SetQuitGuildCd(time_util.NowSec())
		sys.SendProto3(29, 104, &pb3.S2C_29_104{GuildId: guildId})
	}
	return nil
}

func (sys *GuildSys) onNewDay() {
	guildData := sys.GetData()
	guildData.DonateList = nil
	sys.owner.SendProto3(29, 110, &pb3.S2C_29_110{Donates: guildData.DonateList})
}

func (sys *GuildSys) inActivity() bool {
	actIds := []uint32{activitydef.ActGuildParty, activitydef.ActGuildSecret}

	for _, id := range actIds {
		if activity.GetActStatus(id) == activitydef.ActStart {
			return true
		}
	}

	return false
}

func offlineGuildInvite(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.OfflineGuildInvite)
	if !ok {
		return
	}
	guildSys, ok := player.GetSysObj(sysdef.SiGuild).(*GuildSys)
	if !ok || !guildSys.IsOpen() {
		return
	}
	if !guildmgr.CheckApplyLv(player.GetLevel()) {
		return
	}
	if player.GetGuildId() == 0 {
		data := guildSys.GetData()
		g := guildmgr.GetGuildById(st.GetGuild())
		if nil == g {
			return
		}
		if !utils.SliceContainsUint64(data.GuildInviteList, st.GetGuild()) {
			data.GuildInviteList = append(data.GuildInviteList, st.GetGuild())
			player.SendProto3(29, 114, &pb3.S2C_29_114{Basic: g.GetBasicInfo()})
		}
	}

}

func offlineJoinGuild(player iface.IPlayer, msg pb3.Message) {
	guildSys, ok := player.GetSysObj(sysdef.SiGuild).(*GuildSys)
	if !ok || !guildSys.IsOpen() {
		return
	}
	player.TriggerQuestEvent(custom_id.QttJoinGuildTimes, 0, 1)

}

func onDataBaseChange(actor iface.IPlayer, args ...interface{}) {
	if sys, ok := actor.GetSysObj(sysdef.SiGuild).(*GuildSys); ok && sys.IsOpen() {
		if guild := sys.GetGuild(); nil != guild {
			guild.OnMemberDataBaseChange(manager.GetSimplyData(actor.GetId()))
		}
	}
}

func checkGuildTransfer(actor iface.IPlayer, args ...interface{}) {
	if sys, ok := actor.GetSysObj(sysdef.SiGuild).(*GuildSys); ok && sys.IsOpen() {
		if guild := sys.GetGuild(); nil != guild {
			guild.CheckGuildTransfer(actor)
		}
	}
}

func onCheckGmGuildInvite(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 2 {
		return
	}
	oldLv, ok := args[0].(uint32)
	if !ok {
		return
	}
	newLv, ok := args[1].(uint32)
	if !ok {
		return
	}
	sys, ok := actor.GetSysObj(sysdef.SiGuild).(*GuildSys)
	if !ok || !sys.IsOpen() {
		return
	}
	conf := jsondata.GetGuildConf()
	if oldLv < conf.GMInviteLevel && newLv >= conf.GMInviteLevel {
		guildmgr.SendSpInviteInfo(actor)
	}
}

func onMoneyChange(player iface.IPlayer, args ...interface{}) {
	mt, ok := args[0].(uint32)
	if !ok || mt != moneydef.GuildDonate {
		return
	}
	count, ok := args[1].(int64)
	if !ok || count <= 0 {
		return
	}
	if sys, ok := player.GetSysObj(sysdef.SiGuild).(*GuildSys); ok && sys.IsOpen() {
		if guild := sys.GetGuild(); nil != guild {
			guild.AddPersonDonate(player.GetId(), count)
		}
	}
}

func onCreateGuildMonitorRet(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}
	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.TpSensitiveWord)
		return nil
	}
	req, ok := word.Data.(*pb3.C2S_29_2)
	if !ok {
		return errors.New("not *pb3.C2S_29_2")
	}
	if sys, ok := player.GetSysObj(sysdef.SiGuild).(*GuildSys); ok && sys.IsOpen() {
		res, err := sys.CreateGuild(req)
		sys.SendProto3(29, 2, &pb3.S2C_29_2{Success: res})
		return err
	}
	return nil
}

func onChangeGuildNameMonitorRet(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}
	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.TpSensitiveWord)
		return nil
	}
	req, ok := word.Data.(*pb3.C2S_29_105)
	if !ok {
		return errors.New("not *pb3.C2S_29_105")
	}
	if sys, ok := player.GetSysObj(sysdef.SiGuild).(*GuildSys); ok && sys.IsOpen() {
		return sys.onChangeGuildName(req)
	}
	return nil
}

func onChangeGuildBannerMonitorRet(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}
	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.TpSensitiveWord)
		return nil
	}
	req, ok := word.Data.(*pb3.C2S_29_106)
	if !ok {
		return errors.New("not *pb3.C2S_29_106")
	}
	if sys, ok := player.GetSysObj(sysdef.SiGuild).(*GuildSys); ok && sys.IsOpen() {
		return sys.onChangeGuildBanner(req)
	}
	return nil
}

func onChangeGuildNoticeMonitorRet(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}
	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.TpSensitiveWord)
		return nil
	}
	req, ok := word.Data.(*pb3.C2S_29_116)
	if !ok {
		return errors.New("not *pb3.C2S_29_116")
	}
	if sys, ok := player.GetSysObj(sysdef.SiGuild).(*GuildSys); ok && sys.IsOpen() {
		return sys.onChangeGuildNotice(req)
	}
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiGuild, func() iface.ISystem {
		return &GuildSys{}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiGuild).(*GuildSys); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})

	event.RegActorEvent(custom_id.AeJoinGuild, func(player iface.IPlayer, args ...interface{}) {
		player.TriggerQuestEvent(custom_id.QttJoinGuild, 0, 1)
	})

	engine.RegisterMessage(gshare.OfflineInviteGuild, func() pb3.Message {
		return &pb3.OfflineGuildInvite{}
	}, offlineGuildInvite)

	engine.RegisterMessage(gshare.OfflineJoinGuild, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineJoinGuild)

	engine.RegQuestTargetProgress(custom_id.QttJoinGuild, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		sys, ok := actor.GetSysObj(sysdef.SiGuild).(*GuildSys)
		if !ok || !sys.IsOpen() {
			return 0
		}
		if nil != sys.GetGuild() {
			return 1
		}
		return 0
	})

	event.RegActorEvent(custom_id.AeChangeName, onDataBaseChange)
	event.RegActorEvent(custom_id.AeLevelUp, onDataBaseChange)
	event.RegActorEvent(custom_id.AeLevelUp, onCheckGmGuildInvite)
	event.RegActorEvent(custom_id.AeLevelDown, onDataBaseChange)
	event.RegActorEvent(custom_id.AeCircleChange, onDataBaseChange)
	event.RegActorEvent(custom_id.AeVipLevelUp, onDataBaseChange)
	event.RegActorEvent(custom_id.AeMoneyChange, onMoneyChange)
	event.RegActorEvent(custom_id.AeFightValueChange, onDataBaseChange)
	event.RegActorEvent(custom_id.AeFightValueChange, checkGuildTransfer)
	event.RegActorEvent(custom_id.AeFlyCampChange, onDataBaseChange)

	//仙盟基础
	net.RegisterSysProto(29, 0, sysdef.SiGuild, (*GuildSys).c2sGuildList)
	net.RegisterSysProto(29, 2, sysdef.SiGuild, (*GuildSys).c2sCreateGuild)
	net.RegisterSysProto(29, 3, sysdef.SiGuild, (*GuildSys).c2sGuildApply)
	net.RegisterSysProto(29, 4, sysdef.SiGuild, (*GuildSys).c2sGuildCancel)
	net.RegisterSysProto(29, 5, sysdef.SiGuild, (*GuildSys).c2sApplyList)
	net.RegisterSysProto(29, 6, sysdef.SiGuild, (*GuildSys).c2sReplyApply)
	net.RegisterSysProto(29, 8, sysdef.SiGuild, (*GuildSys).c2sGuildExit)
	net.RegisterSysProto(29, 9, sysdef.SiGuild, (*GuildSys).c2sGuildPreview)
	net.RegisterSysProto(29, 11, sysdef.SiGuild, (*GuildSys).c2sPrefixNameUse)

	net.RegisterSysProto(29, 100, sysdef.SiGuild, (*GuildSys).c2sSetApplyMode)
	net.RegisterSysProto(29, 101, sysdef.SiGuild, (*GuildSys).c2sRemoveMember)
	net.RegisterSysProto(29, 103, sysdef.SiGuild, (*GuildSys).c2sTransferLeader)
	net.RegisterSysProto(29, 104, sysdef.SiGuild, (*GuildSys).c2sGuildDismiss)
	net.RegisterSysProto(29, 105, sysdef.SiGuild, (*GuildSys).c2sChangeGuildName)
	net.RegisterSysProto(29, 106, sysdef.SiGuild, (*GuildSys).c2sChangeGuildBanner)
	net.RegisterSysProto(29, 107, sysdef.SiGuild, (*GuildSys).c2sSetGuildPos)
	net.RegisterSysProto(29, 108, sysdef.SiGuild, (*GuildSys).c2sIssueGuildRecruit)
	net.RegisterSysProto(29, 109, sysdef.SiGuild, (*GuildSys).c2sGuildDonate)
	net.RegisterSysProto(29, 114, sysdef.SiGuild, (*GuildSys).c2sSendGuildInvite)
	net.RegisterSysProto(29, 115, sysdef.SiGuild, (*GuildSys).c2sAcceptGuildInvite)
	net.RegisterSysProto(29, 116, sysdef.SiGuild, (*GuildSys).c2sChangeGuildNotice)
	net.RegisterSysProto(29, 120, sysdef.SiGuild, (*GuildSys).c2sMember)

	//仙盟仓库
	net.RegisterSysProto(29, 52, sysdef.SiGuild, (*GuildSys).c2sDonateToDepot)
	net.RegisterSysProto(29, 53, sysdef.SiGuild, (*GuildSys).c2sExchangeDepot)
	net.RegisterSysProto(29, 54, sysdef.SiGuild, (*GuildSys).c2sDepotDestroy)

	engine.RegWordMonitorOpCodeHandler(wordmonitor.CreateGuild, onCreateGuildMonitorRet)
	engine.RegWordMonitorOpCodeHandler(wordmonitor.ChangeGuildName, onChangeGuildNameMonitorRet)
	engine.RegWordMonitorOpCodeHandler(wordmonitor.ChangeGuildBanner, onChangeGuildBannerMonitorRet)
	engine.RegWordMonitorOpCodeHandler(wordmonitor.ChangeGuildNotice, onChangeGuildNoticeMonitorRet)
}
