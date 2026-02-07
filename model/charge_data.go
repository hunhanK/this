/**
 * @Author: ChenJunJi
 * @Date: 2023/11/07
 * @Desc:
**/

package model

type Charge struct {
	Id         uint64 `xorm:"pk 'id'"`
	AccountId  uint64 `xorm:"'account_id'"`
	ActorId    uint64 `xorm:"'actor_id'"`
	ChargeId   uint32 `xorm:"'charge_id'"`
	CashNum    uint32 `xorm:"'cash_num'"`
	CheckTime  int32  `xorm:"'check_time'"`
	InsertTime int32  `xorm:"'insert_time'"`
	PayNo      string `xorm:"'pay_no'"`
	CpNo       string `xorm:"'cp_no'"`
}

func (m Charge) TableName() string {
	return "charge"
}
