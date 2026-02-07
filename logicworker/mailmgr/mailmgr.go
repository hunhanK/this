package mailmgr

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/gameserver/model"

	"github.com/gzjjyz/logger"

	"jjyz/base/time_util"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/manager"
	"math"
)

var serverMailList []*model.ServerMail

// LoadSrvMailSync 同步加载服务器邮件 启动时调用
func LoadSrvMailSync() error {
	var err error
	serverMailList, err = model.LoadServerMail()
	return err
}

// LoadSrvMail 加载服务器邮件 启动、新增时调用
func LoadSrvMail() {
	conf := gshare.GameConf
	gshare.SendDBMsg(custom_id.GMsgLoadSrvMail, conf.SrvId)
}

// 加载服务器邮件返回
func loadSrvMailRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("loadSrvMailRet", 1, len(args)) {
		return
	}
	list, ok := args[0].([]*model.ServerMail)
	if !ok {
		logger.LogError("loadSrvMailRet Args Error!!!")
		return
	}

	serverMailList = list

	manager.OnServerMailLoaded()
}

func addSrvMsgMail(srvId uint32, mail *mailargs.SendMailSt) {
	var contentStr string
	var err error
	if mail.Content != nil {
		contentStr, err = mailargs.MarshalMailArg(mail.Content)
		if nil != err {
			logger.LogError("sendMsgMailToActor failed marshal mail.Content error: %s", err)
			return
		}
	}
	addSrvMsgMailStr(srvId, mail.ConfId, contentStr)
}

func addSrvFileMail(srvId uint32, mail *mailargs.SendMailSt) {
	fileCount := len(mail.Rewards)

	if fileCount <= 0 {
		return
	}

	var contentStr string
	var err error
	if mail.Content != nil {
		contentStr, err = mailargs.MarshalMailArg(mail.Content)
		if nil != err {
			logger.LogError("sendFileMailToActor failed marshal mail.Content error: %s", err)
			return
		}
	}

	addSrvFileMailStr(srvId, mail.ConfId, contentStr, mail.Rewards)
}

func addSrvMsgMailStr(srvId uint32, confId uint16, argStr string) {
	st := &model.ServerMail{
		ConfId:   confId,
		Type:     gshare.MailTypeMsg,
		SendTick: time_util.NowSec(),
		Content:  argStr,
	}
	gshare.SendDBMsg(custom_id.GMsgAddServerMail, st)
}

func addSrvFileMailStr(srvId uint32, confId uint16, argStr string, rewards []*jsondata.StdReward) {
	fileCount := len(rewards)

	if fileCount <= 0 {
		return
	}

	splitedRewards := splitMailRewardsAndBuildToStrArray(rewards)

	for _, rewardStr := range splitedRewards {
		st := &model.ServerMail{
			ConfId:   confId,
			Type:     gshare.MailTypeFile,
			SendTick: time_util.NowSec(),
			Content:  argStr,
			AwardStr: rewardStr,
		}
		gshare.SendDBMsg(custom_id.GMsgAddServerMail, st)
	}
}

func splitMailRewardsAndBuildToStrArray(r []*jsondata.StdReward) []string {
	rewards := jsondata.MergeStdReward(r)

	var result []string
	fileCount := len(rewards)

	if fileCount == 0 {
		return nil
	}

	count := int(math.Ceil(float64(fileCount) / gshare.MaxMailFileCount))
	for i := 0; i < count; i++ {
		var awardStr string

		for j := 0; j < gshare.MaxMailFileCount; j++ {
			index := i*gshare.MaxMailFileCount + j
			if index >= fileCount {
				break
			}

			isBind := 1
			if !rewards[index].Bind {
				isBind = 0
			}

			if rewards[index].Count > 0 {
				awardStr += fmt.Sprintf("%d_%d_%d", rewards[index].Id, rewards[index].Count, isBind)
				awardStr += "|"
			}

		}

		result = append(result, awardStr)
	}
	return result
}

// sendFileMailToActor 发送附件邮件给玩家
func sendFileMailToActor(actorId uint64, mail *mailargs.SendMailSt) {
	fileCount := len(mail.Rewards)

	if fileCount == 0 {
		return
	}

	var contentStr string
	var err error
	if mail.Content != nil {
		contentStr, err = mailargs.MarshalMailArg(mail.Content)
		if nil != err {
			logger.LogError("sendFileMailToActor failed marshal mail.Content error: %s", err)
			return
		}
	}

	splitedRewards := splitMailRewardsAndBuildToStrArray(mail.Rewards)

	for _, rewardStr := range splitedRewards {
		st := &gshare.ActorMailSt{
			MailId:   series.AllocMailIdSeries(),
			ConfId:   mail.ConfId,
			ActorId:  actorId,
			MailType: gshare.MailTypeFile,
			SendTick: time_util.NowSec(),
			AwardStr: rewardStr,
			Content:  contentStr,
			Title:    mail.Title,
		}

		addActorMail2Db(actorId, st)
	}

}

func sendUserItemMailToActor(actorId uint64, mail *mailargs.SendMailSt) {
	itemCount := len(mail.UserItems)

	if itemCount == 0 {
		return
	}

	var contentStr string
	var err error
	if mail.Content != nil {
		contentStr, err = mailargs.MarshalMailArg(mail.Content)
		if nil != err {
			logger.LogError("sendUserItemMailToActor failed marshal mail.Content error: %s", err)
			return
		}
	}
	for _, item := range mail.UserItems {
		st := &gshare.ActorMailSt{
			MailId:   series.AllocMailIdSeries(),
			ConfId:   mail.ConfId,
			ActorId:  actorId,
			MailType: gshare.MailTypeUserItem,
			SendTick: time_util.NowSec(),
			Content:  contentStr,
			Title:    mail.Title,
		}

		st.UserItem = base.Pb2Byte(item)
		addActorMail2Db(actorId, st)
	}
}

func addActorMail2Db(actorId uint64, data *gshare.ActorMailSt) {
	gshare.SendDBMsg(custom_id.GMsgAddMail, actorId, data)
}

// notice: 这个函数指挥把邮件内容发给玩家，不会发附件,
// mail 参数里面的 `Rewards' 和 `UserItems' 都会被忽略
func sendMsgMailToActor(actorId uint64, mail *mailargs.SendMailSt) {
	var contentStr string
	var err error
	if mail.Content != nil {
		contentStr, err = mailargs.MarshalMailArg(mail.Content)
		if nil != err {
			logger.LogError("sendMsgMailToActor failed marshal mail.Content error: %s", err)
			return
		}
	}

	st := &gshare.ActorMailSt{
		MailId:   series.AllocMailIdSeries(),
		ConfId:   mail.ConfId,
		ActorId:  actorId,
		MailType: gshare.MailTypeMsg,
		SendTick: time_util.NowSec(),
		Content:  contentStr,
		Title:    mail.Title,
	}

	addActorMail2Db(actorId, st)
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadSrvMailRet, loadSrvMailRet)
	})
}
