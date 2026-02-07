/**
 * @Author: ChenJunJi
 * @Date: 2023/11/08
 * @Desc:
**/

package model

type ActorSeries struct {
	Id       uint32 `xorm:"pk 'id'"`
	ServerId uint32 `xorm:"'server_id'"`
	Series   uint32 `xorm:"'series'"`
}

func (m ActorSeries) TableName() string {
	return "actor_series"
}
