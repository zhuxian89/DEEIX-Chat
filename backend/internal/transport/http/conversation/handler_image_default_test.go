package conversation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/gin-gonic/gin"
)

func TestDefaultImageModelRouteUsesAuthenticatedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer test" {
			response.Error(c, http.StatusUnauthorized, "unauthorized")
			c.Abort()
			return
		}
	})
	module := &Module{Handler: &Handler{}}
	module.RegisterRoutes(group)
	for _, path := range []string{"default-image-model-candidate", "default-model-candidate"} {
		for _, authorized := range []bool{false, true} {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/"+path, nil)
			if authorized {
				request.Header.Set("Authorization", "Bearer test")
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if !authorized {
				if recorder.Code != http.StatusUnauthorized {
					t.Fatalf("unauthenticated %s returned %d", path, recorder.Code)
				}
				var result response.Envelope
				if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
					t.Fatalf("unauthenticated %s returned invalid error envelope: %v", path, err)
				}
				if result.ErrorCode != response.CodeAuthUnauthorized || result.ErrorMsg != "unauthorized" || result.Data != nil {
					t.Fatalf("unauthenticated %s returned unexpected error envelope: %+v", path, result)
				}
				continue
			}
			if recorder.Code != http.StatusOK {
				t.Fatalf("authenticated %s returned %d", path, recorder.Code)
			}
			var result struct {
				Data ConversationDefaultModelCandidateResponse `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Data.PlatformModelName != "" || result.Data.Source != "" {
				t.Fatal("empty configuration should return an empty candidate")
			}
		}
	}
}
