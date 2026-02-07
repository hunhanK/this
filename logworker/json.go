package logworker

import "encoding/json"

func ConvertJsonStr(m map[string]interface{}) string {
	logArg, _ := json.Marshal(m)
	return string(logArg)
}
