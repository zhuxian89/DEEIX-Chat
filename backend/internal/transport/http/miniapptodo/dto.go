package miniapptodo

import (
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	"time"
)

type EntryStatusResponse struct {
	OwnerKey   string     `json:"ownerKey"`
	Unlocked   bool       `json:"unlocked"`
	UnlockedAt *time.Time `json:"unlockedAt,omitempty"`
}
type UnlockRequest struct {
	Code string `json:"code" binding:"max=64"`
}
type FeedbackRequest struct {
	Content string `json:"content" binding:"required,max=2000"`
}
type SavedResponse struct {
	Saved bool `json:"saved"`
}
type ListResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Position  int        `json:"position"`
	Version   int64      `json:"version"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}
type TaskResponse struct {
	ID          string     `json:"id"`
	ListID      string     `json:"listID"`
	ParentID    string     `json:"parentID"`
	Title       string     `json:"title"`
	Notes       string     `json:"notes"`
	DueDate     string     `json:"dueDate"`
	DueTime     string     `json:"dueTime"`
	Timezone    string     `json:"timezone"`
	Important   bool       `json:"important"`
	Position    int        `json:"position"`
	RepeatKind  string     `json:"repeatKind"`
	RepeatDay   int        `json:"repeatDay"`
	SeriesID    string     `json:"seriesID"`
	Occurrence  int        `json:"occurrence"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
}
type TaskDraftRequest struct {
	ID         string `json:"id"`
	ListID     string `json:"listID"`
	ParentID   string `json:"parentID"`
	Title      string `json:"title"`
	Notes      string `json:"notes"`
	DueDate    string `json:"dueDate"`
	DueTime    string `json:"dueTime"`
	Timezone   string `json:"timezone"`
	Important  bool   `json:"important"`
	Position   int    `json:"position"`
	RepeatKind string `json:"repeatKind"`
	RepeatDay  int    `json:"repeatDay"`
}
type ListDraftRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}
type OperationRequest struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	EntityID    string            `json:"entityID"`
	BaseVersion int64             `json:"baseVersion"`
	Task        *TaskDraftRequest `json:"task,omitempty"`
	List        *ListDraftRequest `json:"list,omitempty"`
	Completed   bool              `json:"completed,omitempty"`
}
type SyncRequest struct {
	Operations []OperationRequest `json:"operations" binding:"required,min=1,max=50"`
}
type OperationResultResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}
type SnapshotResponse struct {
	Lists []ListResponse `json:"lists"`
	Tasks []TaskResponse `json:"tasks"`
}
type SyncResponse struct {
	Results  []OperationResultResponse `json:"results"`
	Snapshot SnapshotResponse          `json:"snapshot"`
}
type TaskPageResponse struct {
	Results []TaskResponse `json:"results"`
	Total   int64          `json:"total"`
}
type ExportResponse struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
	MimeType string `json:"mimeType"`
}

func snapshotResponse(value domain.Snapshot) SnapshotResponse {
	result := SnapshotResponse{Lists: []ListResponse{}, Tasks: tasksResponse(value.Tasks)}
	for _, list := range value.Lists {
		result.Lists = append(result.Lists, ListResponse(list))
	}
	return result
}
func tasksResponse(tasks []domain.Task) []TaskResponse {
	result := make([]TaskResponse, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, TaskResponse(task))
	}
	return result
}
func (op OperationRequest) domain() domain.Operation {
	result := domain.Operation{ID: op.ID, Kind: op.Kind, EntityID: op.EntityID, BaseVersion: op.BaseVersion, Completed: op.Completed}
	if op.Task != nil {
		draft := domain.TaskDraft(*op.Task)
		result.Task = &draft
	}
	if op.List != nil {
		draft := domain.ListDraft(*op.List)
		result.List = &draft
	}
	return result
}
