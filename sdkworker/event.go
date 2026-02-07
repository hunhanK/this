/**
 * @Author: PengZiMing
 * @Desc:
 * @Date: 2022/11/5 10:33
 */

package sdkworker

import (
	"io/ioutil"
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"net/http"

	"github.com/gzjjyz/logger"
	"github.com/tidwall/gjson"
)

type SDKEventCallBack func(args ...interface{}) error

var SDKEventDispatcher = make(map[uint32]map[uint32]SDKEventCallBack)

func RegSDKEvent(ditchId, infoType uint32, fn SDKEventCallBack) {
	if nil == fn {
		logger.LogStack("RegSDKEvent cb func is nil")
		return
	}

	ditchMap := SDKEventDispatcher[ditchId]
	if ditchMap == nil {
		ditchMap = make(map[uint32]SDKEventCallBack)
		SDKEventDispatcher[ditchId] = ditchMap
	}
	ditchMap[infoType] = fn
}

func getSDKEvent(ditchId, infoType uint32) SDKEventCallBack {
	ditchMap, ok := SDKEventDispatcher[ditchId]
	if !ok {
		return nil
	}
	if fn, ok := ditchMap[infoType]; ok {
		return fn
	}
	return nil
}

func DitchGet(param ...interface{}) {
	url := param[0].(string)
	resp, err := http.Get(url)
	if err != nil {
		logger.LogError("%s", err)
		return
	}
	defer resp.Body.Close()
	ParseResp(resp)
	return
}

func ParseResp(resp *http.Response) {
	if resp.StatusCode != 200 {
		logger.LogError("api error:%s ", resp.Status)
	} else {
		if body, err := ioutil.ReadAll(resp.Body); nil != err {
			logger.LogError("api error:%v ", err)
			return
		} else {
			ret := string(body)
			status := gjson.Get(ret, "result").Bool()
			if status != true {
				logger.LogError("%v", ret)
			}
		}
	}
}

func init() {
	event.RegSysEvent(custom_id.SeSDKWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterSDKMsgHandler(custom_id.GMsgIdDitchGet, DitchGet)
	})
}
