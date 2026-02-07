/**
 * @Author: zjj
 * @Date: 2025/1/6
 * @Desc:
**/

package manager

import (
	"encoding/json"
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
	"jjyz/gameserver/redisworker/redismid"
)

type PfChannelChatRule struct {
	PfId              uint32 `json:"pf_id"`
	Channel           uint32 `json:"channel"`
	ChargeTotalAmount uint32 `json:"charge_total_amount"` // 累积充值金额
	VipLevel          uint32 `json:"vip_level"`           // 贵族等级
	Level             uint32 `json:"level"`               // 角色等级
}

var chatRuleMap = make(map[string]*PfChannelChatRule) // key pfId_channel

func GetChatRule(pfId uint32, channel uint32) *PfChannelChatRule {
	key := fmt.Sprintf("%d_%d", pfId, channel)
	return chatRuleMap[key]
}

func (r *PfChannelChatRule) Check(player iface.IPlayer) bool {
	if r.ChargeTotalAmount != 0 && player.GetBinaryData().ChargeInfo.TotalChargeMoney < r.ChargeTotalAmount {
		player.LogWarn("充值金额未达到，当前:%d, 限制:%d", player.GetBinaryData().ChargeInfo.TotalChargeMoney, r.ChargeTotalAmount)
		player.SendTipMsg(tipmsgid.TpChatLimitByCharge, r.ChargeTotalAmount/100)
		return false
	}
	if r.ChargeTotalAmount != 0 && player.GetBinaryData().ChargeInfo.TotalChargeMoney < r.ChargeTotalAmount && r.Level != 0 && player.GetLevel() < r.Level {
		player.LogWarn("充值金额未达到，当前:%d, 限制:%d", player.GetBinaryData().ChargeInfo.TotalChargeMoney, r.ChargeTotalAmount)
		player.SendTipMsg(tipmsgid.TpChatLimitByCharge, r.ChargeTotalAmount/100)
		return false
	}
	return true
}

func (r *PfChannelChatRule) ToPb() *pb3.ChatChannelRule {
	return &pb3.ChatChannelRule{
		Channel:           r.Channel,
		ChargeTotalAmount: r.ChargeTotalAmount,
		VipLv:             r.VipLevel,
		Level:             r.Level,
	}
}

func LoadChatRule() {
	gshare.SendRedisMsg(redismid.LoadChatRule)
}

func onGMsgLoadChatRule(param ...interface{}) {
	if len(param) == 0 {
		return
	}
	vec, ok := param[0].(map[string]string)
	if !ok {
		return
	}
	chatRuleMap = make(map[string]*PfChannelChatRule)
	for _, buf := range vec {
		var basic PfChannelChatRule
		err := json.Unmarshal([]byte(buf), &basic)
		if err != nil {
			logger.LogError(err.Error())
			continue
		}
		chatRuleMap[fmt.Sprintf("%d_%d", basic.PfId, basic.Channel)] = &basic
	}
	SendChatRule(nil)
}

func SendChatRule(player iface.IPlayer) {
	var ret = make(map[uint32]*pb3.ChatChannelRule)
	for _, rule := range chatRuleMap {
		pb := rule.ToPb()
		ret[pb.Channel] = pb
	}
	var resp = &pb3.S2C_5_11{
		RuleMap: ret,
	}
	bytes, err := pb3.Marshal(resp)
	if err != nil {
		logger.LogError("convert byte failed")
		return
	}
	if player != nil {
		player.SendProtoBuffer(5, 11, bytes)
		return
	}
	AllOnlinePlayerDo(func(p iface.IPlayer) {
		p.SendProtoBuffer(5, 11, bytes)
	})
}

func init() {
	event.RegSysEvent(custom_id.SeRedisWorkerInitDone, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadChatRule, onGMsgLoadChatRule)
		LoadChatRule()
	})
	net.RegisterProto(5, 11, func(player iface.IPlayer, msg *base.Message) error {
		SendChatRule(player)
		return nil
	})
}
