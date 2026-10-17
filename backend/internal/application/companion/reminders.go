package companion

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
)

type ReminderStore interface {
	ReminderSnapshot(context.Context, uint, string, time.Time) (domain.ReminderSnapshot, error)
	ExecuteReminder(context.Context, uint, domain.ReminderCommand, time.Time) (domain.ReminderResult, error)
	TickReminders(context.Context, time.Time, string) error
	// The boolean reports whether another candidate batch may remain.
	ClaimReminderNotification(context.Context, time.Time, string) (*domain.NotificationDelivery, bool, error)
	FinishReminderNotification(context.Context, string, string) error
}

func (s *Service) Reminders(ctx context.Context, user uint, conversationID string) (domain.ReminderSnapshot, error) {
	store, ok := s.Store.(ReminderStore)
	if !ok {
		return domain.ReminderSnapshot{}, ErrNotFound
	}
	return store.ReminderSnapshot(ctx, user, conversationID, time.Now())
}

func (s *Service) ReminderCommand(ctx context.Context, user uint, c domain.ReminderCommand) (domain.ReminderResult, error) {
	store, ok := s.Store.(ReminderStore)
	if !ok {
		return domain.ReminderResult{}, ErrNotFound
	}
	if len(c.RequestID) < 8 || len(c.RequestID) > 100 || len([]rune(c.Input)) > 8000 {
		return domain.ReminderResult{}, ErrInvalid
	}
	p, err := s.Store.Profile(ctx, user)
	if err != nil {
		return domain.ReminderResult{}, err
	}
	if c.ConversationID != "" && c.ConversationID != p.ConversationPublicID {
		return domain.ReminderResult{}, ErrNotFound
	}
	c.ConversationID = p.ConversationPublicID
	now := time.Now()
	if c.Action == "" {
		state, err := store.ReminderSnapshot(ctx, user, c.ConversationID, now)
		if err != nil {
			return domain.ReminderResult{}, err
		}
		parsed := ParseReminderCommand(c.Input, state, now)
		if parsed.Action == "" {
			return domain.ReminderResult{Handled: false}, nil
		}
		parsed.RequestID, parsed.ConversationID, parsed.Input = c.RequestID, c.ConversationID, c.Input
		c = parsed
	}
	return store.ExecuteReminder(ctx, user, c, now)
}

// The scheduler has no chat/model dependency, profile lease or proactivity gate.
func (s *Service) TickReminders(ctx context.Context) error {
	store, ok := s.Store.(ReminderStore)
	if !ok {
		return nil
	}
	return store.TickReminders(ctx, time.Now(), s.notificationRuntime().TemplateID)
}

// Delivery runs separately so a slow external request cannot delay due events.
func (s *Service) DispatchReminders(ctx context.Context) error {
	return s.dispatchReminders(ctx, 8, 100*time.Millisecond)
}

// Bound both simultaneous requests and request starts. Idle users create no jobs.
func (s *Service) dispatchReminders(ctx context.Context, workers int, spacing time.Duration) error {
	store, ok := s.Store.(ReminderStore)
	if !ok {
		return nil
	}
	ticker := time.NewTicker(spacing)
	defer ticker.Stop()
	var wg sync.WaitGroup
	var claims sync.Mutex
	claimsStopped := false
	stopClaims := make(chan struct{})
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 128; n++ {
				select {
				case <-ctx.Done():
					return
				case <-stopClaims:
					return
				case <-ticker.C:
				}
				claims.Lock()
				if claimsStopped {
					claims.Unlock()
					return
				}
				notification := s.notificationRuntime()
				if notification.Sender == nil || notification.TemplateID == "" {
					claimsStopped = true
					close(stopClaims)
					claims.Unlock()
					return
				}
				d, more, err := store.ClaimReminderNotification(ctx, time.Now(), notification.TemplateID)
				if err != nil || (d == nil && !more) {
					claimsStopped = true
					close(stopClaims)
				}
				claims.Unlock()
				if err != nil {
					errCh <- err
					return
				}
				if d == nil {
					if !more {
						return
					}
					continue
				}
				status := "uncertain"
				sendCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
				err = notification.Sender.Send(sendCtx, *d)
				cancel()
				if err == nil {
					status = "sent"
				}
				finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				err = store.FinishReminderNotification(finishCtx, d.ID, status)
				finishCancel()
				if err != nil {
					errCh <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if ctx.Err() != nil {
		errs = append(errs, ctx.Err())
	}
	return errors.Join(errs...)
}

var durationRE = regexp.MustCompile(`([0-9一二两三四五六七八九十百零〇半.]+)\s*个?\s*(小时|分钟|分|天|周)`)
var clockRE = regexp.MustCompile(`(凌晨|早上|早晨|上午|中午|下午|傍晚|晚上|晚间|早|晚)?\s*([0-9一二两三四五六七八九十零〇]+)\s*(?:点|时|:|：)\s*(半|[0-9一二两三四五六七八九十零〇]+)?\s*分?`)
var dateRE = regexp.MustCompile(`(?:(\d{4})[-年/])?(\d{1,2})[-月/](\d{1,2})[日号]?`)
var monthDayRE = regexp.MustCompile(`每月\s*([0-9一二两三四五六七八九十]+)\s*[日号]`)
var indexRE = regexp.MustCompile(`第\s*([0-9一二两三四五六七八九十]+)\s*[个条项]`)
var reminderTitleRE = regexp.MustCompile(`"([^"]*)"|“([^”]*)”|「([^」]*)」`)

func chineseNumber(s string) (float64, bool) {
	if s == "半" {
		return .5, true
	}
	if n, e := strconv.ParseFloat(s, 64); e == nil {
		return n, true
	}
	digits := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	n, part := 0, 0
	for _, r := range s {
		if r == '十' || r == '百' {
			unit := 10
			if r == '百' {
				unit = 100
			}
			if part == 0 {
				part = 1
			}
			n += part * unit
			part = 0
		} else if d, ok := digits[r]; ok {
			part = part*10 + d
		} else {
			return 0, false
		}
	}
	return float64(n + part), s != ""
}

func reminderDuration(text string) (int, bool) {
	minutes, _, ok := reminderDurationSpan(text)
	return minutes, ok
}

// Only adjacent duration parts belong to the same relative time. A duration
// inside the task itself (such as 跳绳十分钟) must remain part of its title.
func reminderDurationSpan(text string) (int, string, bool) {
	idx := durationRE.FindStringIndex(text)
	if idx == nil {
		return 0, "", false
	}
	rest := text[idx[0]:]
	used := 0
	total := float64(0)
	for {
		m := durationRE.FindStringSubmatchIndex(rest)
		if m == nil || m[0] != 0 {
			break
		}
		n, ok := chineseNumber(rest[m[2]:m[3]])
		if !ok {
			return 0, "", false
		}
		mult := float64(1)
		switch rest[m[4]:m[5]] {
		case "小时":
			mult = 60
		case "天":
			mult = 1440
		case "周":
			mult = 10080
		}
		total += n * mult
		used += m[1]
		rest = rest[m[1]:]
		trimmed := strings.TrimLeft(rest, " \t")
		used += len(rest) - len(trimmed)
		rest = trimmed
	}
	return int(total), text[idx[0] : idx[0]+used], total >= 1 && total <= 366*1440 && total == float64(int(total))
}

func reminderContains(s string, parts ...string) bool {
	for _, p := range parts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}
func cleanReminderTarget(s string) string {
	for _, p := range []string{"帮我", "麻烦", "请", "小禾", "一下", "这个", "那个", "这条", "那条", "我的", "提醒事项", "定时提醒", "提醒", "闹钟", "倒计时", "还有多久", "还剩多久", "什么时候", "几点", "哪些", "所有", "全部", "查看", "查询", "看看", "列出", "取消", "删除", "删掉", "删了", "不再", "暂停", "恢复", "继续", "推迟", "延后", "稍后", "已经", "已完成", "完成了", "完成", "跳过", "本次", "这次", "今天", "的", "把", "了"} {
		s = strings.ReplaceAll(s, p, "")
	}
	return strings.Trim(s, " ，。！？?！、\n\t")
}
func matchReminderTitle(s string) string {
	s = strings.ReplaceAll(s, "喝药", "吃药")
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "陪伴", "陪")
}

func quotedReminderTitle(text string) (title, command, reason string) {
	const quotes = "\"“”「」"
	if !strings.ContainsAny(text, quotes) {
		return "", text, ""
	}
	matches := reminderTitleRE.FindAllStringSubmatch(text, -1)
	command = reminderTitleRE.ReplaceAllString(text, " ")
	if len(matches) != 1 || strings.ContainsAny(command, quotes) {
		return "", command, "请用一对完整的引号标出一个提醒名称，例如：把“为了测试”提醒改到3分钟后。"
	}
	for _, part := range matches[0][1:] {
		if strings.TrimSpace(part) != "" {
			return strings.TrimSpace(part), command, ""
		}
	}
	return "", command, "引号内需要填写提醒名称。"
}

// Preserve complete names before stripping conversational words such as 的、了、把.
// A quoted name is literal and must never fall back to another reminder.
func namedReminderMatches(text, quotedTitle string, plans []domain.Reminder) []domain.Reminder {
	var matches []domain.Reminder
	for _, p := range plans {
		if p.Status == "deleted" || p.Title == "" {
			continue
		}
		if quotedTitle != "" {
			if p.Title == quotedTitle {
				matches = append(matches, p)
			}
		} else if strings.Contains(matchReminderTitle(text), matchReminderTitle(p.Title)) {
			matches = append(matches, p)
		}
	}
	return matches
}

func ParseReminderCommand(input string, state domain.ReminderSnapshot, now time.Time) domain.ReminderCommand {
	text := strings.TrimSpace(input)
	var last *domain.ReminderTurn
	for i := len(state.Turns) - 1; i >= 0; i-- {
		if state.Turns[i].Input != "" && now.Sub(state.Turns[i].CreatedAt) < 30*time.Minute {
			last = &state.Turns[i]
			break
		}
	}
	// Continue a time clarification, retaining the actual original request.
	if last != nil && strings.HasPrefix(last.Reply, "还需要") && !reminderContains(text, "取消", "删除", "暂停", "查询", "有哪些") && len([]rune(text)) < 40 {
		if clockRE.MatchString(text) || durationRE.MatchString(text) {
			text = last.Input + " " + text
		}
	}
	quotedTitle, text, quoteReason := quotedReminderTitle(text)
	// A common dictated typo is a duration followed by 小时候, not childhood.
	text = regexp.MustCompile(`([0-9一二两三四五六七八九十半])个?小时候`).ReplaceAllString(text, "${1}小时后")
	halfHour := regexp.MustCompile(`([0-9一二两三四五六七八九十]+)个?(?:半小时|小时半)`)
	text = halfHour.ReplaceAllStringFunc(text, func(v string) string {
		m := halfHour.FindStringSubmatch(v)
		n, _ := chineseNumber(m[1])
		return strconv.FormatFloat(n+.5, 'f', -1, 64) + "小时"
	})
	text = regexp.MustCompile(`早晚\s*([0-9一二两三四五六七八九十]+)点`).ReplaceAllString(text, "早上${1}点和晚上${1}点")
	if text == "" || reminderContains(text, "比如", "例如", "举个例子", "假如", "假设", "如果我说", "怎么实现", "如何实现", "代码") {
		return domain.ReminderCommand{}
	}
	action := ""
	switch {
	case reminderContains(text, "删除", "删掉", "删了", "取消", "不再提醒", "不要提醒", "不用提醒", "别提醒"):
		action = "delete"
	case strings.Contains(text, "暂停"):
		action = "pause"
	case reminderContains(text, "恢复", "继续提醒"):
		action = "resume"
	case reminderContains(text, "推迟", "延后", "稍后", "晚点提醒"):
		action = "snooze"
	case reminderContains(text, "跳过", "今天不提醒"):
		action = "skip"
	case reminderContains(text, "已完成", "完成了", "已经吃药", "已经喝药", "吃完药", "喝完药", "完成提醒"):
		action = "complete"
	case reminderContains(text, "改成", "改到", "改为", "修改"):
		action = "update"
	case reminderContains(text, "倒计时", "还有多久", "还剩多久", "有哪些提醒", "有什么提醒", "所有提醒", "查看提醒", "查询提醒", "提醒列表", "提醒什么时候", "提醒几点") || strings.Contains(text, "提醒") && reminderContains(text, "查询", "查看", "看看", "哪些", "什么时候", "几点"):
		action = "list"
	}
	if action != "" {
		// Do not intercept unrelated delete/edit requests or ordinary completion reports.
		potential := reminderContains(text, "提醒", "闹钟", "倒计时", "多久")
		if len(namedReminderMatches(text, quotedTitle, state.Plans)) > 0 {
			potential = true
		}
		if last != nil && len(last.ReminderIDs) == 1 && reminderContains(text, "这个", "这条", "那个", "改成", "推迟", "已完成", "暂停", "恢复") {
			potential = true
		}
		if !potential {
			return domain.ReminderCommand{}
		}
		if quoteReason != "" {
			return domain.ReminderCommand{Action: "clarify", Reply: quoteReason}
		}
		c := domain.ReminderCommand{Action: action}
		if action == "list" {
			for _, p := range state.Plans {
				if p.Status != "deleted" {
					c.IDs = append(c.IDs, p.ID)
				}
			}
			matches := []string{}
			query := matchReminderTitle(cleanReminderTarget(text))
			for _, p := range namedReminderMatches(text, quotedTitle, state.Plans) {
				matches = append(matches, p.ID)
			}
			if len(matches) == 0 && quotedTitle == "" {
				for _, p := range state.Plans {
					if p.Status != "deleted" && query != "" && strings.Contains(matchReminderTitle(p.Title), query) {
						matches = append(matches, p.ID)
					}
				}
			}
			if len(matches) > 0 {
				c.IDs = matches
			} else if quotedTitle != "" {
				return domain.ReminderCommand{Action: "clarify", Reply: fmt.Sprintf("没有找到名为“%s”的提醒。可以说“查看所有提醒”。", quotedTitle)}
			} else if query != "" && !reminderContains(query, "现在", "目前", "都", "有", "是") {
				return domain.ReminderCommand{Action: "clarify", Reply: "没有找到这个提醒。可以说“查看所有提醒”。"}
			}
			return c
		}
		query := text
		if action == "update" {
			for _, sep := range []string{"改成", "改到", "改为", "修改"} {
				if idx := strings.Index(query, sep); idx >= 0 {
					query = query[:idx]
					break
				}
			}
		}
		if action == "snooze" {
			query = durationRE.ReplaceAllString(query, "")
		}
		matches := namedReminderMatches(query, quotedTitle, state.Plans)
		if len(matches) == 0 && quotedTitle != "" {
			return domain.ReminderCommand{Action: "clarify", Reply: fmt.Sprintf("没有找到名为“%s”的提醒。可以说“查看所有提醒”。", quotedTitle)}
		}
		query = cleanReminderTarget(query)
		if len(matches) == 0 {
			for _, p := range state.Plans {
				if p.Status == "deleted" {
					continue
				}
				if query != "" && (strings.Contains(matchReminderTitle(p.Title), matchReminderTitle(query)) || strings.Contains(matchReminderTitle(query), matchReminderTitle(p.Title))) {
					matches = append(matches, p)
				}
			}
		}
		if len(matches) == 0 && last != nil {
			ids := last.ReminderIDs
			if m := indexRE.FindStringSubmatch(text); m != nil {
				n, _ := chineseNumber(m[1])
				if int(n) > 0 && int(n) <= len(ids) {
					ids = []string{ids[int(n)-1]}
				}
			}
			if len(ids) == 1 && (query == "" || reminderContains(text, "这个", "这条", "那个", "第", "已完成", "改成", "推迟")) {
				for _, p := range state.Plans {
					if p.ID == ids[0] && p.Status != "deleted" {
						matches = append(matches, p)
					}
				}
			}
		}
		if len(matches) == 0 && query == "" {
			for _, p := range state.Plans {
				if p.Status == "active" || p.Status == "paused" {
					matches = append(matches, p)
				}
			}
		}
		if len(matches) != 1 {
			c.Action = "clarify"
			c.Reply = "请说出要操作的提醒名称。"
			if len(matches) > 1 {
				c.Reply = "有多条符合的提醒，请说出完整名称，或点击对应提醒操作。"
			}
			for _, p := range matches {
				c.IDs = append(c.IDs, p.ID)
			}
			return c
		}
		p := matches[0]
		c.ID, c.Revision, c.Title = p.ID, p.Revision, p.Title
		c.IDs = []string{p.ID}
		if action == "snooze" {
			minutes, ok := reminderDuration(text)
			if !ok {
				c.Action = "clarify"
				c.Reply = "还需要推迟多久，例如“把吃药提醒推迟十分钟”。"
			} else {
				c.DelayMinutes = minutes
			}
			return c
		}
		if action == "update" {
			schedule, _, reason := parseReminderSchedule(text, now)
			if reason != "" {
				c.Action = "clarify"
				c.Reply = reason
				return c
			}
			if schedule.Kind == "once" && p.Schedule.Kind != "once" && !reminderContains(text, "今天", "明天", "后天", "分钟后", "小时后", "不重复", "单次") {
				old := p.Schedule
				old.Times = schedule.Times
				old.StartAt = now.Truncate(time.Minute)
				schedule = old
			}
			c.Schedule = schedule
		}
		return c
	}
	explicit := reminderContains(text, "提醒我", "提醒一下", "提醒下", "叫我", "闹钟", "定个提醒", "设置提醒", "添加提醒")
	implicit := strings.HasPrefix(text, "我") && reminderContains(text, "后需要", "后要", "之后需要") && durationRE.MatchString(text)
	if !explicit && !implicit {
		if reminderContains(text, "提醒", "闹钟") && (clockRE.MatchString(text) || durationRE.MatchString(text) || reminderContains(text, "设置", "添加", "每天")) {
			return domain.ReminderCommand{Action: "clarify", Reply: "还需要明确的事项和时间，例如“每天晚上八点提醒我陪孩子写作业”。"}
		}
		return domain.ReminderCommand{}
	}
	if quoteReason != "" {
		return domain.ReminderCommand{Action: "clarify", Reply: quoteReason}
	}
	schedule, rest, reason := parseReminderSchedule(text, now)
	if reason != "" {
		return domain.ReminderCommand{Action: "clarify", Reply: reason}
	}
	if len([]rune(text)) > 1000 {
		return domain.ReminderCommand{Action: "clarify", Reply: "请单独说出要提醒的事情和时间。"}
	}
	for _, part := range []string{"提醒我", "提醒一下", "提醒下", "定个提醒", "设置提醒", "添加提醒", "叫我", "闹钟", "到时候", "届时"} {
		rest = strings.ReplaceAll(rest, part, "")
	}
	for {
		old := rest
		rest = strings.Trim(rest, " ，。！？、\n\t")
		for _, pre := range []string{"小禾", "请", "麻烦", "帮我", "我", "在", "需要", "要", "去", "后", "之后", "以后"} {
			rest = strings.TrimPrefix(rest, pre)
		}
		if rest == old {
			break
		}
	}
	if quotedTitle != "" {
		rest = quotedTitle
	}
	if rest == "" || len([]rune(rest)) > 120 {
		return domain.ReminderCommand{Action: "clarify", Reply: "还需要提醒的事情，例如“一个小时后提醒我吃药”。"}
	}
	return domain.ReminderCommand{Action: "create", Title: rest, Schedule: schedule}
}

func parseReminderSchedule(text string, now time.Time) (domain.ReminderSchedule, string, string) {
	s := domain.ReminderSchedule{Kind: "once", StartAt: now.Truncate(time.Minute)}
	rest := text
	local := now.In(domain.ReminderZone)
	if strings.Contains(text, "每隔") {
		minutes, span, ok := reminderDurationSpan(text)
		if !ok {
			return s, rest, "还需要有效的提醒间隔，例如“每隔两小时提醒我喝水”。"
		}
		s.Kind = "interval"
		s.IntervalMinutes = minutes
		s.StartAt = now.Add(time.Duration(minutes) * time.Minute)
		rest = strings.Replace(strings.Replace(rest, "每隔", "", 1), span, "", 1)
		return s, rest, ""
	}
	if m := durationRE.FindString(text); m != "" && reminderContains(text, "后", "之后", "以后") && !reminderContains(text, "每天", "每周", "每月", "每年") {
		minutes, span, ok := reminderDurationSpan(text)
		if !ok {
			return s, rest, "还需要有效的时间，例如“一个小时后”。"
		}
		s.StartAt = now.Add(time.Duration(minutes) * time.Minute)
		rest = strings.Replace(rest, span, "", 1)
		rest = strings.Replace(rest, "之后", "", 1)
		rest = strings.Replace(rest, "以后", "", 1)
		rest = strings.Replace(rest, "后", "", 1)
		return s, rest, ""
	}
	if reminderContains(text, "每天", "每日") {
		s.Kind = "daily"
		rest = strings.ReplaceAll(strings.ReplaceAll(rest, "每天", ""), "每日", "")
	}
	if strings.Contains(text, "每周") || strings.Contains(text, "周一至周五") || strings.Contains(text, "工作日") || strings.Contains(text, "周末") {
		s.Kind = "weekly"
		if reminderContains(text, "工作日", "周一至周五") {
			s.Weekdays = []int{1, 2, 3, 4, 5}
		} else if strings.Contains(text, "周末") {
			s.Weekdays = []int{0, 6}
		} else {
			m := regexp.MustCompile(`每周([一二三四五六日天、，,和及 ]+)`).FindStringSubmatch(text)
			if m != nil {
				for _, r := range m[1] {
					if i := strings.IndexRune("日一二三四五六", r); i >= 0 {
						s.Weekdays = append(s.Weekdays, len([]rune("日一二三四五六"[:i])))
					} else if r == '天' {
						s.Weekdays = append(s.Weekdays, 0)
					}
				}
				rest = strings.Replace(rest, m[0], "", 1)
			}
		}
		for _, v := range []string{"每周一至周五", "工作日", "周一至周五", "周末"} {
			rest = strings.ReplaceAll(rest, v, "")
		}
	}
	if strings.Contains(text, "每月") {
		s.Kind = "monthly"
		if reminderContains(text, "月底", "最后一天") {
			s.Day = -1
			rest = strings.ReplaceAll(strings.ReplaceAll(rest, "每月月底", ""), "每月最后一天", "")
		} else if m := monthDayRE.FindStringSubmatch(text); m != nil {
			n, _ := chineseNumber(m[1])
			s.Day = int(n)
			rest = strings.Replace(rest, m[0], "", 1)
		}
	}
	if strings.Contains(text, "每年") {
		s.Kind = "yearly"
		rest = strings.ReplaceAll(rest, "每年", "")
	}
	clocks := clockRE.FindAllStringSubmatch(text, -1)
	if len(clocks) == 0 {
		return s, rest, "还需要具体时间，例如“每天晚上八点提醒我陪孩子写作业”。"
	}
	for _, m := range clocks {
		h, ok := chineseNumber(m[2])
		minute := float64(0)
		if m[3] == "半" {
			minute = 30
		} else if m[3] != "" {
			minute, _ = chineseNumber(m[3])
		}
		if !ok || h < 0 || h > 23 || minute < 0 || minute > 59 {
			return s, rest, "还需要有效的时间，小时为0至23，分钟为0至59。"
		}
		if reminderContains(m[1], "下午", "晚上", "傍晚", "晚间", "晚") && h < 12 {
			h += 12
		}
		if m[1] == "中午" && h < 11 {
			h += 12
		}
		if reminderContains(m[1], "凌晨", "早上", "早晨", "上午", "早") && h == 12 {
			h = 0
		}
		s.Times = append(s.Times, int(h)*60+int(minute))
		rest = strings.Replace(rest, m[0], "", 1)
	}
	dayOffset := 0
	if strings.Contains(text, "明天") {
		dayOffset = 1
	}
	if strings.Contains(text, "后天") {
		dayOffset = 2
	}
	year, month, day := local.Year(), local.Month(), local.Day()
	dateExplicit := dayOffset > 0 || strings.Contains(text, "今天")
	if m := dateRE.FindStringSubmatch(text); m != nil {
		dateExplicit = true
		if m[1] != "" {
			year, _ = strconv.Atoi(m[1])
		}
		mo, _ := strconv.Atoi(m[2])
		day, _ = strconv.Atoi(m[3])
		month = time.Month(mo)
		rest = strings.Replace(rest, m[0], "", 1)
	}
	base := time.Date(year, month, day, 0, 0, 0, 0, domain.ReminderZone)
	if base.Year() != year || base.Month() != month || base.Day() != day {
		return s, rest, "还需要有效的日期。"
	}
	base = base.AddDate(0, 0, dayOffset)
	if s.Kind == "yearly" {
		s.Month, s.Day = int(month), day
		if !dateExplicit {
			return s, rest, "还需要每年的月日，例如“每年10月18日上午八点提醒我交保险”。"
		}
	}
	if s.Kind == "once" {
		if len(s.Times) != 1 {
			return s, rest, "还需要明确是否每天重复；单次提醒请指定一个时间。"
		}
		s.StartAt = base.Add(time.Duration(s.Times[0]) * time.Minute)
		if !s.StartAt.After(now) && !dateExplicit {
			s.StartAt = s.StartAt.AddDate(0, 0, 1)
		}
		if !s.StartAt.After(now) {
			return s, rest, "这个时间已经过去，请指定未来的日期和时间。"
		}
	} else if dayOffset > 0 {
		s.StartAt = base
	}
	for _, v := range []string{"今天", "明天", "后天", "从现在开始", "开始", "每天", "每日", "每周", "每月", "每年"} {
		rest = strings.ReplaceAll(rest, v, "")
	}
	if !s.Validate() {
		return s, rest, "还需要完整的重复规则和有效时间。"
	}
	return s, strings.Trim(rest, " 、，和"), ""
}

func reminderContextText(snapshot domain.ReminderSnapshot) string {
	var b strings.Builder
	b.WriteString("\n提醒由专用功能实际保存。不得声称已创建、修改或删除未经执行的提醒。以下是当前已保存的提醒；没有记录就不能说已经安排：\n")
	for _, p := range snapshot.Plans {
		if p.Status == "active" || p.Status == "paused" {
			fmt.Fprintf(&b, "%s；%s；状态%s；下次%s\n", p.Title, p.Schedule.Description(), p.Status, domain.ReminderTime(p.NextAt))
		}
	}
	return b.String()
}
