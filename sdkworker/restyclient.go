/**
 * @Author: zjj
 * @Date: 2024/3/14
 * @Desc: API
**/

package sdkworker

import "github.com/go-resty/resty/v2"

type RestyClient struct {
	*resty.Client
}

var defaultRestyClient *RestyClient

func init() {
	defaultRestyClient = NewRestyClient()
}

func NewRestyClient() *RestyClient {
	return &RestyClient{
		resty.New(),
	}
}

func NewRequest() *resty.Request {
	return defaultRestyClient.R()
}
