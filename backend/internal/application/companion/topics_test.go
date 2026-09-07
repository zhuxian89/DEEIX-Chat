package companion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainchat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
)

type topicProviderStub struct {
	calls atomic.Int32
	fail  bool
}

func (p *topicProviderStub) Search(_ context.Context, _ uint, _, category string, now time.Time) ([]Topic, error) {
	p.calls.Add(1)
	if p.fail {
		return nil, errors.New("offline")
	}
	return []Topic{{Category: category, Title: "公开消息", URL: "https://www.bbc.com/news/topic-test", Opener: "有个新鲜的小话题。", PublishedAt: now.Add(-time.Hour), FetchedAt: now}}, nil
}

func TestCompanionSourceValidationRejectsOldUndatedFutureAndSpoofedHosts(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, beijing)
	raw := `Title: 有趣的新发现
URL: https://www.nasa.gov/news/discovery
Published: 2026-09-08T00:00:00Z
Highlights: 官方公布了一项新发现。
Title: 重复
URL: https://www.nasa.gov/news/discovery
Published Date: 2026-09-08
Title: 旧闻
URL: https://www.bbc.com/news/old
Published: 2026-08-01
Title: 没有日期
URL: https://www.bbc.com/news/undated
Title: 未来消息
URL: https://www.bbc.com/news/future
Published: 2026-09-09
Title: 冒充来源
URL: https://bbc.com.evil.example/news/fake
Published: 2026-09-08
Title: 网传私人绯闻
URL: https://www.bbc.com/news/rumor
Published: 2026-09-08`
	sources := parseTopicSources(raw, now)
	if len(sources) != 1 || sources[0].Title != "有趣的新发现" {
		t.Fatalf("bad sources: %+v", sources)
	}
	for _, bad := range []string{"http://www.bbc.com/a", "https://user:pass@www.bbc.com/a", "https://127.0.0.1/a", "https://bbc.com:9000/a", "javascript:alert(1)"} {
		if validTopicURL(bad) {
			t.Fatalf("accepted unsafe source %s", bad)
		}
	}
	jsonSources, _ := json.Marshal(map[string]interface{}{"results": sources})
	if len(parseTopicSources(string(jsonSources), now)) != 1 {
		t.Fatal("JSON search format unsupported")
	}
}

func TestCompanionInterestsRespectNegationRecencyAndExplicitControls(t *testing.T) {
	now := time.Now()
	memories := []Memory{
		{Key: "偏好", Value: "不喜欢电影但喜欢音乐", ExpiresAt: now.Add(time.Hour), UpdatedAt: now.Add(-time.Hour)},
		{Key: "过期", Value: "不喜欢音乐", ExpiresAt: now.Add(-time.Hour), UpdatedAt: now},
	}
	scores := interestScores(memories, now)
	if scores["film"] >= 0 || scores["music"] != 20 {
		t.Fatalf("negation confused categories: %v", scores)
	}
	memories = append(memories, Memory{Key: "topic:music", Value: "少聊音乐", ExpiresAt: now.Add(time.Hour), UpdatedAt: now})
	for day := 0; day < 10; day++ {
		for _, category := range rankedCategories(memories, 7, now.Add(time.Duration(day)*time.Minute), "音乐新闻") {
			if category.ID == "film" || category.ID == "music" {
				t.Fatal("exploration or weak query overrode rejection")
			}
		}
	}
	memories = append(memories, Memory{Key: "改变主意", Value: "现在喜欢音乐了", ExpiresAt: now.Add(time.Hour), UpdatedAt: now.Add(time.Second)})
	if interestScores(memories, now)["music"] != 20 {
		t.Fatal("new explicit preference ignored")
	}
	corrected := []Memory{{Key: "topic:music", Value: "喜欢篮球", UpdatedAt: now, ExpiresAt: now.Add(time.Hour)}}
	if scores := interestScores(corrected, now); scores["music"] != 0 || scores["sports"] != 20 {
		t.Fatalf("manual correction stayed tied to old category: %v", scores)
	}
}

func TestCompanionTopicDigestMustCiteItsOwnSource(t *testing.T) {
	now := time.Now()
	zero, one := 0, 1
	sources := []topicSource{
		{Title: "官方消息", URL: "https://www.nasa.gov/news/a", PublishedDate: now.Add(-time.Hour).Format(time.RFC3339), Text: "官方宣布了一次新的科学观测计划"},
		{Title: "另一个消息", URL: "https://www.bbc.com/news/b", PublishedDate: now.Add(-time.Hour).Format(time.RFC3339), Text: "这条来源并没有上述科学计划的内容"},
	}
	topics := validatedTopics([]topicDigest{
		{Opener: "没有来源编号", Evidence: "官方宣布了一次新的科学观测计划"},
		{Source: &one, Opener: "错误地引用另一条来源", Evidence: "官方宣布了一次新的科学观测计划"},
		{Source: &zero, Opener: "新观测计划挺有意思。", Evidence: "官方宣布了一次新的科学观测计划"},
		{Source: &zero, Opener: "重复消息", Evidence: "官方宣布了一次新的科学观测计划"},
	}, sources, "science", now)
	if len(topics) != 1 || topics[0].URL != sources[0].URL || topics[0].Opener != "新观测计划挺有意思。" {
		t.Fatalf("unattributed digest accepted: %+v", topics)
	}
}

func TestCompanionTopicRefreshIsSharedAndBacksOffAfterFailure(t *testing.T) {
	store := testStore(t)
	_, _ = store.Ensure(t.Context(), 7)
	provider := &topicProviderStub{}
	service := &Service{Store: store, TopicProvider: provider}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := service.RefreshTopics(t.Context(), 7); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if provider.calls.Load() != 1 {
		t.Fatalf("duplicate paid refreshes: %d", provider.calls.Load())
	}
	topics, err := service.topics(t.Context(), time.Now())
	if err != nil || len(topics) != 1 {
		t.Fatalf("cache not available: %+v %v", topics, err)
	}
	store.db.Table("companion_topic_caches").Where("1 = 1").Update("refresh_after", 0)
	provider.fail = true
	if err := service.RefreshTopics(t.Context(), 7); err == nil {
		t.Fatal("expected search failure")
	}
	if err := service.RefreshTopics(t.Context(), 7); err != nil {
		t.Fatal("backoff should avoid a second search")
	}
	if provider.calls.Load() != 2 {
		t.Fatal("failed search was retried immediately")
	}
	store.db.Table("companion_profiles").Where("user_id = ?", 7).Update("quiet", true)
	store.db.Table("companion_topic_caches").Where("1 = 1").Update("refresh_after", 0)
	if err := service.RefreshTopics(t.Context(), 7); err != nil || provider.calls.Load() != 2 {
		t.Fatal("quiet mode still searches proactively")
	}
}

func TestCompanionTopicFeedbackIsScopedForgettableAndBounded(t *testing.T) {
	store := testStore(t)
	p, _ := store.Ensure(t.Context(), 7)
	_, _ = store.Ensure(t.Context(), 8)
	now := time.Now()
	topic := Topic{Category: "music", Title: "公开演出", URL: "https://www.bbc.com/news/music", PublishedAt: now, FetchedAt: now}
	encoded, _ := json.Marshal(topic)
	store.db.Table("companion_profiles").Where("user_id = ?", 7).Updates(map[string]interface{}{"last_topic_json": string(encoded), "refresh_token": "stale"})
	for i := 0; i < 60; i++ {
		store.db.Table("companion_memories").Create(&Memory{ID: fmt.Sprint(i), UserID: 7, Key: fmt.Sprint(i), Value: "普通记忆", ExpiresAt: now.Add(time.Hour), UpdatedAt: now.Add(-time.Hour)})
	}
	service := &Service{Store: store}
	if err := service.TopicFeedback(t.Context(), 8, topic.URL, "avoid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign topic accepted: %v", err)
	}
	if err := service.TopicFeedback(t.Context(), 7, "https://www.bbc.com/forged", "like"); !errors.Is(err, ErrNotFound) {
		t.Fatal("unseen topic accepted")
	}
	if err := service.TopicFeedback(t.Context(), 7, topic.URL, "avoid"); err != nil {
		t.Fatal(err)
	}
	memories, _ := store.Memories(t.Context(), 7)
	if len(memories) != 60 || interestScores(memories, now)["music"] >= 0 {
		t.Fatal("feedback or memory cap lost")
	}
	if err := service.saveExtraction(t.Context(), p, "stale", 10, extractedMemory{Summary: "stale"}, nil); err != nil {
		t.Fatal(err)
	}
	profile, _ := store.Profile(t.Context(), 7)
	if profile.Summary == "stale" || profile.Revision != 1 {
		t.Fatal("stale extraction overwrote feedback")
	}
	if err := store.Forget(t.Context(), 7, 20, ""); err != nil {
		t.Fatal(err)
	}
	memories, _ = store.Memories(t.Context(), 7)
	profile, _ = store.Profile(t.Context(), 7)
	if len(memories) != 0 || profile.LastTopicJSON != "" || profile.SeenTopicsJSON != "" {
		t.Fatal("forget retained recommendation state")
	}
}

func TestCompanionTopicGreetingAvoidsRepetitionDistressAndLateNight(t *testing.T) {
	store := testStore(t)
	p, _ := store.Ensure(t.Context(), 7)
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, beijing)
	topic := Topic{Category: "music", Title: "音乐新闻", Opener: "有个新话题。", URL: "https://www.bbc.com/news/music", PublishedAt: now.Add(-time.Hour), FetchedAt: now}
	encoded, _ := json.Marshal([]Topic{topic})
	store.db.Table("companion_topic_caches").Create(map[string]interface{}{"category": "music", "topics_json": string(encoded), "fetched_at": now})
	service := &Service{Store: store, TopicProvider: &topicProviderStub{}}
	if service.pickGreetingTopic(t.Context(), p, nil, now) != nil {
		t.Fatal("news replaced first introduction")
	}
	p.GreetingAt = now.Add(-48 * time.Hour)
	if service.pickGreetingTopic(t.Context(), p, nil, now) == nil {
		t.Fatal("eligible source not offered")
	}
	p.SeenTopicsJSON = rememberTopic("", topic.URL)
	if service.pickGreetingTopic(t.Context(), p, nil, now) != nil {
		t.Fatal("same news repeated")
	}
	p.SeenTopicsJSON = ""
	messages := []domainchat.Message{{ID: 1, Role: "user", Status: "success", Content: "我今天很难过", CreatedAt: now.Add(-12 * time.Hour)}}
	if service.pickGreetingTopic(t.Context(), p, messages, now) != nil {
		t.Fatal("news interrupted emotional context")
	}
	if service.pickGreetingTopic(t.Context(), p, nil, time.Date(2026, 9, 8, 23, 0, 0, 0, beijing)) != nil {
		t.Fatal("late night news offered")
	}
	for i := 0; i < 100; i++ {
		p.SeenTopicsJSON = rememberTopic(p.SeenTopicsJSON, fmt.Sprint(i))
	}
	if len(seenTopics(p.SeenTopicsJSON)) != 20 || !strings.Contains(p.SeenTopicsJSON, "99") {
		t.Fatal("seen history unbounded")
	}
}
