package dailycheckin

import (
	"errors"
	"net/http"
	"time"

	appdailycheckin "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/dailycheckin"
	domaindailycheckin "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/dailycheckin"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service *appdailycheckin.Service }

func NewHandler(service *appdailycheckin.Service) *Handler { return &Handler{service: service} }

// GetStatus godoc
// @Summary 获取每日签到状态
// @Tags daily-checkin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} DailyCheckinStatusResponse
// @Failure 503 {object} response.Envelope
// @Router /daily-checkin/status [get]
func (h *Handler) GetStatus(c *gin.Context) {
	status, err := h.service.Status(c.Request.Context(), middleware.MustUserID(c))
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "每日签到暂不可用")
		return
	}
	view := DailyCheckinStatusResponse{
		Enabled:         status.Enabled,
		BusinessDate:    status.BusinessDate.Format("2006-01-02"),
		NextAvailableAt: status.NextAvailableAt.Format(time.RFC3339),
		Claimed:         status.Claimed,
		UnitPriceUsd:    domaindailycheckin.USD(status.UnitPriceNanousd),
		StreakDays:      status.StreakDays,
		Prizes:          make([]DailyCheckinPrizeResponse, 0, len(status.Prizes)),
	}
	for _, prize := range status.Prizes {
		view.Prizes = append(view.Prizes, DailyCheckinPrizeResponse{PrizeKey: prize.Key, Calls: prize.Calls, WeightBps: prize.WeightBps})
	}
	if status.Claim != nil {
		view.AwardedCalls = status.Claim.AwardedCalls
		view.UnitPriceUsd = domaindailycheckin.USD(status.Claim.UnitPriceNanousd)
		view.RewardUsd = domaindailycheckin.USD(status.Claim.RewardNanousd)
		view.PrizeKey = status.Claim.PrizeKey
	}
	response.Success(c, view)
}

// Claim godoc
// @Summary 领取每日签到转盘奖励
// @Tags daily-checkin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} DailyCheckinClaimResponse
// @Failure 503 {object} response.Envelope
// @Router /daily-checkin/claim [post]
func (h *Handler) Claim(c *gin.Context) {
	result, err := h.service.Claim(c.Request.Context(), middleware.MustUserID(c))
	if err != nil {
		message := "每日签到领取失败，请稍后重试"
		if errors.Is(err, domaindailycheckin.ErrDisabled) {
			message = "每日签到暂未开放"
		}
		response.Error(c, http.StatusServiceUnavailable, message)
		return
	}
	response.Success(c, DailyCheckinClaimResponse{
		ClaimedNow:     result.ClaimedNow,
		AlreadyClaimed: !result.ClaimedNow,
		BusinessDate:   result.Claim.BusinessDate.Format("2006-01-02"),
		AwardedCalls:   result.Claim.AwardedCalls,
		UnitPriceUsd:   domaindailycheckin.USD(result.Claim.UnitPriceNanousd),
		RewardUsd:      domaindailycheckin.USD(result.Claim.RewardNanousd),
		PrizeKey:       result.Claim.PrizeKey,
		StreakDays:     result.Claim.StreakDays,
	})
}
