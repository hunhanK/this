/**
 * @Author: DaiGuanYu
 * @Desc:
 * @Date: 2022/11/1 12:00
 */

package pyy

type YYA struct {
	PlayerYYBase
}

//func (yy *YYA) getData() *pb3.YYTestA {
//	datas := yy.GetYYData()
//
//	if nil == datas.YyTest {
//		datas.YyTest = make(map[uint32]*pb3.YYTestA)
//	}
//
//	tmp, _ := datas.YyTest[yy.Id]
//	if tmp == nil {
//		tmp = &pb3.YYTestA{}
//		datas.YyTest[yy.Id] = tmp
//	}
//
//	return tmp
//}

func (yy *YYA) OnOpen() {
	yy.LogWarn("测试活动打开")
}
func (yy *YYA) OnEnd() {
	yy.LogWarn("测试活动关闭")
}
func (yy *YYA) Login()  {}
func (yy *YYA) NewDay() {}

func (yy *YYA) c2sTest() {

}

func init() {
	//RegPlayerYY(yydefine.SiPlayerYYTest, func() iface.IPlayerYY {
	//	return &YYA{}
	//})

	//net.RegisterYYSysProto(10000, 10000, (*YYA).c2sTest)
}
