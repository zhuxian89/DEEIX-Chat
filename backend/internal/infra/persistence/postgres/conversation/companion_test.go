package conversation

import (
	"fmt"
	"testing"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
)

func TestCompanionMemoryCursorDoesNotSkipBacklogOrOtherUsers(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	var expected []uint
	for i := 1; i <= 80; i++ {
		item := model.Message{ConversationID: 7, UserID: 1, PublicID: fmt.Sprintf("companion_%d", i), Role: "user", ContentType: "text", Content: "hello", Status: "success"}
		if i%2 == 0 {
			item.Role = "assistant"
		}
		if i == 10 {
			item.Status = "blocked"
		}
		if i == 11 {
			item.UserID = 2
		}
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
		if i != 10 && i != 11 {
			expected = append(expected, item.ID)
		}
	}
	repo := NewRepo(db)
	var after uint
	var got []uint
	for page := 0; page < 3; page++ {
		batch, err := repo.ListCompanionMemoryMessages(t.Context(), 1, 7, after, 1000)
		if err != nil || len(batch) > 32 {
			t.Fatalf("invalid cursor batch: %d %v", len(batch), err)
		}
		for _, item := range batch {
			if item.UserID != 1 || item.Status != "success" {
				t.Fatal("ineligible memory source")
			}
			got = append(got, item.ID)
			after = item.ID
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(expected) {
		t.Fatalf("cursor skipped messages: %v, expected %v", got, expected)
	}
}
