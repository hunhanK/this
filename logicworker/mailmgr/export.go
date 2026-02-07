package mailmgr

import (
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/model"
)

func Init() error {
	return LoadSrvMailSync()
}

func GetServerMailList() []*model.ServerMail {
	return serverMailList
}

func SendMailToActor(actorId uint64, mail *mailargs.SendMailSt) {
	if baseData, ok := manager.GetData(actorId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		mail.Rewards = jsondata.FilterRewardByOption(mail.Rewards,
			jsondata.WithFilterRewardOptionByJob(baseData.Job),
			jsondata.WithFilterRewardOptionBySex(baseData.Sex),
			jsondata.WithFilterRewardOptionByLvRange(baseData.Lv),
			jsondata.WithFilterRewardOptionByOpenDayRange(gshare.GetOpenServerDay()),
			jsondata.WithFilterRewardOptionByOpenWeekRange(gshare.GetOpenServerWeeks()),
			jsondata.WithFilterRewardOptionByWeekCycle(),
			jsondata.WithFilterRewardOptionByOpenWeekCycle(gshare.GetOpenServerWeeks()),
		)
	}

	fileCount := len(mail.Rewards)

	if fileCount > 0 {
		sendFileMailToActor(actorId, mail)
		return
	}

	itemCount := len(mail.UserItems)

	if itemCount > 0 {
		sendUserItemMailToActor(actorId, mail)
		return
	}

	sendMsgMailToActor(actorId, mail)
}

func SendMailToActors(actorIds []uint64, mail *mailargs.SendMailSt) {
	if len(actorIds) <= 0 {
		return
	}
	for _, actorId := range actorIds {
		SendMailToActor(actorId, mail)
	}
}

// SendFileMailToActorStr
func SendFileMailToActorStr(actorId uint64, mail *mailargs.SendMailStrSt) {
	st := &gshare.ActorMailSt{
		ConfId:   mail.ConfId,
		MailId:   series.AllocMailIdSeries(),
		ActorId:  actorId,
		MailType: gshare.MailTypeFile,
		SendTick: time_util.NowSec(),
		Content:  mail.Content,
		AwardStr: mail.Reward,
		Title:    mail.Title,
	}

	addActorMail2Db(actorId, st)
}

// AddSrvMailStr 新增服务器邮件
func AddSrvMailStr(confId uint16, argStr string, rewards []*jsondata.StdReward) {
	conf := gshare.GameConf
	rewards = jsondata.FilterRewardByOption(rewards,
		jsondata.WithFilterRewardOptionByOpenDayRange(gshare.GetOpenServerDay()),
		jsondata.WithFilterRewardOptionByOpenWeekRange(gshare.GetOpenServerWeeks()),
		jsondata.WithFilterRewardOptionByWeekCycle(),
		jsondata.WithFilterRewardOptionByOpenWeekCycle(gshare.GetOpenServerWeeks()),
	)
	if fileCount := len(rewards); fileCount > 0 {
		addSrvFileMailStr(conf.SrvId, confId, argStr, rewards)
	} else {
		addSrvMsgMailStr(conf.SrvId, confId, argStr)
	}
	LoadSrvMail()
}

func AddSrvMail(mail *mailargs.SendMailSt) {
	conf := gshare.GameConf
	if len(mail.Rewards) > 0 {
		addSrvFileMail(conf.SrvId, mail)
	} else {
		addSrvMsgMail(conf.SrvId, mail)
	}
	LoadSrvMail()
}

func onFightSrvSendSrvMail(buf []byte) {
	msg := &pb3.FSendMail{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	awards := jsondata.Pb3RewardVecToStdRewardVec(msg.Awards)
	AddSrvMailStr(uint16(msg.ConfId), msg.ArgString, awards)
}

func init() {
	engine.RegisterSysCall(sysfuncid.FGSendSrvMail, onFightSrvSendSrvMail)
}
