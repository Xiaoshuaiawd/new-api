package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

type channelModelManageRequest struct {
	Model  string `json:"model"`
	Reason string `json:"reason,omitempty"`
}

// DisableChannelModel disables a single model for a channel (does not disable the whole channel).
// POST /api/channel/:id/model/disable
func DisableChannelModel(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid channel id"})
		return
	}

	req := channelModelManageRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || req.Model == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "model is required"})
		return
	}

	matchName := ratio_setting.FormatMatchingModelName(req.Model)
	if matchName == "" {
		matchName = req.Model
	}

	changed, err := model.DisableChannelModel(channelId, matchName, req.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": changed})
}

// EnableChannelModel re-enables a single model for a channel.
// POST /api/channel/:id/model/enable
func EnableChannelModel(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid channel id"})
		return
	}

	req := channelModelManageRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || req.Model == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "model is required"})
		return
	}

	matchName := ratio_setting.FormatMatchingModelName(req.Model)
	if matchName == "" {
		matchName = req.Model
	}

	changed, err := model.EnableChannelModel(channelId, matchName)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": changed})
}
