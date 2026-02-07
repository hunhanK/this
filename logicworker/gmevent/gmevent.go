/**
* @Author: ChenJunJi
* @Desc:
* @Date: 2021/7/14 20:52
 */

package gmevent

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
	"strings"
)

type funcSt struct {
	fn    func(player iface.IPlayer, args ...string) bool
	level int
}

var funcList = make(map[string]*funcSt)

func Register(cmd string, cb func(player iface.IPlayer, args ...string) bool, level int) {
	cmd = strings.ToLower(cmd)
	if _, exist := funcList[cmd]; exist {
		logger.LogError("gm cmd name exist:%s", cmd)
		return
	}
	funcList[cmd] = &funcSt{
		fn:    cb,
		level: level,
	}
}

func DoGmFunc(actor iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_2_255
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return nil
	}

	logger.LogInfo("%s 使用gm命令：%s %v", actor.GetName(), req.GetCmd(), req.Args[:])

	//判断gm等级
	gmLevel := actor.GetGmLevel()
	cmd := strings.ToLower(req.GetCmd())
	st, exist := funcList[cmd]
	if !exist { //本服不存在的转发到战斗服
		if buf, err := pb3.Marshal(&req); nil == err {
			actor.CallActorFunc(actorfuncid.ActorGm, &pb3.ProtoByteArray{Buff: buf, U32Param: gmLevel})
		}
		return nil
	}

	if gmLevel < uint32(st.level) {
		return neterror.GmLevelLimitError("gm: %s gm等级不足", cmd)
	}

	if nil == st.fn {
		return neterror.GmLevelLimitError("gm: %s 处理函数为空", cmd)
	}
	if st.fn(actor, req.Args[:]...) {
		actor.SendTipMsg(tipmsgid.TpStr, "GM Success")
	} else {
		actor.SendTipMsg(tipmsgid.TpStr, "GM Failed")
	}

	return nil
}

func init() {
	net.RegisterProto(2, 255, DoGmFunc)

	Register("msg.test", func(actor iface.IPlayer, args ...string) bool {
		return true
	}, 1)

}
