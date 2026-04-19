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

package app

import (
	"context"

	intelligenceCommon "github.com/coze-dev/coze-studio/backend/api/model/app/intelligence/common"
	projectAPI "github.com/coze-dev/coze-studio/backend/api/model/app/intelligence/project"
	searchEntity "github.com/coze-dev/coze-studio/backend/domain/search/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/errorx"
)

func (a *APPApplicationService) CloneDraftProjectToUser(ctx context.Context, projectID, targetUserID, targetSpaceID int64) (int64, error) {
	draftAPP, err := a.ValidateDraftAPPAccess(ctx, projectID)
	if err != nil {
		return 0, errorx.Wrapf(err, "validate clone draft project failed")
	}

	req := &projectAPI.DraftProjectCopyRequest{
		ProjectID:   projectID,
		ToSpaceID:   targetSpaceID,
		Name:        draftAPP.GetName(),
		Description: draftAPP.GetDesc(),
		IconURI:     draftAPP.GetIconURI(),
	}

	newAPPID, err := a.duplicateDraftAPP(ctx, targetUserID, req)
	if err != nil {
		return 0, err
	}

	if err = a.projectEventBus.PublishProject(ctx, &searchEntity.ProjectDomainEvent{
		OpType: searchEntity.Created,
		Project: &searchEntity.ProjectDocument{
			Status:  intelligenceCommon.IntelligenceStatus_Using,
			Type:    intelligenceCommon.IntelligenceType_Project,
			ID:      newAPPID,
			SpaceID: &targetSpaceID,
			OwnerID: &targetUserID,
			Name:    &req.Name,
		},
	}); err != nil {
		return 0, err
	}

	return newAPPID, nil
}
