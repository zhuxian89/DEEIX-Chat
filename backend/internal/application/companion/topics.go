package companion

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	domainchat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/google/uuid"
)

// topicData preserves the private cache and prompt encoding. HTTP uses transport DTOs.
type topicData struct {
	Category    string    `json:"category"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Opener      string    `json:"opener"`
	PublishedAt time.Time `json:"publishedAt"`
	FetchedAt   time.Time `json:"fetchedAt"`
}

type TopicProvider interface {
	Search(context.Context, uint, string, string, time.Time) ([]Topic, error)
}

type topicCategory struct {
	ID       string
	Label    string
	Keywords []string
}

var topicCategories = []topicCategory{
	{ID: "science", Label: "科技与科普", Keywords: []string{"科技", "科普", "科学", "天文", "航天", "人工智能", "机器人"}},
	{ID: "film", Label: "电影与剧集", Keywords: []string{"电影", "影视", "电视剧", "剧集", "综艺", "追剧"}},
	{ID: "music", Label: "音乐", Keywords: []string{"音乐", "爵士", "摇滚", "演唱会", "专辑", "钢琴", "吉他"}},
	{ID: "games", Label: "游戏", Keywords: []string{"游戏", "主机", "任天堂", "电竞"}},
	{ID: "sports", Label: "体育", Keywords: []string{"体育", "篮球", "足球", "网球", "运动", "跑步", "游泳"}},
	{ID: "culture", Label: "文化与旅行", Keywords: []string{"文化", "旅行", "旅游", "博物馆", "摄影", "美食", "读书", "文学"}},
}

func categoryByID(id string) (topicCategory, bool) {
	for _, category := range topicCategories {
		if category.ID == id {
			return category, true
		}
	}
	return topicCategory{}, false
}

func containsAny(value string, words []string) bool {
	for _, word := range words {
		if strings.Contains(value, word) {
			return true
		}
	}
	return false
}

func avoidsTopic(value string) bool {
	return containsAny(value, []string{"不喜欢", "不爱", "不感兴趣", "没兴趣", "不想聊", "不关注", "少聊", "别提", "不要推荐"})
}

// Explicit preferences are strong signals. Merely mentioning a subject is weak.
// Newer statements override older ones, including a user's manual correction.
func interestScores(memories []Memory, now time.Time) map[string]int {
	ordered := append([]Memory(nil), memories...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].UpdatedAt.Before(ordered[j].UpdatedAt) })
	scores := make(map[string]int)
	for _, memory := range ordered {
		if !memory.ExpiresAt.After(now) {
			continue
		}
		mentionsCategory := false
		for _, category := range topicCategories {
			mentionsCategory = mentionsCategory || containsAny(memory.Value, category.Keywords)
		}
		for _, category := range topicCategories {
			manual := memory.Key == "topic:"+category.ID && !mentionsCategory
			value := strings.NewReplacer("但是", "，", "不过", "，", "但", "，").Replace(memory.Value)
			for _, statement := range strings.FieldsFunc(value, func(r rune) bool {
				return strings.ContainsRune("，,。；;！!\n", r)
			}) {
				if !manual && !containsAny(statement, category.Keywords) {
					continue
				}
				if avoidsTopic(statement) {
					scores[category.ID] = -100
				} else if manual || containsAny(statement, []string{"喜欢", "爱看", "爱听", "感兴趣", "关注", "偏好"}) {
					scores[category.ID] = 20
				} else if scores[category.ID] >= 0 && scores[category.ID] < 2 {
					scores[category.ID] = 2
				}
			}
		}
	}
	return scores
}

func rankedCategories(memories []Memory, userID uint, now time.Time, query string) []topicCategory {
	scores := interestScores(memories, now)
	eligible := make([]topicCategory, 0, len(topicCategories))
	for _, category := range topicCategories {
		if scores[category.ID] < 0 {
			continue
		}
		if containsAny(query, category.Keywords) {
			scores[category.ID] += 4
		}
		eligible = append(eligible, category)
	}
	if len(eligible) == 0 {
		return eligible
	}
	day := (now.Unix() + 8*60*60) / int64((24 * time.Hour).Seconds())
	offset := (int(userID%1000) + int(day)) % len(eligible)
	eligible = append(eligible[offset:], eligible[:offset]...)
	// One out of five user-days explores an allowed category. Rejections always win.
	if (day+int64(userID))%5 != 0 || query != "" {
		sort.SliceStable(eligible, func(i, j int) bool { return scores[eligible[i].ID] > scores[eligible[j].ID] })
	}
	return eligible
}

func rankTopics(topics []Topic, memories []Memory, p *Profile, now time.Time, query string) []Topic {
	categories := rankedCategories(memories, p.UserID, now, query)
	result := make([]Topic, 0, len(topics))
	seen := map[string]bool{}
	for _, category := range categories {
		for _, topic := range topics {
			if topic.Category == category.ID && !seen[topic.URL] && freshTopic(topic, now) {
				result = append(result, topic)
				seen[topic.URL] = true
			}
		}
	}
	return result
}

func freshTopic(topic Topic, now time.Time) bool {
	return validTopicURL(topic.URL) && !topic.PublishedAt.IsZero() && !topic.PublishedAt.After(now) &&
		now.Sub(topic.PublishedAt) <= 48*time.Hour && !topic.FetchedAt.IsZero() &&
		!topic.FetchedAt.After(now) && now.Sub(topic.FetchedAt) <= 2*time.Hour
}

func storedTopic(raw string) *Topic {
	var data topicData
	if json.Unmarshal([]byte(raw), &data) != nil || !validTopicURL(data.URL) {
		return nil
	}
	topic := Topic(data)
	return &topic
}

func seenTopics(raw string) []string {
	var urls []string
	_ = json.Unmarshal([]byte(raw), &urls)
	return urls
}

func rememberTopic(raw, url string) string {
	urls := []string{url}
	for _, previous := range seenTopics(raw) {
		if previous != url && len(urls) < 20 {
			urls = append(urls, previous)
		}
	}
	encoded, _ := json.Marshal(urls)
	return string(encoded)
}

func lastUserMessage(messages []domainchat.Message, floor uint) domainchat.Message {
	var last domainchat.Message
	for _, message := range messages {
		if message.Role == "user" && message.Status == "success" && message.ID > floor && message.ID > last.ID {
			last = message
		}
	}
	return last
}

func (s *Service) topics(ctx context.Context, now time.Time) ([]Topic, error) {
	rows, err := s.Store.TopicCaches(ctx, now.Add(-2*time.Hour), len(topicCategories))
	if err != nil {
		return nil, err
	}
	topics := []Topic{}
	for _, row := range rows {
		var cached []topicData
		if json.Unmarshal([]byte(row.TopicsJSON), &cached) != nil {
			continue
		}
		for _, data := range cached {
			topic := Topic(data)
			if topic.Category == row.Category && freshTopic(topic, now) {
				topics = append(topics, topic)
			}
		}
	}
	return topics, nil
}

func (s *Service) pickGreetingTopic(ctx context.Context, p *Profile, messages []domainchat.Message, now time.Time) *Topic {
	if s.TopicProvider == nil || p.GreetingAt.IsZero() || p.Quiet || clockAt(now).Period == "深夜" || now.Sub(p.LastTopicAt) < 24*time.Hour {
		return nil
	}
	last := lastUserMessage(messages, p.ForgetThroughID)
	if !last.CreatedAt.IsZero() && now.Sub(last.CreatedAt) < 6*time.Hour {
		return nil
	}
	if containsAny(last.Content, []string{"难过", "难受", "失眠", "分手", "焦虑", "痛苦", "生病", "不想活", "伤心", "压力", "累", "不感兴趣", "别推荐", "别推", "换个话题", "别聊这个"}) {
		return nil
	}
	memories, err := s.Store.Memories(ctx, p.UserID)
	if err != nil {
		return nil
	}
	topics, err := s.topics(ctx, now)
	if err != nil {
		return nil
	}
	seen := seenTopics(p.SeenTopicsJSON)
	for _, topic := range rankTopics(topics, memories, p, now, "") {
		alreadySeen := false
		for _, url := range seen {
			alreadySeen = alreadySeen || topic.URL == url
		}
		if !alreadySeen {
			return &topic
		}
	}
	return nil
}

// RefreshTopics is on-demand, with a per-category DB claim shared by all users
// and instances. A failed attempt waits five minutes; success waits one hour.
func (s *Service) RefreshTopics(ctx context.Context, userID uint) error {
	if s.TopicProvider == nil {
		return nil
	}
	p, err := s.Store.Profile(ctx, userID)
	if err != nil || p.Quiet {
		return err
	}
	memories, err := s.Store.Memories(ctx, userID)
	if err != nil {
		return err
	}
	now := time.Now()
	categories := rankedCategories(memories, userID, now, "")
	if len(categories) == 0 {
		return nil
	}
	category := categories[0].ID
	token := uuid.NewString()
	claimed, err := s.Store.ClaimTopics(ctx, category, token, now)
	if err != nil || !claimed {
		return err
	}
	topics, err := s.TopicProvider.Search(ctx, userID, p.Model, category, now)
	if err != nil {
		return err
	}
	valid := []topicData{}
	for _, topic := range topics {
		if topic.Category == category && freshTopic(topic, now) && topic.Opener != "" && len([]rune(topic.Opener)) <= 160 && len(valid) < 3 {
			valid = append(valid, topicData(topic))
		}
	}
	encoded, _ := json.Marshal(valid)
	return s.Store.SaveTopics(ctx, category, token, string(encoded), now)
}

func (s *Service) TopicFeedback(ctx context.Context, userID uint, topicURL, preference string) error {
	if preference != "like" && preference != "avoid" {
		return ErrInvalid
	}
	token, err := s.Store.Acquire(ctx, userID)
	if err != nil {
		return err
	}
	defer s.Store.Release(userID, token)
	p, err := s.Store.Profile(ctx, userID)
	if err != nil {
		return err
	}
	topic := storedTopic(p.LastTopicJSON)
	if topic == nil || topic.URL != topicURL {
		return ErrNotFound
	}
	category, ok := categoryByID(topic.Category)
	if !ok {
		return ErrInvalid
	}
	now := time.Now()
	value := "喜欢聊" + category.Label
	if preference == "avoid" {
		value = "少聊" + category.Label
	}
	memory := Memory{ID: uuid.NewString(), UserID: userID, Key: "topic:" + category.ID, Value: value,
		Evidence: "由你选择：" + value, SourceAt: now, UpdatedAt: now, ExpiresAt: now.Add(90 * 24 * time.Hour)}
	return s.Store.SaveTopicFeedback(ctx, userID, token, memory)
}

func topicPromptData(topics []Topic) []topicData {
	if topics == nil {
		return nil
	}
	data := make([]topicData, len(topics))
	for i, topic := range topics {
		data[i] = topicData(topic)
	}
	return data
}
