package redisworker

import (
	"context"
	"encoding/json"
	"fmt"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/rediskey"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/redisworker/redismid"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

func onSaveGameBasic(args ...interface{}) {
	if len(args) <= 0 {
		return
	}
	req, ok := args[0].(argsdef.GameBasicData)
	if !ok {
		return
	}

	key := fmt.Sprintf(rediskey.GameBasicOfPf, req.PfId)
	field := utils.I32toa(req.SrvId)

	cfgJ, err := json.Marshal(req)
	if err != nil {
		logger.LogError("on save game basic error! %v", err.Error())
		return
	}

	err = client.HSet(context.Background(), key, field, cfgJ).Err()
	if nil != err {
		logger.LogError("on save game basic error! %v", err.Error())
		return
	}
}

func onDelGameBasic(args ...interface{}) {
	if len(args) <= 0 {
		return
	}
	req, ok := args[0].(argsdef.GameBasicData)
	if !ok {
		return
	}

	key := fmt.Sprintf(rediskey.GameBasicOfPf, req.PfId)
	field := utils.I32toa(req.SrvId)

	err := client.HDel(context.Background(), key, field).Err()
	if nil != err {
		logger.LogError("on save game basic error! %v", err.Error())
		return
	}
}

func onLoadSmallCross(args ...interface{}) {
	info := getCrossInfo()
	if info != nil {
		gshare.SendGameMsg(custom_id.GMsgLoadSmallCrossRet, info)
	}
}

func getCrossInfo() *argsdef.GameGetCross {
	cross, err := argsdef.NewGameSmallCrossData(client, engine.GetPfId(), engine.GetServerId())
	if nil != err {
		logger.LogError("load small cross error. %v", err)
		return nil
	}

	ret := argsdef.GameGetCross{
		Times: cross.CrossTimes,
	}

	if info := cross.Info; nil != info {
		ret.Camp = info.Camp
		ret.CrossTime = info.CrossTime
		basic, err := argsdef.NewSmallCrossBasicData(client, info.ZoneId, info.CrossId)
		if err != nil {
			logger.LogError("on get small cross basic error! %v", err)
			return nil
		}
		if nil != basic {
			ret.Host = basic.Host
			ret.Port = basic.Port
			ret.Camp = info.Camp
			ret.CrossId = basic.CrossId
			ret.ZoneId = basic.ZoneId
		}
	}

	return &ret
}

func onEnterSmallCross(args ...interface{}) {
	info := getCrossInfo()

	if info != nil {
		gshare.SendGameMsg(custom_id.GMsgEnterSmallCrossRet, info)
	}
}

func getMediumCrossInfo() *argsdef.GameGetMediumCross {
	cross, err := argsdef.NewGameMediumCrossData(client, engine.GetPfId(), engine.GetServerId())
	if nil != err {
		logger.LogError("load small cross error. %v", err)
		return nil
	}

	ret := argsdef.GameGetMediumCross{}

	if info := cross.Info; nil != info {
		ret.CrossTime = info.CrossTime
		basic, err := argsdef.NewMediumCrossBasicData(client, info.ZoneId, info.CrossId)
		if err != nil {
			logger.LogError("on get small cross basic error! %v", err)
			return nil
		}
		if nil != basic {
			ret.Host = basic.Host
			ret.Port = basic.Port
			ret.CrossId = basic.CrossId
			ret.ZoneId = basic.ZoneId
		}
	}

	return &ret
}

func onLoadMediumCross(args ...interface{}) {
	info := getMediumCrossInfo()
	if info != nil {
		gshare.SendGameMsg(custom_id.GMsgLoadMediumCrossRet, info)
	}
}

func onEnterMediumCross(args ...interface{}) {
	info := getMediumCrossInfo()

	if info != nil {
		gshare.SendGameMsg(custom_id.GMsgEnterMediumCrossRet, info)
	}
}

func onLoadChatRule(_ ...interface{}) {
	basicVec, err := client.HGetAll(context.Background(), fmt.Sprintf(rediskey.GameChatRule, engine.GetPfId())).Result()
	if err != nil {
		logger.LogWarn("not found chat rule cache")
		return
	}
	gshare.SendGameMsg(custom_id.GMsgLoadChatRule, basicVec)
}

func init() {
	Register(redismid.SaveGameBasic, onSaveGameBasic)
	Register(redismid.DelGameBasic, onDelGameBasic)
	Register(redismid.LoadSmallCross, onLoadSmallCross)
	Register(redismid.EnterSmallCross, onEnterSmallCross)
	Register(redismid.LoadMediumCross, onLoadMediumCross)
	Register(redismid.EnterMediumCross, onEnterMediumCross)
	Register(redismid.LoadChatRule, onLoadChatRule)
}
