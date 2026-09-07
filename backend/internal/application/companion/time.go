package companion

import (
	"encoding/json"
	"time"
)

// Beijing uses UTC+8 year-round; this also works in images without zoneinfo.
var beijing = time.FixedZone("Asia/Shanghai", 8*60*60)

type clockContext struct {
	Now     string `json:"now_beijing"`
	Weekday string `json:"weekday"`
	Period  string `json:"period"`
}

func clockAt(now time.Time) clockContext {
	local := now.In(beijing)
	period := "深夜"
	switch hour := local.Hour(); {
	case hour >= 5 && hour < 9:
		period = "早晨"
	case hour >= 9 && hour < 11:
		period = "上午"
	case hour >= 11 && hour < 14:
		period = "午间"
	case hour >= 14 && hour < 18:
		period = "下午"
	case hour >= 18 && hour < 23:
		period = "晚上"
	}
	weekdays := [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	return clockContext{Now: local.Format(time.RFC3339), Weekday: weekdays[local.Weekday()], Period: period}
}

func beijingTimestamp(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.In(beijing).Format(time.RFC3339)
}

func continuityPrompt(now, lastUserAt time.Time, topics []Topic) string {
	data := map[string]interface{}{"recent_public_topics": topicPromptData(topics)}
	if !lastUserAt.IsZero() && !lastUserAt.After(now) {
		local, previous := now.In(beijing), lastUserAt.In(beijing)
		today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, beijing)
		lastDay := time.Date(previous.Year(), previous.Month(), previous.Day(), 0, 0, 0, 0, beijing)
		data["last_user_message_beijing"] = beijingTimestamp(lastUserAt)
		data["minutes_since_last_user_message"] = int(now.Sub(lastUserAt).Minutes())
		data["calendar_days_since_last_user_message"] = int(today.Sub(lastDay).Hours() / 24)
	}
	encoded, _ := json.Marshal(data)
	return "\n<conversation_context>\n" + string(encoded) + "\n</conversation_context>"
}

func timeGreeting(now time.Time) string {
	switch clockAt(now).Period {
	case "早晨":
		return "早呀。"
	case "上午":
		return "上午好。"
	case "午间":
		return "中午好。"
	case "下午":
		return "下午好。"
	case "晚上":
		return "晚上好。"
	default:
		return "夜深了，你来啦。"
	}
}

func greetingForTime(p *Profile, now time.Time) string {
	opening := timeGreeting(now)
	if p.GreetingAt.IsZero() {
		return opening + "我是小伴，一个 AI 聊天伙伴。不用想好问题，随口说点什么也可以。"
	}
	choices := map[string][]string{
		"早晨": {"新的一天，想从一件小事聊起吗？", "今天想慢慢来，还是已经有期待的事了？"},
		"上午": {"这会儿有什么想随口说说的？", "想接着上次聊，还是换个轻松的话题？"},
		"午间": {"想聊点轻松的，给这会儿添点趣味吗？", "今天到现在，有没有一件想分享的小事？"},
		"下午": {"这会儿想分享点什么，或者吐槽两句？", "下午也可以留一点空隙，随便聊聊。"},
		"晚上": {"今天有没有一件让你想多说两句的事？", "这会儿可以聊聊今天，也可以说点别的。"},
		"深夜": {"想说点什么，我在听。不急着把话理清楚。", "想安静聊一会儿也可以，不用特意找话题。"},
	}
	options := choices[clockAt(now).Period]
	index := (now.In(beijing).YearDay() + int(p.UserID%2)) % len(options)
	if opening+options[index] == p.LastGreeting {
		index = (index + 1) % len(options)
	}
	return opening + options[index]
}
