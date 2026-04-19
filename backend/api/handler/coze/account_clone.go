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

	accountclone "github.com/coze-dev/coze-studio/backend/application/accountclone"
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
