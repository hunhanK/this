package gshare

type MsgSt struct {
	MsgId   int64
	MsgType int
	Msg     []byte
}

const (
	OfflineDisActiveTitle                   = iota + 1 //回收称号
	OfflineActiveTitle                                 //激活称号
	OfflineChargeOrder                                 //充值
	OfflineJinYan                                      //禁言
	OfflineAddMoney                                    //离线加钱
	OfflineSkipFreeVip                                 //跳过免费会员
	OfflineLearnSkill                                  // 学习技能
	OfflinePlayerMergeEvent                            // 触发合服事件
	OfflineDeleteItemByHand                            // 删除道具
	OfflineDisActiveFashion                            // 离线回收时装
	OfflineActiveFashion                               //离线激活时装
	OfflineFriendApply                                 //离线收到好友申请
	OfflineAcceptFriend                                //通过离线好友
	OfflineInviteGuild                                 //离线收到邀请信息
	OfflineFairyDynastyBeWorshiped                     //离线仙朝膜拜
	OfflineTopFightScoreReulst                         //巅峰竞技淘汰赛结果
	OfflineResetTopFight                               //重置巅峰竞技
	OfflineDirectWinPrepare                            //巅峰竞技轮空准备阶段
	OfflineConfessMsg                                  //被表白通知
	OfflineMarryCdClear                                //清结婚cd
	OfflineWBossUsedTimes                              //结婚副本双人boss次数
	OfflineJoinGuild                                   //加入仙盟
	OfflineChangeRoleName                              //修改角色名
	OfflinePwdPacketsSendThank                         //下发口令红包感谢奖励
	OfflineSectAuctionRecord                           //新增仙宗拍卖记录
	OfflineGuildPartyTeach                             //仙盟宴会-传功奖励
	OfflineAddAuctionRecord                            //新增拍卖记录
	OfflineRspMarryReq                                 //回应结婚请求
	OfflineGMReAcceptQuest                             //离线GM操作任务
	OfflineGMReAcceptPYYQuest                          //离线GM操作个人运营活动任务
	OfflineGMAddMileCharge                             //离线GM操作法则累充额度
	OfflineGMClearChargeToken                          //清除代币额度
	OfflineGMSevenDayReturnMoneyDailyData              //离线GM操作个人运营活动-七日返利
	OfflineGMSectTaskRefresh                           //离线GM刷新宗门任务
	OfflineCmdDeductMoney                              //离线移除货币
	OfflineFixNewJingJieTuPo                           //离线修复新境界雷劫次数
	OfflineHiddenSuperVip                              //离线更新隐藏超级VIP面板
	OfflineResetFirstRecharge                          //离线重置首充
	OfflineAddAskForHelpLog                            //离线新增求助日志
	OfflineGetNewCollectCard                           //离线新增卡片（仅收集）
	OfflineClearActorChat                              //离线清除指定玩家聊天记录
	OfflineYYCelebrationFreePrivilege                  //庆典节日特权
	OfflineSectHuntingSysResetLiveTime                 //仙宗狩猎-重置进入时长
	OfflineCompleteDivineRealmOpenBoxQuest             //仙域2.0宝箱任务触发完成
	OfflineAddActSweepDay                              //增加活动任务日常扫荡时长
	OfflineSectHuntingSysResetChallengeBoss            //仙宗狩猎-重置BOSS
	OfflineOpenSpiritPaintingSeason                    //开启灵画新赛季
	OfflineAddBloodlineFBTimes                         //增加血脉副本次数
	OfflineQiXiConfession                              //七夕离线被表白
	OfflineKillHeavenlyPalaceBoss                      //移除召唤的boss
	OfflineReturnHeavenlyPalaceBossConsume             //返回召唤boss的材料
	OfflineSetFeatherStatus                            //设置羽翼状态
	OfflineActiveHonor                                 //激活头衔

	OfflineMax
)
