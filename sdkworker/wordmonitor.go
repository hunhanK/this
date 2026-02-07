package sdkworker

import (
	"crypto/md5"
	"errors"
	"fmt"
	"github.com/bitly/go-simplejson"
	"io"
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"

	wm "github.com/gzjjyz/wordmonitor"
)

func onGMsgWordMonitor(args ...interface{}) {
	word, ok := args[0].(*wordmonitor.Word)
	if !ok {
		return
	}

	commonData := word.ChatBaseData
	if commonData == nil {
		commonData = &wm.CommonData{}
	}
	commonData.Content = word.Content

	// 跳过敏感词检测
	if gshare.GameConf != nil && gshare.GameConf.SkipWordMonitor {
		word.Ret = 0
		gshare.SendGameMsg(custom_id.GMsgWordMonitorRet, word)
		return
	}

	var ret wm.Ret
	var err error

	monitor := GetMonitor(word.DitchId)
	start := time.Now()
	switch word.Type {
	case wordmonitor.Chat:
		ret, err = monitor.CheckChat(commonData)
	case wordmonitor.Name:
		ret, err = monitor.CheckName(commonData)
	case wordmonitor.GuildBasic:
		ret, err = checkGuildName(monitor, commonData, word.Data)
	case wordmonitor.GuildName, wordmonitor.GuildBanner, wordmonitor.GuildNotice:
		ret, err = monitor.CheckName(commonData)
	case wordmonitor.InfFairyPlaceSignature:
		ret, err = monitor.CheckName(commonData)
	case wordmonitor.TitleCustom:
		ret, err = monitor.CheckName(commonData)
	case wordmonitor.HonorCustom:
		ret, err = monitor.CheckName(commonData)
	case wordmonitor.TipCustom:
		ret, err = monitor.CheckName(commonData)
	case wordmonitor.CardCustom:
		ret, err = monitor.CheckName(commonData)
	case wordmonitor.GuildPrefixName:
		ret, err = monitor.CheckName(commonData)
	case wordmonitor.QiXiConfession:
		ret, err = monitor.CheckName(commonData)
	case wordmonitor.ClearCache:
		monitor.ClearCache()
		return
	default:
		logger.LogError("world check error! invalid world.Type %v", word.Type)
		return
	}

	if nil != err {
		logger.LogError("word check error! content=%s, %v", word.Content, err)
		return
	}

	word.Ret = ret
	if word.Ret == wm.Success {
		ret2, err := checkChatByBackStage(word)
		if nil != err {
			logger.LogError("word backStage check error! content=%s, %v", word.Content, err)
		}
		word.BackStageRet = ret2
	}

	gshare.SendGameMsg(custom_id.GMsgWordMonitorRet, word)
	logger.LogInfo("=============sdk check word filter cost:%v", time.Since(start))
}

func checkGuildName(monitor wm.Monitor, commonData *wm.CommonData, data interface{}) (wm.Ret, error) {
	req, ok := data.(*pb3.C2S_29_2)
	if !ok {
		return 0, fmt.Errorf("C2S_29_2 parse fail:%v", data)
	}
	commonData.Content = req.GetName()
	ret1, err := monitor.CheckName(commonData)
	if err != nil {
		return ret1, err
	}
	commonData.Content = req.GetBanner().GetBannerChar()
	ret2, err := monitor.CheckName(commonData)
	if err != nil {
		return ret2, err
	}
	return wm.Ret(utils.MaxInt(int(ret1), int(ret2))), nil
}

func checkChatByBackStage(word *wordmonitor.Word) (wm.Ret, error) {
	params := url.Values{}
	timestamp := time.Now().Unix()
	pfId := engine.GetPfId()
	content := word.Content
	params.Set("pid", fmt.Sprintf("%v", pfId))
	params.Set("content", content)
	params.Set("time", fmt.Sprintf("%v", timestamp))
	params.Set("requestSign", genBackStageCheckSign(pfId, content, timestamp))
	resp, err := http.Post(gshare.GameConf.BackStageChatUrl, "application/x-www-form-urlencoded", strings.NewReader(params.Encode()))
	if nil != err {
		return 0, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if nil != err {
		return 0, err
	}

	retJson, err := simplejson.NewJson(body)
	if err != nil {
		return 0, err
	}

	code, _ := retJson.Get("code").Int()
	if code == 200 {
		data, _ := retJson.Get("data").Int()
		return wm.Ret(data), nil
	} else {
		msg, err := retJson.Get("message").String()
		if nil != err {
			return 0, err
		}
		return 0, errors.New(msg)
	}
}

func genBackStageCheckSign(ditchId uint32, content string, time int64) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("content=%s", content))
	builder.WriteString(fmt.Sprintf("pid=%d", ditchId))
	builder.WriteString(fmt.Sprintf("time=%d", time))
	builder.WriteString(gshare.GameConf.BackStageChatKey)
	logger.LogInfo("builder :%s", builder.String())
	md5Ret := md5.Sum([]byte(builder.String()))
	return fmt.Sprintf("%x", md5Ret)
}

func init() {
	event.RegSysEvent(custom_id.SeSDKWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterSDKMsgHandler(custom_id.GMsgWordMonitor, onGMsgWordMonitor)
	})
}
