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

package user

import (
	"context"
	"fmt"

	userEntity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

type CloneSourceUser struct {
	Name        string
	Description string
	Locale      string
}

func (u *UserApplicationService) GetCloneSource(ctx context.Context, userID int64) (*CloneSourceUser, []*userEntity.Space, error) {
	sourceUser, err := u.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	spaceUsers, err := u.spaceRepo.GetSpaceList(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if len(spaceUsers) == 0 {
		return &CloneSourceUser{
			Name:        sourceUser.Name,
			Description: sourceUser.Description,
			Locale:      sourceUser.Locale,
		}, nil, nil
	}

	spaceIDs := make([]int64, 0, len(spaceUsers))
	for _, spaceUser := range spaceUsers {
		spaceIDs = append(spaceIDs, spaceUser.SpaceID)
	}

	spaces, err := u.spaceRepo.GetSpaceByIDs(ctx, spaceIDs)
	if err != nil {
		return nil, nil, err
	}

	resultSpaces := make([]*userEntity.Space, 0, len(spaceUsers))
	for _, spaceUser := range spaceUsers {
		matched := false
		for _, space := range spaces {
			if space.ID != spaceUser.SpaceID {
				continue
			}

			resultSpaces = append(resultSpaces, &userEntity.Space{
				ID:          space.ID,
				Name:        space.Name,
				Description: space.Description,
				IconURL:     space.IconURI,
				OwnerID:     space.OwnerID,
				CreatorID:   space.CreatorID,
				CreatedAt:   space.CreatedAt,
				UpdatedAt:   space.UpdatedAt,
			})
			matched = true
			break
		}
		if !matched {
			continue
		}
	}

	return &CloneSourceUser{
		Name:        sourceUser.Name,
		Description: sourceUser.Description,
		Locale:      sourceUser.Locale,
	}, resultSpaces, nil
}

func (u *UserApplicationService) CloneSpacesForUser(ctx context.Context, targetUserID int64, spaces []*userEntity.Space) (map[int64]int64, error) {
	spaceIDMap := make(map[int64]int64, len(spaces))
	for _, sourceSpace := range spaces {
		targetSpaceID, err := u.idgen.GenID(ctx)
		if err != nil {
			return nil, fmt.Errorf("generate target space id failed: %w", err)
		}

		now := sourceSpace.CreatedAt
		if now <= 0 {
			now = sourceSpace.UpdatedAt
		}
		if now <= 0 {
			now = 1
		}

		err = u.DomainSVC.CreateSpace(ctx, &userEntity.Space{
			ID:          targetSpaceID,
			Name:        sourceSpace.Name,
			Description: sourceSpace.Description,
			IconURL:     sourceSpace.IconURL,
			OwnerID:     targetUserID,
			CreatorID:   targetUserID,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			return nil, fmt.Errorf("create target space failed, sourceSpaceID=%d: %w", sourceSpace.ID, err)
		}

		spaceIDMap[sourceSpace.ID] = targetSpaceID
	}

	return spaceIDMap, nil
}

func (u *UserApplicationService) BindUserToClonedSpaces(ctx context.Context, targetUserID int64, sourceToTargetSpaceIDs map[int64]int64, primarySourceSpaceID int64) error {
	for sourceSpaceID, targetSpaceID := range sourceToTargetSpaceIDs {
		if sourceSpaceID == primarySourceSpaceID {
			continue
		}
		if err := u.DomainSVC.AddSpaceUser(ctx, targetSpaceID, targetUserID, 1); err != nil {
			return fmt.Errorf("add cloned space user failed, sourceSpaceID=%d targetSpaceID=%d: %w", sourceSpaceID, targetSpaceID, err)
		}
	}

	return nil
}

func (u *UserApplicationService) RollbackCloneTarget(ctx context.Context, targetUserID int64) error {
	return u.userRepo.DeleteCloneTarget(ctx, targetUserID)
}
