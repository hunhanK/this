/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 15:43
 */

package dbworker

const (
	SQLLoadActorMail         = "call loadmaillist(?,?)" //加载角色邮件列表
	SQLAddActorMail          = `call addmail(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	SQLAddUserItemMail       = "call adduseritemmail(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	SQLUpdateMailStatus      = "call updatemailstatus(?, ?, ?)"
	SQLLoadActorServerMail   = "call loadactorservermail(?)"
	SQLUpdateActorServerMail = "call updateactorservermail(?, ?)"
	SQLDeleteActorMail       = "call deletemail(?,?)"
	SQLUpdateServerInfo      = "call updateserverinfo(?, ?)"
)
