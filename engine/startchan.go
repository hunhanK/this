/**
 * @Author: zjj
 * @Date: 2025/6/3
 * @Desc:
**/

package engine

import "sync"

var localFightConnChannel = make(chan struct{})
var gateConnChannel = make(chan struct{})
var onceLocalFightConn sync.Once
var onceGateConn sync.Once

func WaitLocalFightConn() {
	<-localFightConnChannel
	return
}

func WaitGateConn() {
	<-gateConnChannel
	return
}

func NotifyLocalFightConn() {
	onceLocalFightConn.Do(func() {
		localFightConnChannel <- struct{}{}
	})
	return
}

func NotifyGateConn() {
	onceGateConn.Do(func() {
		gateConnChannel <- struct{}{}
	})
	return
}
