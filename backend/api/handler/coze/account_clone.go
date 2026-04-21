/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package coze

import (
	"context"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"

	accountclone "github.com/coze-dev/coze-studio/backend/application/accountclone"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/hertzutil/domain"
)

type cloneAccountRequest struct {
	Email    string `json:"email" form:"email"`
	Password string `json:"password" form:"password"`
}

type cloneAccountData struct {
	UserID  int64                             `json:"user_id,string"`
	Summary *accountclone.CloneAccountSummary `json:"summary,omitempty"`
}

type cloneAccountResponse struct {
	Code int32             `json:"code"`
	Data *cloneAccountData `json:"data,omitempty"`
}

type accountTargetRequest struct {
	UserID int64  `json:"user_id,string" form:"user_id"`
	Email  string `json:"email" form:"email"`
}

type accountExistsData struct {
	Exists bool  `json:"exists"`
	UserID int64 `json:"user_id,string,omitempty"`
}

type accountExistsResponse struct {
	Code int32              `json:"code"`
	Data *accountExistsData `json:"data,omitempty"`
}

type deleteAccountData struct {
	UserID int64 `json:"user_id,string"`
}

type deleteAccountResponse struct {
	Code int32              `json:"code"`
	Data *deleteAccountData `json:"data,omitempty"`
}

func PassportAccountClonePost(ctx context.Context, c *app.RequestContext) {
	var req cloneAccountRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := accountclone.CloneCurrentAccount(ctx, &accountclone.CloneAccountRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(http.StatusOK, &cloneAccountResponse{
		Code: 0,
		Data: &cloneAccountData{
			UserID:  resp.UserID,
			Summary: resp.Summary,
		},
	})
}

func PassportAccountExistPost(ctx context.Context, c *app.RequestContext) {
	var req accountTargetRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	if req.UserID <= 0 && req.Email == "" {
		invalidParamRequestResponse(c, "user_id or email is required")
		return
	}

	resp, err := accountclone.CheckAccountExists(ctx, &accountclone.AccountExistsRequest{
		UserID: req.UserID,
		Email:  req.Email,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(http.StatusOK, &accountExistsResponse{
		Code: 0,
		Data: &accountExistsData{
			Exists: resp.Exists,
			UserID: resp.UserID,
		},
	})
}

func PassportAccountDeletePost(ctx context.Context, c *app.RequestContext) {
	var req accountTargetRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	if req.UserID <= 0 && req.Email == "" {
		invalidParamRequestResponse(c, "user_id or email is required")
		return
	}

	resp, err := accountclone.DeleteAccount(ctx, &accountclone.DeleteAccountRequest{
		UserID: req.UserID,
		Email:  req.Email,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	if currentUserID := ctxutil.GetUIDFromCtx(ctx); currentUserID != nil && *currentUserID == resp.UserID {
		c.SetCookie(entity.SessionKey,
			"",
			-1,
			"/", domain.GetOriginHost(c),
			protocol.CookieSameSiteDefaultMode,
			false, true)
	}

	c.JSON(http.StatusOK, &deleteAccountResponse{
		Code: 0,
		Data: &deleteAccountData{
			UserID: resp.UserID,
		},
	})
}
