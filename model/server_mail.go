/**
 * @Author: ChenJunJi
 * @Date: 2023/11/09
 * @Desc:
**/

package model

import "jjyz/base/db"

type ServerMail struct {
	Id       uint64 `xorm:"pk 'id'"`
	ConfId   uint16 `xorm:"'conf_id'"`
	Type     int32  `xorm:"'type'"`
	SendTick uint32 `xorm:"'send_tick'"`
	Title    string `xorm:"'title'"`
	Content  string `xorm:"'content'"`
	AwardStr string `xorm:"'award_str'"`
}

func (m ServerMail) TableName() string {
	return "server_mail"
}

func LoadServerMail() ([]*ServerMail, error) {
	var ret []*ServerMail
	err := db.OrmEngine.OrderBy("id asc").Find(&ret)
	return ret, err
}
