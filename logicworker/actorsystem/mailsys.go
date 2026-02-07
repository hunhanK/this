package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"

	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
)

const (
	MaxPrcessCountForGetAllReward = 200
)

type MailSys struct {
	Base

	loadFlag     bool
	loadAllTime  uint32 //加载全部邮件过程时间,在这个时间内禁止操作邮件
	serverMailId uint64 //上次接收到的全服邮件索引
	isLoaded     bool
	mailMap      map[uint64]*pb3.MailInfo //当前邮件列表
}

func (sys *MailSys) OnInit() {
	sys.mailMap = make(map[uint64]*pb3.MailInfo)
	sys.loadFlag = false
	sys.serverMailId = 0

	sys.SendDbLoadMail(0)
	sys.LoadActorServerMail()
}

func (sys *MailSys) IsOpen() bool {
	return true
}

func (sys *MailSys) OnReconnect() {
	sys.s2cMails()
}

func (sys *MailSys) OnServerMailIndexLoaded(index uint64) {
	sys.isLoaded = true
	sys.serverMailId = index
	sys.OnServerMailLoaded()
}

func (sys *MailSys) OnServerMailLoaded() {
	if !sys.isLoaded {
		return
	}
	actorId, oldIndex := sys.owner.GetId(), sys.serverMailId
	createTime := sys.owner.GetCreateTime()

	list := mailmgr.GetServerMailList()
	count := len(list)
	for i := count - 1; i >= 0; i-- {
		mail := list[i]
		if mail.Id <= oldIndex {
			break
		}
		if sys.serverMailId < mail.Id {
			sys.serverMailId = mail.Id
		}
		if createTime > mail.SendTick {
			continue
		}

		sendSt := &mailargs.SendMailStrSt{
			ConfId:  mail.ConfId,
			Reward:  mail.AwardStr,
			Title:   mail.Title,
			Content: mail.Content,
		}
		mailmgr.SendFileMailToActorStr(actorId, sendSt)

	}
	if oldIndex != sys.serverMailId {
		sys.updateActorServerMail()
	}
}

func (sys *MailSys) updateActorServerMail() {
	gshare.SendDBMsg(custom_id.GMsgUpdateActorServerMail, sys.owner.GetId(), sys.serverMailId)
}

// SendDbLoadMail 请求加载邮件
func (sys *MailSys) SendDbLoadMail(id uint64) {
	if id == 0 {
		now := time_util.NowSec()
		if now < sys.loadAllTime {
			return
		}
		sys.loadAllTime = now + 120
	}
	gshare.SendDBMsg(custom_id.GMsgLoadActorMail, sys.owner.GetId(), id)
}

// LoadActorServerMail 加载角色群发邮件
func (sys *MailSys) LoadActorServerMail() {
	gshare.SendDBMsg(custom_id.GMsgLoadActorServerMail, sys.owner.GetId())
}

// OnLoadFromDb 等前端做的时候要把这个MailInfo结构改掉
func (sys *MailSys) OnLoadFromDb(loadId uint64, list []*gshare.ActorMailSt) {
	if loadId == 0 {
		sys.loadAllTime = 0
	}
	for _, mail := range list {
		if _, ok := sys.mailMap[mail.MailId]; ok {
			continue
		}
		info := &pb3.MailInfo{
			MailId:     mail.MailId,
			ConfId:     uint32(mail.ConfId),
			Status:     mail.Status,
			CreateTime: mail.SendTick,
			Files:      mail.AwardStr,
		}

		if info.ConfId == 0 {
			info.Title = mail.Title
			info.Content = mail.Content
		} else {
			info.Args = mail.Content
		}

		if len(mail.UserItem) > 0 {
			itemSt := new(pb3.ItemSt)
			if err := pb3.Unmarshal(mail.UserItem, itemSt); nil == err {
				info.Items = itemSt
			} else {
				sys.LogError("%s load mail from db error:%v", sys.owner.GetName(), err)
			}
		}

		sys.mailMap[mail.MailId] = info

		if sys.loadFlag {
			sys.S2CAddMail(info)
		}
	}

	if !sys.loadFlag {
		sys.s2cMails()
		sys.loadFlag = true
	}
}

const (
	mailPage = 50
)

func (sys *MailSys) s2cMails() {
	rsp := pb3.S2C_10_1{Mails: make([]*pb3.MailInfo, 0, mailPage)}

	var offset int
	for _, mail := range sys.mailMap {
		offset++
		rsp.Mails = append(rsp.Mails, mail)
		if offset >= mailPage {
			sys.SendProto3(10, 1, &rsp)
			rsp.Mails = make([]*pb3.MailInfo, 0, mailPage)
			offset = 0
		}
	}

	if offset > 0 {
		sys.SendProto3(10, 1, &rsp)
	}
}

func (sys *MailSys) OnAddMailDbReturn(addId, delId uint64) {
	sys.SendDbLoadMail(addId)

	if delId > 0 {
		delete(sys.mailMap, delId)
		sys.SendProto3(10, 7, &pb3.S2C_10_7{Ids: []uint64{delId}})
	}
}

func (sys *MailSys) UpdateMailStatus(mailId int64, status uint32) {
	gshare.SendDBMsg(custom_id.GMsgUpdateMailStatus, sys.owner.GetId(), mailId, status)
}

func (sys *MailSys) S2CAddMail(mail *pb3.MailInfo) {
	sys.SendProto3(10, 2, &pb3.S2C_10_2{Mail: mail})
}

func (sys *MailSys) c2sRead(msg *base.Message) {
	var req pb3.C2S_10_3
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return
	}

	mail, ok := sys.mailMap[req.GetMailId()]
	if !ok {
		return
	}
	if mail.GetStatus() != gshare.MailUnRead {
		return
	}
	mail.Status = gshare.MailRead

	sys.UpdateMailStatus(int64(mail.GetMailId()), gshare.MailRead)

	sys.SendProto3(10, 4, &pb3.S2C_10_4{Datas: []*pb3.MailStatus{{MailId: mail.MailId, Status: mail.Status}}})
}

// 领取邮件奖励
func (sys *MailSys) getReward(id uint64) {
	mail, ok := sys.mailMap[id]
	if !ok {
		return
	}
	if mail.GetStatus() == gshare.MailRewarded {
		return
	}

	bagSys, ok := sys.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return
	}

	if nil != mail.Items {
		if bagSys.AvailableCount() <= 0 {
			sys.owner.SendTipMsg(tipmsgid.TpBagIsFull)
			return
		}

		mail.Status = gshare.MailRewarded
		sys.UpdateMailStatus(int64(mail.GetMailId()), gshare.MailRewarded)
		bagSys.AddItemPtr(mail.Items, true, pb3.LogId_LogMailReward)
	} else if mail.Files != "" {
		tmpStr := utils.StrToStrVec(mail.GetFiles(), "|")
		size := len(tmpStr)
		tmpStr = tmpStr[:size-1]
		rewards := make([]*jsondata.StdReward, 0, len(tmpStr))

		for _, line := range tmpStr {
			item := utils.StrToUintVec(line, "_")
			awardNum := len(item)

			if awardNum < 3 {
				sys.LogError("actorId:%d, Mail Error %v", sys.owner.GetId(), mail.GetFiles())
				return
			}
			st := &jsondata.StdReward{
				Id:    item[0],
				Count: int64(item[1]),
			}

			st.Bind = false
			if item[2] > 0 {
				st.Bind = true
			}

			rewards = append(rewards, st)
		}

		flag := engine.CheckRewards(sys.owner, rewards)
		if !flag {
			sys.owner.SendTipMsg(tipmsgid.TpBagIsFull)
			return
		}

		mail.Status = gshare.MailRewarded
		sys.UpdateMailStatus(int64(mail.GetMailId()), gshare.MailRewarded)
		if len(rewards) > 0 {
			engine.GiveRewards(sys.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMailReward})
		}
	}

	sys.SendProto3(10, 4, &pb3.S2C_10_4{Datas: []*pb3.MailStatus{{MailId: mail.MailId, Status: mail.Status}}})
}

// 请求领取邮件奖励
func (sys *MailSys) c2sGetReward(msg *base.Message) {
	var req pb3.C2S_10_5
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return
	}
	sys.getReward(req.GetMailId())
}

// 请求一键领取邮件奖励
func (sys *MailSys) c2sGetAllReward(msg *base.Message) {
	var processed int
	for id := range sys.mailMap {
		if processed > MaxPrcessCountForGetAllReward {
			return
		}
		sys.getReward(id)
	}
}

// 请求删除邮件
func (sys *MailSys) c2sDeleteMail(msg *base.Message) {
	var req pb3.C2S_10_7
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return
	}
	if len(req.Ids) > 0 {
		sys.DeleteMailByIds(req.Ids, false)
	}
}

func (sys *MailSys) DeleteMailByIds(ids []uint64, force bool) {
	nowSec := time_util.NowSec()

	var delVec []uint64
	for _, id := range ids {
		if mail, ok := sys.mailMap[id]; ok {
			if force || (len(mail.GetFiles()) <= 0 || mail.GetStatus() == gshare.MailRewarded || (mail.GetCreateTime()+7*86400) < nowSec) {
				delete(sys.mailMap, id)
				delVec = append(delVec, id)
			}
		}
	}

	sys.SendProto3(10, 7, &pb3.S2C_10_7{Ids: delVec})
	gshare.SendDBMsg(custom_id.GMsgDeleteActorMail, sys.owner.GetId(), delVec)
}

// 加载玩家邮件返回
func loadActorMailRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("loadActorMailRet", 4, len(args)) {
		return
	}
	actorId, ok1 := args[0].(uint64)
	success, ok2 := args[1].(bool)
	mailId, ok3 := args[2].(uint64)
	list, ok4 := args[3].([]*gshare.ActorMailSt)

	if !ok1 || !ok2 || !ok3 || !ok4 {
		logger.LogError("loadActorMailRet Error!!!. ok1=%v, ok2=%v, ok3=%v", ok1, ok2, ok3)
		return
	}
	actor := manager.GetPlayerPtrById(actorId)
	if nil == actor {
		return
	}
	sys, ok := actor.GetSysObj(sysdef.SiMail).(*MailSys)
	if !ok {
		return
	}
	if !success {
		logger.LogError("loadActorMailRet failed!!! actorId:%d", actorId)
		return
	}
	sys.OnLoadFromDb(mailId, list)
}

func loadActorServerMail(args ...interface{}) {
	if !gcommon.CheckArgsCount("loadActorServerMail", 3, len(args)) {
		return
	}
	actorId, ok1 := args[0].(uint64)
	success, ok2 := args[1].(bool)
	index, ok3 := args[2].(uint64)
	if !ok1 || !ok2 || !ok3 {
		logger.LogError("loadActorServerMail Args Error!!!, ok1=%v, ok2=%v, ok3=%v", ok1, ok2, ok3)
		return
	}

	actor := manager.GetPlayerPtrById(actorId)
	if nil == actor {
		return
	}
	sys, ok := actor.GetSysObj(sysdef.SiMail).(*MailSys)
	if !ok {
		return
	}
	if !success {
		sys.LoadActorServerMail()
		return
	}
	sys.OnServerMailIndexLoaded(index)
}

// 添加邮件返回
func dbAddMailRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("addMailRet", 3, len(args)) {
		return
	}
	actorId, ok1 := args[0].(uint64)
	addId, ok2 := args[1].(uint64)
	delId, ok3 := args[2].(uint64)
	if !ok1 || !ok2 || !ok3 {
		logger.LogError("addMailRet args error!!!, ok1=%v, ok2=%v, ok3=%v", ok1, ok2, ok3)
		return
	}

	actor := manager.GetPlayerPtrById(actorId)
	if nil == actor {
		return
	}
	if sys, ok := actor.GetSysObj(sysdef.SiMail).(*MailSys); ok {
		sys.OnAddMailDbReturn(addId, delId)
	}
}

func GmMakeFilesMail(actor iface.IPlayer, args ...string) bool {
	length := len(args)
	if length < 3 {
		logger.LogError("GmMakeFilesMail invalid args length")
		return false
	}

	confId := utils.AtoUint32(args[0])

	testMailArg := &mailargs.Test{
		Arg1: args[1],
		Arg2: args[2],
	}

	mail := &mailargs.SendMailSt{
		ConfId:  uint16(confId),
		Content: testMailArg,
	}

	awards := []uint32{10090001, 11290001, 11450001, 11170001, 11290002,
		11290003, 11290004, 11010001, 18020001, 11450011}
	for _, awardId := range awards {
		mail.Rewards = append(mail.Rewards, &jsondata.StdReward{
			Id:    awardId,
			Count: 10,
			Bind:  true,
		})
	}

	actor.SendMail(mail)

	return true
}

func GmMakeContentMail(actor iface.IPlayer, args ...string) bool {
	length := len(args)
	if length < 3 {
		logger.LogError("GmMakeContentMail invalid args length")
		return false
	}

	confId := utils.AtoUint32(args[0])

	testMailArg := &mailargs.Test{
		Arg1: args[1],
		Arg2: args[2],
	}

	mail := &mailargs.SendMailSt{
		ConfId:  uint16(confId),
		Content: testMailArg,
	}

	actor.SendMail(mail)

	return true

}

func init() {
	RegisterSysClass(sysdef.SiMail, func() iface.ISystem {
		return &MailSys{}
	})

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadActorMail, loadActorMailRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadActorServerMail, loadActorServerMail)
		gshare.RegisterGameMsgHandler(custom_id.GMsgAddMailRet, dbAddMailRet)

		gmevent.Register("make_files_mail", GmMakeFilesMail, 1)
		gmevent.Register("make_content_mail", GmMakeContentMail, 1)
	})

	net.RegisterSysProto(10, 3, sysdef.SiMail, (*MailSys).c2sRead)
	net.RegisterSysProto(10, 5, sysdef.SiMail, (*MailSys).c2sGetReward)
	net.RegisterSysProto(10, 6, sysdef.SiMail, (*MailSys).c2sGetAllReward)
	net.RegisterSysProto(10, 7, sysdef.SiMail, (*MailSys).c2sDeleteMail)
	event.RegSysEvent(custom_id.SeHourArrive, handleRushRankHourArrive)
	event.RegSysEvent(custom_id.SeAfterRushRankInit, handleAfterRushRankInit)
}
