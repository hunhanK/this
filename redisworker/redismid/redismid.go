package redismid

type RedisMID uint32

const (
	SaveGameBasic      RedisMID = iota + 1 // 保存服务器基础信息
	LoadSmallCross                         // 获取逻辑服的小跨服信息
	EnterSmallCross                        // 进入小跨服
	DelGameBasic                           // 移除服务器基础信息
	LoadChatRule                           // 获取聊天规则
	SaveActorCache                         // 保存角色缓存
	LoadMediumCross                        // 获取逻辑服的中跨服信息
	EnterMediumCross                       // 进入中跨服
	ReportPlayerLogout                     // 上报玩家登出
	ReloadGuildRule                        // 加载仙盟规则
	SaveCmdYYSetting                       // 保存后台运营活动设置
)
