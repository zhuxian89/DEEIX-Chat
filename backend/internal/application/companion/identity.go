package companion

import "strings"

const Name = "小禾"

// Only update the fixed introduction in cached greetings; user history stays intact.
func currentGreeting(greeting string) string {
	return strings.Replace(greeting, "我是小伴，一个 AI 聊天伙伴。", "我是"+Name+"，一个 AI 聊天伙伴。", 1)
}
