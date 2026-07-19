package control

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"sub2api-desktop/core/internal/cloudsync"
)

func (c *Control) withCloudV2(w http.ResponseWriter, r *http.Request, timeout time.Duration, run func(context.Context) (any, error)) {
	if c.cloud == nil {
		writeControlError(w, http.StatusServiceUnavailable, "cloud_unavailable", "Amber Cloud is unavailable", true, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	result, err := run(ctx)
	if err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (c *Control) cloudV2Profile(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 60*time.Second, func(ctx context.Context) (any, error) { return c.cloud.EnsureProfile(ctx, "") })
}

func (c *Control) cloudV2Workspace(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 60*time.Second, func(ctx context.Context) (any, error) { return c.cloud.LoadWorkspace(ctx) })
}

func (c *Control) cloudV2UpdateProfile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		DisplayName string `json:"display_name"`
	}
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) { return c.cloud.UpdateProfile(ctx, request.DisplayName) })
}

func (c *Control) cloudV2Friends(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) { return c.cloud.ListFriends(ctx) })
}

func (c *Control) cloudV2FriendRequests(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) { return c.cloud.ListFriendRequests(ctx) })
}

func (c *Control) cloudV2CreateFriendRequest(w http.ResponseWriter, r *http.Request) {
	var request struct {
		FriendCode string `json:"friend_code"`
	}
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.FriendAction(ctx, http.MethodPost, "/v1/friend-requests", map[string]string{"friend_code": request.FriendCode})
	})
}

func (c *Control) cloudV2FriendRequestAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if action != "accept" && action != "decline" && action != "cancel" {
		writeControlError(w, http.StatusBadRequest, "invalid_friend_action", "The friend request action is invalid", false, nil)
		return
	}
	path := fmt.Sprintf("/v1/friend-requests/%s/%s", url.PathEscape(r.PathValue("id")), action)
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) { return c.cloud.FriendAction(ctx, http.MethodPost, path, nil) })
}

func (c *Control) cloudV2FriendUpdate(w http.ResponseWriter, r *http.Request) {
	var request map[string]any
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	path := fmt.Sprintf("/v1/friends/%s", url.PathEscape(r.PathValue("id")))
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.FriendAction(ctx, http.MethodPatch, path, request)
	})
}

func (c *Control) cloudV2FriendDelete(w http.ResponseWriter, r *http.Request) {
	mode := "pause"
	if r.URL.Query().Get("mode") == "revoke" {
		mode = "revoke"
	}
	path := fmt.Sprintf("/v1/friends/%s?mode=%s", url.PathEscape(r.PathValue("id")), mode)
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) { return c.cloud.FriendAction(ctx, http.MethodDelete, path, nil) })
}

func (c *Control) cloudV2FriendBlock(w http.ResponseWriter, r *http.Request) {
	path := fmt.Sprintf("/v1/friends/%s/block", url.PathEscape(r.PathValue("id")))
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) { return c.cloud.FriendAction(ctx, http.MethodPost, path, nil) })
}

func (c *Control) cloudV2FriendUnblock(w http.ResponseWriter, r *http.Request) {
	path := fmt.Sprintf("/v1/friends/%s/block", url.PathEscape(r.PathValue("id")))
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) { return c.cloud.FriendAction(ctx, http.MethodDelete, path, nil) })
}

func (c *Control) cloudV2ShareGroups(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) { return c.cloud.ListShareGroups(ctx) })
}

func (c *Control) cloudV2CreateShareGroup(w http.ResponseWriter, r *http.Request) {
	var request cloudsync.CreateShareGroupInput
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 90*time.Second, func(ctx context.Context) (any, error) { return c.cloud.CreateShareGroup(ctx, request) })
}

func cloudGroupPath(r *http.Request, suffix string) string {
	return fmt.Sprintf("/v1/share-groups/%s%s", url.PathEscape(r.PathValue("id")), suffix)
}

func (c *Control) cloudV2ShareGroupDetail(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.CloudV2Request(ctx, http.MethodGet, cloudGroupPath(r, ""), nil, "")
	})
}

func (c *Control) cloudV2ShareGroupUpdate(w http.ResponseWriter, r *http.Request) {
	var request map[string]any
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.CloudV2Request(ctx, http.MethodPatch, cloudGroupPath(r, ""), request, "")
	})
}

func (c *Control) cloudV2ShareGroupDelete(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.CloudV2Request(ctx, http.MethodDelete, cloudGroupPath(r, ""), nil, "")
	})
}

func (c *Control) cloudV2ShareGroupAddAccount(w http.ResponseWriter, r *http.Request) {
	var request cloudsync.ShareGroupAccountSelection
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 60*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.AddShareGroupAccount(ctx, r.PathValue("id"), request)
	})
}

func (c *Control) cloudV2ShareGroupAccountUpdate(w http.ResponseWriter, r *http.Request) {
	var request map[string]any
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	suffix := fmt.Sprintf("/accounts/%s", url.PathEscape(r.PathValue("accountId")))
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.CloudV2Request(ctx, http.MethodPatch, cloudGroupPath(r, suffix), request, "")
	})
}

func (c *Control) cloudV2ShareGroupAccountDelete(w http.ResponseWriter, r *http.Request) {
	suffix := fmt.Sprintf("/accounts/%s", url.PathEscape(r.PathValue("accountId")))
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.CloudV2Request(ctx, http.MethodDelete, cloudGroupPath(r, suffix), nil, "")
	})
}

func (c *Control) cloudV2ShareGroupInvite(w http.ResponseWriter, r *http.Request) {
	var request struct {
		IdempotencyKey string                                   `json:"idempotency_key"`
		Recipients     []cloudsync.ShareGroupRecipientSelection `json:"recipients"`
	}
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 60*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.InviteShareGroupRecipients(ctx, r.PathValue("id"), request.Recipients, request.IdempotencyKey)
	})
}

func (c *Control) cloudV2ShareGroupRecipientUpdate(w http.ResponseWriter, r *http.Request) {
	var request map[string]any
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	suffix := fmt.Sprintf("/recipients/%s", url.PathEscape(r.PathValue("recipientId")))
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.CloudV2Request(ctx, http.MethodPatch, cloudGroupPath(r, suffix), request, "")
	})
}

func (c *Control) cloudV2ShareGroupRecipientDelete(w http.ResponseWriter, r *http.Request) {
	suffix := fmt.Sprintf("/recipients/%s", url.PathEscape(r.PathValue("recipientId")))
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.CloudV2Request(ctx, http.MethodDelete, cloudGroupPath(r, suffix), nil, "")
	})
}

func (c *Control) cloudV2ShareGroupRotateKey(w http.ResponseWriter, r *http.Request) {
	var request struct {
		FriendshipID   string `json:"friendship_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 60*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.RotateRecipientKey(ctx, r.PathValue("id"), r.PathValue("recipientId"), request.FriendshipID, request.IdempotencyKey)
	})
}

func (c *Control) cloudV2ShareGroupUsage(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.CloudV2Request(ctx, http.MethodGet, cloudGroupPath(r, "/usage"), nil, "")
	})
}

func (c *Control) cloudV2ShareGroupAudit(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.CloudV2Request(ctx, http.MethodGet, cloudGroupPath(r, "/audit"), nil, "")
	})
}

func (c *Control) cloudV2ReceivedShares(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 60*time.Second, func(ctx context.Context) (any, error) { return c.cloud.ListReceivedShares(ctx) })
}

func (c *Control) cloudV2ReceivedShareAction(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimSpace(r.PathValue("action"))
	if action != "accept" && action != "decline" && action != "leave" {
		writeControlError(w, http.StatusBadRequest, "invalid_received_share_action", "The received share action is invalid", false, nil)
		return
	}
	if action == "accept" {
		c.withCloudV2(w, r, 60*time.Second, func(ctx context.Context) (any, error) { return c.cloud.AcceptReceivedShare(ctx, r.PathValue("id")) })
		return
	}
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.ReceivedShareAction(ctx, r.PathValue("id"), action)
	})
}

func (c *Control) cloudV2ReceivedShareTest(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 45*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.TestReceivedShare(ctx, r.PathValue("id"), "")
	})
}

func (c *Control) cloudV2Devices(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) { return c.cloud.ListDevices(ctx) })
}

func (c *Control) cloudV2EnsureDevice(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 60*time.Second, func(ctx context.Context) (any, error) { return c.cloud.EnsureDevice(ctx) })
}

func (c *Control) cloudV2DeleteDevice(w http.ResponseWriter, r *http.Request) {
	path := fmt.Sprintf("/v1/devices/%s", url.PathEscape(r.PathValue("id")))
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.CloudV2Request(ctx, http.MethodDelete, path, nil, "")
	})
}

func (c *Control) cloudV2SetRelay(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	if request.Enabled == nil {
		writeControlError(w, http.StatusBadRequest, "invalid_relay_setting", "The enabled field is required", false, nil)
		return
	}
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		if err := c.cloud.SetRelayEnabled(ctx, *request.Enabled); err != nil {
			return nil, err
		}
		return map[string]bool{"enabled": *request.Enabled}, nil
	})
}

func (c *Control) cloudConnectHost(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.GetConnectHost(ctx)
	})
}

func (c *Control) cloudConnectEvents(w http.ResponseWriter, r *http.Request) {
	cursor := int64(0)
	if value := strings.TrimSpace(r.URL.Query().Get("cursor")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			writeControlError(w, http.StatusBadRequest, "invalid_event_cursor", "The event cursor is invalid", false, nil)
			return
		}
		cursor = parsed
	}
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.ListConnectEvents(ctx, cursor)
	})
}

func (c *Control) cloudConnectHostAccounts(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Accounts []cloudsync.ShareGroupAccountSelection `json:"accounts"`
	}
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 60*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.ConfigureConnectHostAccounts(ctx, request.Accounts)
	})
}

func (c *Control) cloudConnectHostStart(w http.ResponseWriter, r *http.Request) {
	c.cloudConnectStart(w, r, false)
}

func (c *Control) cloudConnectHostRotatePassword(w http.ResponseWriter, r *http.Request) {
	c.cloudConnectStart(w, r, true)
}

func (c *Control) cloudConnectStart(w http.ResponseWriter, r *http.Request, rotate bool) {
	var request cloudsync.ConnectHostStartInput
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 45*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.StartConnectHost(ctx, request, rotate)
	})
}

func (c *Control) cloudConnectHostAction(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimSpace(r.PathValue("action"))
	if action != "pause" && action != "resume" && action != "reset-code" {
		writeControlError(w, http.StatusBadRequest, "invalid_connect_action", "The sharing action is invalid", false, nil)
		return
	}
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.ConnectHostAction(ctx, action)
	})
}

func (c *Control) cloudConnectRecipientUpdate(w http.ResponseWriter, r *http.Request) {
	var request map[string]any
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.ConnectRecipientRequest(ctx, http.MethodPatch, r.PathValue("id"), request)
	})
}

func (c *Control) cloudConnectRecipientDelete(w http.ResponseWriter, r *http.Request) {
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.ConnectRecipientRequest(ctx, http.MethodDelete, r.PathValue("id"), nil)
	})
}

func (c *Control) cloudConnectClaimAndUse(w http.ResponseWriter, r *http.Request) {
	var request cloudsync.ConnectClaimInput
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 60*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.ClaimConnectAndUse(ctx, request)
	})
}

func (c *Control) cloudConnectReceivedUpdate(w http.ResponseWriter, r *http.Request) {
	var request cloudsync.ConnectReceivedUpdate
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	c.withCloudV2(w, r, 30*time.Second, func(ctx context.Context) (any, error) {
		return c.cloud.UpdateConnectReceived(ctx, r.PathValue("id"), request)
	})
}
