/**
 * @Author: LvYuMeng
 * @Date: 2024/12/27
 * @Desc:
**/

package invitecodemgr

import (
	"encoding/csv"
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"os"
	"path"
)

func generateCode(player iface.IPlayer, msg *base.Message) error {
	sst := gshare.GetStaticVar()
	if sst.InviteCodes == nil {
		sst.InviteCodes = make(map[string]bool)
	}

	if player.GetNirvanaLevel() < jsondata.GlobalUint("inviteCodeNirvanaLv") {
		player.SendTipMsg(tipmsgid.CircleNotEnough)
		return nil
	}
	if player.GetVipLevel() < jsondata.GlobalUint("inviteCodeVip") {
		player.SendTipMsg(tipmsgid.TpVipLvNotEnough)
		return nil
	}

	binary := player.GetBinaryData()
	if binary.GetInviteCode() != "" {
		return neterror.ParamsInvalidError("code is create")
	}

	for true {
		code := random.IntervalUU(10000000, 99999999) // 随机一个八位数
		codeStr := utils.I32toa(code)
		if _, exist := sst.InviteCodes[codeStr]; !exist {
			sst.InviteCodes[codeStr] = false // 未使用

			binary.InviteCode = codeStr
			player.SendProto3(2, 195, &pb3.S2C_2_195{
				Code: codeStr,
			})
			logger.LogDebug("generate invite code :%s", codeStr)
			return nil
		}
	}

	return nil
}

func GenerateCodeGroup(num uint32) {
	sst := gshare.GetStaticVar()
	if sst.InviteCodes == nil {
		sst.InviteCodes = make(map[string]bool)
	}

	cnt := uint32(0)
	var codes []string
	for cnt < num {
		code := random.IntervalUU(10000000, 99999999) // 随机一个八位数
		codeStr := utils.I32toa(code)
		if _, exist := sst.InviteCodes[codeStr]; !exist {
			sst.InviteCodes[codeStr] = false // 未使用

			logger.LogInfo(fmt.Sprintf("平台id:%d, 服务器id:%d, 生成激活码:%s", engine.GetPfId(), engine.GetServerId(), codeStr))
			cnt++
			codes = append(codes, codeStr)
		}
	}

	file, err := os.Create(path.Join(utils.GetCurrentDir(), fmt.Sprintf("invite_codes_%d_%d_%d.csv", engine.GetPfId(), engine.GetServerId(), time_util.NowSec())))
	if err != nil {
		logger.LogError(fmt.Sprintf("创建CSV文件失败: %v", err))
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, code := range codes {
		if err := writer.Write([]string{code}); err != nil {
			logger.LogError(fmt.Sprintf("写入CSV文件失败: %v", err))
			return
		}
	}
}

func CheckCode(code string) uint32 {
	sst := gshare.GetStaticVar()
	if sst.InviteCodes == nil {
		return custom_id.InviteCodeInvalid
	}

	if _, exist := sst.InviteCodes[code]; !exist {
		return custom_id.InviteCodeInvalid
	}

	if sst.InviteCodes[code] == true {
		return custom_id.InviteCodeUsed
	}

	return 0
}

func UseCode(code string) uint32 {
	if errNo := CheckCode(code); errNo != 0 {
		return errNo
	}
	sst := gshare.GetStaticVar()
	sst.InviteCodes[code] = true
	return 0
}

func sendCode(player iface.IPlayer) {
	player.SendProto3(2, 195, &pb3.S2C_2_195{
		Code: player.GetBinaryData().InviteCode,
	})
}

func init() {
	gmevent.Register("icode", func(actor iface.IPlayer, args ...string) bool {
		generateCode(actor, nil)
		return true
	}, 1)

	gmevent.Register("icodegroup", func(actor iface.IPlayer, args ...string) bool {
		GenerateCodeGroup(10)
		return true
	}, 1)

	event.RegActorEvent(custom_id.AeAfterLogin, func(player iface.IPlayer, args ...interface{}) {
		sendCode(player)
	})
	event.RegActorEvent(custom_id.AeReconnect, func(player iface.IPlayer, args ...interface{}) {
		sendCode(player)
	})

	net.RegisterProto(2, 195, generateCode)
}
