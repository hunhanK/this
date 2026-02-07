/**
 * @Author: ChenJunJi
 * @Date: 2023/11/08
 * @Desc:
**/

package model

type GmCmd struct {
	Id       int64  `xorm:"pk 'id'"`    // 自增id
	DelTime  int32  `xorm:"'deltime'"`  // 删除时间
	ExecTime int32  `xorm:"'exectime'"` // 指令执行时间
	Cmd      string `xorm:"'cmd'"`      // 命令
	Param1   string `xorm:"'param1'"`   // 参数1
	Param2   string `xorm:"'param2'"`   // 参数2
	Param3   string `xorm:"'param3'"`   // 参数3
	Param4   string `xorm:"'param4'"`   // 参数4
	Param5   string `xorm:"'param5'"`   // 参数5
}

func (m GmCmd) TableName() string {
	return "gmcmd"
}
