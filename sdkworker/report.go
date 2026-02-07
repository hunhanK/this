/**
 * @Author: zjj
 * @Date: 2024/3/14
 * @Desc:
**/

package sdkworker

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/gzjjyz/logger"
	micro "github.com/gzjjyz/simple-micro"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/base/db/mysql"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"net/url"
	"strings"
	"time"
)

var eventNameMap = map[pb3.SdkReportEventType]string{
	pb3.SdkReportEventType_SdkReportEventTypeRoleRegister: "role_register",
	pb3.SdkReportEventType_SdkReportEventTypeRoleLogin:    "role_login",
}

const (
	ApiReport = "/api/report"
)

var marshaller = jsonpb.Marshaler{
	EmitDefaults: true,
	OrigName:     true,
}

var unmarshaler = &jsonpb.Unmarshaler{AllowUnknownFields: true}

func unmarshal(data []byte, m proto.Message) error {
	var buf bytes.Buffer
	buf.Write(data)
	err := unmarshaler.Unmarshal(&buf, m)
	if err != nil {
		logger.LogError("err:%v,onSdkReport resp %s", err, string(data))
		return err
	}
	return nil
}

func onSdkReport(args ...interface{}) {
	if gshare.GameConf != nil && gshare.GameConf.SkipSdkReport {
		return
	}
	utils.ProtectRun(func() {
		on360WanSdkReport(args...)
	})

	utils.ProtectRun(func() {
		onSelfSdkReport(args...)
	})
}
func onSelfSdkReport(args ...interface{}) {
	if len(args) < 2 {
		logger.LogError("sdk report failed. args = %v", args)
		return
	}

	meta := micro.MustMeta()
	if meta == nil {
		logger.LogWarn("meta is nil")
		return
	}

	sdkInfo := meta.Sdk
	if sdkInfo.ApiUrl == "" || sdkInfo.ApiSecret == "" || sdkInfo.AppId == 0 {
		logger.LogWarn("sdk have err , val is %v", sdkInfo)
		return
	}

	reportEventType := args[0].(uint32)

	// 组装 http 请求
	eventName, ok := eventNameMap[pb3.SdkReportEventType(reportEventType)]
	if !ok {
		logger.LogError("sdk report , event name is nil, reportEventType %d", reportEventType)
		return
	}
	var reportReqBody = &pb3.SdkReportSt{
		EventName: eventName,
		Appid:     sdkInfo.AppId,
	}

	// 判断是什么类型的上报
	var pbMsg proto.Message
	switch reportEventType {
	case uint32(pb3.SdkReportEventType_SdkReportEventTypeRoleRegister):
		createPlayerRet := args[1].(mysql.CreatePlayerRet)
		pbMsg = &pb3.RoleRegister{
			Account:    createPlayerRet.AccountName,
			RoleId:     fmt.Sprintf("%d", createPlayerRet.ActorId),
			Platform:   engine.GetPfId(),
			Channel:    createPlayerRet.DitchId,
			SubChannel: createPlayerRet.SubDitch,
		}
	case uint32(pb3.SdkReportEventType_SdkReportEventTypeRoleLogin):
		player := args[1].(*pb3.SimpleReportPlayer)
		pbMsg = &pb3.RoleLogin{
			Account:    player.GetAccount(),
			RoleId:     fmt.Sprintf("%d", player.GetId()),
			Platform:   engine.GetPfId(),
			Channel:    player.GetDitchId(),
			SubChannel: player.GetDitchId(),
			LoginTime:  player.GetLoginTime(),
		}
	}

	if pbMsg == nil {
		logger.LogError("not found report event type %d", reportEventType)
		return
	}

	// 组装数据
	data, err := marshaller.MarshalToString(pbMsg)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	reportReqBody.Data = data
	reportReqBody.Sign = genSign(reportReqBody, sdkInfo.ApiSecret)
	result, err := url.JoinPath(sdkInfo.ApiUrl, ApiReport)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	ctx, cancelFunc := context.WithTimeout(context.Background(), 3*time.Second)
	sdkResp, err := NewRequest().SetContext(ctx).SetHeaders(
		map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
	).SetBody(reportReqBody).Post(result)
	cancelFunc()
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	// 解析结果
	var resp pb3.SdkReportResp
	err = unmarshal(sdkResp.Body(), &resp)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	if resp.Code > 0 {
		logger.LogError("onSdkReport failed, code: %d , msg: %s, data: %s", resp.Code, resp.Message, resp.Data)
		return
	}
	logger.LogInfo("onSdkReport succeeded reportEventType %d, resp:%s", reportEventType, sdkResp.Body())
}

func genSign(st *pb3.SdkReportSt, apiKey string) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("appid=%d", st.Appid))
	builder.WriteString(fmt.Sprintf("data=%s", st.Data))
	builder.WriteString(fmt.Sprintf("event_name=%s", st.EventName))
	builder.WriteString(apiKey)
	logger.LogTrace("builder :%s", builder.String())
	md5Ret := md5.Sum([]byte(builder.String()))
	return fmt.Sprintf("%x", md5Ret)
}

func init() {
	event.RegSysEvent(custom_id.SeSDKWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterSDKMsgHandler(custom_id.GMsgSdkReport, onSdkReport)
	})
}
