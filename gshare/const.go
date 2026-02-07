package gshare

// 静态定义
const DAY_SECOND = 86400 //一天的秒数

const (
	DEBUG_RANK        = false //排行榜debug
	DEBUG_ROLE_ACTION = true  //角色行为
)

// 性别
const (
	Male   = 1 //男
	Female = 2 //女
)

// 邮件状态
const (
	MailUnRead   = 0 //未读
	MailRead     = 1 //已读
	MailRewarded = 2 //已领取
)

// 数据库邮件类型
const (
	MailTypeGlobal = 1 //全服邮件
	MailTypeServer = 2 //单服邮件
)

// 阵营战阵营名字
const (
	CampWoMa = 1 //沃玛阵营
	CampZuMa = 2 //祖玛阵营
)

var EnemyCamp = map[uint8]uint8{
	CampWoMa: CampZuMa,
	CampZuMa: CampWoMa,
}

// 奖励id
const (
	WorshipReward = 1 //膜拜奖励
)

// 邮件
const (
	MailTypeMsg      = 0 //不带附件
	MailTypeFile     = 1 //普通附件
	MailTypeUserItem = 2 //UserItem

	MaxMailFileCount = 10 //邮件最大附件数量
)
