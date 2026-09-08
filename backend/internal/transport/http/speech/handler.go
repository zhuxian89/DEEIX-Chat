package speech

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/speech"
	speechport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/speech"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service *app.Service }

func NewHandler(service *app.Service) *Handler { return &Handler{service: service} }
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/speech/capabilities", h.Capabilities)
	group.POST("/speech/transcriptions", h.Transcribe)
}

type SpeechCapabilities struct {
	Enabled       bool `json:"enabled"`
	MaxDurationMS int  `json:"maxDurationMs"`
}
type SpeechCapabilitiesResponse struct {
	ErrorMsg string             `json:"errorMsg"`
	Data     SpeechCapabilities `json:"data"`
}
type SpeechTranscriptionRequest struct {
	// Base64-encoded MP3, 16 kHz, mono, up to 60 seconds and 512 KiB decoded.
	Audio []byte `json:"audio" swaggertype:"string"`
}
type SpeechTranscript struct {
	Text       string `json:"text"`
	DurationMS int    `json:"durationMs"`
}
type SpeechTranscriptResponse struct {
	ErrorMsg string           `json:"errorMsg"`
	Data     SpeechTranscript `json:"data"`
}

// Capabilities godoc
// @Summary 查询语音输入是否可用
// @Tags speech
// @Produce json
// @Security BearerAuth
// @Success 200 {object} SpeechCapabilitiesResponse
// @Router /speech/capabilities [get]
func (h *Handler) Capabilities(c *gin.Context) {
	response.Success(c, SpeechCapabilities{Enabled: h.service.Enabled(c.Request.Context()), MaxDurationMS: 60000})
}

// Transcribe godoc
// @Summary 将短录音转为输入框草稿，不发送聊天消息
// @Tags speech
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body SpeechTranscriptionRequest true "Base64 MP3 audio"
// @Success 200 {object} SpeechTranscriptResponse
// @Failure 400,401,413,429,503 {object} response.Envelope
// @Router /speech/transcriptions [post]
func (h *Handler) Transcribe(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 750*1024)
	defer c.Request.Body.Close()
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var request SpeechTranscriptionRequest
	if err := decoder.Decode(&request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(c, http.StatusRequestEntityTooLarge, "speech_audio_too_large")
			return
		}
		fail(c, http.StatusBadRequest, "speech_invalid_audio")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		fail(c, http.StatusBadRequest, "speech_invalid_audio")
		return
	}
	result, err := h.service.Transcribe(c.Request.Context(), middleware.MustUserID(c), request.Audio)
	if err != nil {
		switch {
		case errors.Is(err, speechport.ErrInvalidAudio):
			fail(c, http.StatusBadRequest, "speech_invalid_audio")
		case errors.Is(err, speechport.ErrNoSpeech):
			fail(c, http.StatusBadRequest, "speech_no_speech")
		case errors.Is(err, speechport.ErrQuota):
			fail(c, http.StatusServiceUnavailable, "speech_quota_exhausted")
		case errors.Is(err, speechport.ErrBusy):
			fail(c, http.StatusTooManyRequests, "speech_busy")
		default:
			fail(c, http.StatusServiceUnavailable, "speech_unavailable")
		}
		return
	}
	response.Success(c, SpeechTranscript{Text: result.Text, DurationMS: result.DurationMS})
}

func fail(c *gin.Context, status int, code string) {
	response.ErrorWithCode(c, status, code, "speech input request failed")
}
