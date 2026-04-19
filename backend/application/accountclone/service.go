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

package accountclone

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/coze-dev/coze-studio/backend/api/model/app/bot_common"
	developerAPI "github.com/coze-dev/coze-studio/backend/api/model/app/developer_api"
	intelligenceCommon "github.com/coze-dev/coze-studio/backend/api/model/app/intelligence/common"
	projectAPI "github.com/coze-dev/coze-studio/backend/api/model/app/intelligence/project"
	"github.com/coze-dev/coze-studio/backend/api/model/data/database/table"
	knowledgeDataset "github.com/coze-dev/coze-studio/backend/api/model/data/knowledge"
	pluginAPI "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop"
	resourceCommon "github.com/coze-dev/coze-studio/backend/api/model/resource/common"
	workflowAPI "github.com/coze-dev/coze-studio/backend/api/model/workflow"
	applicationApp "github.com/coze-dev/coze-studio/backend/application/app"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	knowledgeApp "github.com/coze-dev/coze-studio/backend/application/knowledge"
	"github.com/coze-dev/coze-studio/backend/application/memory"
	pluginApp "github.com/coze-dev/coze-studio/backend/application/plugin"
	searchApp "github.com/coze-dev/coze-studio/backend/application/search"
	singleagentApp "github.com/coze-dev/coze-studio/backend/application/singleagent"
	userApp "github.com/coze-dev/coze-studio/backend/application/user"
	workflowApp "github.com/coze-dev/coze-studio/backend/application/workflow"
	knowledgeModel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	pluginConsts "github.com/coze-dev/coze-studio/backend/crossdomain/plugin/consts"
	searchModel "github.com/coze-dev/coze-studio/backend/crossdomain/search/model"
	pluginDTO "github.com/coze-dev/coze-studio/backend/domain/plugin/dto"
	searchEntity "github.com/coze-dev/coze-studio/backend/domain/search/entity"
	userEntity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	userService "github.com/coze-dev/coze-studio/backend/domain/user/service"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/pkg/errorx"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/conv"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/types/consts"
	"github.com/coze-dev/coze-studio/backend/types/errno"
)

type CloneAccountRequest struct {
	Email    string
	Password string
}

type CloneAccountSummary struct {
	Spaces    int `json:"spaces"`
	Projects  int `json:"projects"`
	Bots      int `json:"bots"`
	Workflows int `json:"workflows"`
	Plugins   int `json:"plugins"`
	Knowledge int `json:"knowledge"`
	Databases int `json:"databases"`
}

type CloneAccountResponse struct {
	UserID  int64                `json:"user_id,string"`
	Summary *CloneAccountSummary `json:"summary,omitempty"`
}

type sourceAssets struct {
	Projects         []*searchEntity.ProjectDocument
	Bots             []*searchEntity.ProjectDocument
	LibraryWorkflows []*searchModel.ResourceDocument
	LibraryPlugins   []*searchModel.ResourceDocument
	LibraryKnowledge []*searchModel.ResourceDocument
	LibraryDatabases []*searchModel.ResourceDocument
	RequiredSpaceIDs map[int64]struct{}
}

func CloneCurrentAccount(ctx context.Context, req *CloneAccountRequest) (resp *CloneAccountResponse, err error) {
	var (
		targetUserID int64
		spaceIDMap   map[int64]int64
	)

	defer func() {
		if err == nil || targetUserID <= 0 {
			return
		}

		rollbackErr := rollbackClonedAccount(ctx, targetUserID, spaceIDMap)
		if rollbackErr == nil {
			return
		}

		logs.CtxErrorf(ctx, "[account_clone] rollback failed, targetUserID=%d err=%v", targetUserID, rollbackErr)
		err = fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
	}()

	if ctxutil.GetUIDFromCtx(ctx) == nil {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "session is required"))
	}

	if !userAppIsValidEmail(req.Email) {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "Invalid email"))
	}

	if strings.TrimSpace(req.Password) == "" {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "Password is required"))
	}

	sourceUserID := ctxutil.MustGetUIDFromCtx(ctx)
	logs.CtxInfof(ctx, "[account_clone] start clone, sourceUserID=%d targetEmail=%s", sourceUserID, req.Email)

	sourceUser, sourceSpaces, err := userApp.UserApplicationSVC.GetCloneSource(ctx, sourceUserID)
	if err != nil {
		logs.CtxErrorf(ctx, "[account_clone] get clone source failed, sourceUserID=%d err=%v", sourceUserID, err)
		return nil, err
	}
	if len(sourceSpaces) == 0 {
		return nil, errorx.New(errno.ErrUserResourceNotFound, errorx.KV("type", "space"), errorx.KV("id", fmt.Sprintf("%d", sourceUserID)))
	}

	assets, err := collectAssets(ctx, sourceUserID, sourceSpaces)
	if err != nil {
		logs.CtxErrorf(ctx, "[account_clone] collect assets failed, sourceUserID=%d err=%v", sourceUserID, err)
		return nil, err
	}
	logs.CtxInfof(ctx, "[account_clone] assets collected, sourceUserID=%d spaces=%d projects=%d bots=%d workflows=%d plugins=%d knowledge=%d databases=%d",
		sourceUserID, len(sourceSpaces), len(assets.Projects), len(assets.Bots), len(assets.LibraryWorkflows), len(assets.LibraryPlugins), len(assets.LibraryKnowledge), len(assets.LibraryDatabases))

	spacesToCopy := filterSpaces(sourceSpaces, assets.RequiredSpaceIDs)
	if len(spacesToCopy) == 0 {
		spacesToCopy = []*userEntity.Space{sourceSpaces[0]}
	}

	targetUserID, err = userApp.UserApplicationSVC.GenerateCloneTargetUserID(ctx)
	if err != nil {
		logs.CtxErrorf(ctx, "[account_clone] generate target user id failed, sourceUserID=%d err=%v", sourceUserID, err)
		return nil, err
	}

	spaceIDMap, err = userApp.UserApplicationSVC.CloneSpacesForUser(ctx, targetUserID, spacesToCopy)
	if err != nil {
		logs.CtxErrorf(ctx, "[account_clone] clone spaces failed, sourceUserID=%d targetUserID=%d err=%v", sourceUserID, targetUserID, err)
		return nil, err
	}
	logs.CtxInfof(ctx, "[account_clone] spaces cloned, sourceUserID=%d targetUserID=%d clonedSpaces=%d", sourceUserID, targetUserID, len(spaceIDMap))

	if _, err = userApp.UserApplicationSVC.DomainSVC.Create(ctx, &userService.CreateUserRequest{
		UserID:      targetUserID,
		SpaceID:     spaceIDMap[spacesToCopy[0].ID],
		Email:       req.Email,
		Password:    req.Password,
		Name:        sourceUser.Name,
		Description: sourceUser.Description,
		Locale:      sourceUser.Locale,
	}); err != nil {
		logs.CtxErrorf(ctx, "[account_clone] create target user failed, sourceUserID=%d targetUserID=%d err=%v", sourceUserID, targetUserID, err)
		return nil, err
	}
	logs.CtxInfof(ctx, "[account_clone] target user created, sourceUserID=%d targetUserID=%d", sourceUserID, targetUserID)

	if err = userApp.UserApplicationSVC.BindUserToClonedSpaces(ctx, targetUserID, spaceIDMap, spacesToCopy[0].ID); err != nil {
		logs.CtxErrorf(ctx, "[account_clone] bind cloned spaces failed, sourceUserID=%d targetUserID=%d err=%v", sourceUserID, targetUserID, err)
		return nil, err
	}

	if err = copyAssets(ctx, targetUserID, spaceIDMap, assets); err != nil {
		logs.CtxErrorf(ctx, "[account_clone] copy assets failed, sourceUserID=%d targetUserID=%d err=%v", sourceUserID, targetUserID, err)
		return nil, err
	}
	logs.CtxInfof(ctx, "[account_clone] clone completed, sourceUserID=%d targetUserID=%d", sourceUserID, targetUserID)

	resp = &CloneAccountResponse{
		UserID: targetUserID,
		Summary: &CloneAccountSummary{
			Spaces:    len(spaceIDMap),
			Projects:  len(assets.Projects),
			Bots:      len(assets.Bots),
			Workflows: len(assets.LibraryWorkflows),
			Plugins:   len(assets.LibraryPlugins),
			Knowledge: len(assets.LibraryKnowledge),
			Databases: len(assets.LibraryDatabases),
		},
	}

	return resp, nil
}

func rollbackClonedAccount(ctx context.Context, targetUserID int64, spaceIDMap map[int64]int64) error {
	logs.CtxWarnf(ctx, "[account_clone] start rollback, targetUserID=%d clonedSpaces=%d", targetUserID, len(spaceIDMap))

	targetSpaceIDs := make([]int64, 0, len(spaceIDMap))
	for _, targetSpaceID := range spaceIDMap {
		targetSpaceIDs = append(targetSpaceIDs, targetSpaceID)
	}

	rollbackCtx := cloneAccountContextForUser(ctx, targetUserID)
	rollbackErrs := make([]error, 0)

	if len(targetSpaceIDs) > 0 {
		if err := rollbackClonedBots(rollbackCtx, targetUserID, targetSpaceIDs); err != nil {
			rollbackErrs = append(rollbackErrs, err)
		}
		if err := rollbackClonedProjects(rollbackCtx, targetUserID, targetSpaceIDs); err != nil {
			rollbackErrs = append(rollbackErrs, err)
		}
		if err := rollbackClonedWorkflows(rollbackCtx, targetUserID, targetSpaceIDs); err != nil {
			rollbackErrs = append(rollbackErrs, err)
		}
		if err := rollbackClonedPlugins(rollbackCtx, targetUserID, targetSpaceIDs); err != nil {
			rollbackErrs = append(rollbackErrs, err)
		}
		if err := rollbackClonedKnowledge(rollbackCtx, targetUserID, targetSpaceIDs); err != nil {
			rollbackErrs = append(rollbackErrs, err)
		}
		if err := rollbackClonedDatabases(rollbackCtx, targetUserID, targetSpaceIDs); err != nil {
			rollbackErrs = append(rollbackErrs, err)
		}
	}

	if err := userApp.UserApplicationSVC.RollbackCloneTarget(ctx, targetUserID); err != nil {
		rollbackErrs = append(rollbackErrs, err)
	}

	return errors.Join(rollbackErrs...)
}

func cloneAccountContextForUser(ctx context.Context, userID int64) context.Context {
	cloneCtx := ctxcache.Init(ctx)
	ctxcache.Store(cloneCtx, consts.SessionDataKeyInCtx, &userEntity.Session{UserID: userID})
	return cloneCtx
}

func rollbackClonedBots(ctx context.Context, targetUserID int64, targetSpaceIDs []int64) error {
	errList := make([]error, 0)
	for _, targetSpaceID := range targetSpaceIDs {
		bots, err := searchProjectsByType(ctx, targetUserID, targetSpaceID, intelligenceCommon.IntelligenceType_Bot)
		if err != nil {
			errList = append(errList, fmt.Errorf("search bots failed, targetSpaceID=%d: %w", targetSpaceID, err))
			continue
		}
		for _, bot := range bots {
			if _, err := singleagentApp.SingleAgentSVC.DeleteAgentDraft(ctx, &developerAPI.DeleteDraftBotRequest{
				SpaceID: targetSpaceID,
				BotID:   bot.ID,
			}); err != nil {
				errList = append(errList, fmt.Errorf("delete bot failed, targetSpaceID=%d botID=%d: %w", targetSpaceID, bot.ID, err))
			}
		}
	}

	return errors.Join(errList...)
}

func rollbackClonedProjects(ctx context.Context, targetUserID int64, targetSpaceIDs []int64) error {
	errList := make([]error, 0)
	for _, targetSpaceID := range targetSpaceIDs {
		projects, err := searchProjectsByType(ctx, targetUserID, targetSpaceID, intelligenceCommon.IntelligenceType_Project)
		if err != nil {
			errList = append(errList, fmt.Errorf("search projects failed, targetSpaceID=%d: %w", targetSpaceID, err))
			continue
		}
		for _, project := range projects {
			if _, err := applicationApp.APPApplicationSVC.DraftProjectDelete(ctx, &projectAPI.DraftProjectDeleteRequest{ProjectID: project.ID}); err != nil {
				errList = append(errList, fmt.Errorf("delete project failed, projectID=%d: %w", project.ID, err))
			}
		}
	}

	return errors.Join(errList...)
}

func rollbackClonedWorkflows(ctx context.Context, targetUserID int64, targetSpaceIDs []int64) error {
	return rollbackClonedLibraryResources(ctx, targetUserID, targetSpaceIDs, resourceCommon.ResType_Workflow, func(spaceID int64, res *searchModel.ResourceDocument) error {
		_, err := workflowApp.SVC.DeleteWorkflow(ctx, &workflowAPI.DeleteWorkflowRequest{
			WorkflowID: conv.Int64ToStr(res.ResID),
			SpaceID:    conv.Int64ToStr(spaceID),
		})
		return err
	})
}

func rollbackClonedPlugins(ctx context.Context, targetUserID int64, targetSpaceIDs []int64) error {
	return rollbackClonedLibraryResources(ctx, targetUserID, targetSpaceIDs, resourceCommon.ResType_Plugin, func(_ int64, res *searchModel.ResourceDocument) error {
		_, err := pluginApp.PluginApplicationSVC.DelPlugin(ctx, &pluginAPI.DelPluginRequest{PluginID: res.ResID})
		return err
	})
}

func rollbackClonedKnowledge(ctx context.Context, targetUserID int64, targetSpaceIDs []int64) error {
	return rollbackClonedLibraryResources(ctx, targetUserID, targetSpaceIDs, resourceCommon.ResType_Knowledge, func(_ int64, res *searchModel.ResourceDocument) error {
		_, err := knowledgeApp.KnowledgeSVC.DeleteKnowledge(ctx, &knowledgeDataset.DeleteDatasetRequest{DatasetID: res.ResID})
		return err
	})
}

func rollbackClonedDatabases(ctx context.Context, targetUserID int64, targetSpaceIDs []int64) error {
	return rollbackClonedLibraryResources(ctx, targetUserID, targetSpaceIDs, resourceCommon.ResType_Database, func(_ int64, res *searchModel.ResourceDocument) error {
		_, err := memory.DatabaseApplicationSVC.DeleteDatabase(ctx, &table.DeleteDatabaseRequest{ID: res.ResID})
		return err
	})
}

func rollbackClonedLibraryResources(
	ctx context.Context,
	targetUserID int64,
	targetSpaceIDs []int64,
	resType resourceCommon.ResType,
	deleteFn func(spaceID int64, res *searchModel.ResourceDocument) error,
) error {
	errList := make([]error, 0)
	for _, targetSpaceID := range targetSpaceIDs {
		resources, err := searchLibraryResources(ctx, targetUserID, targetSpaceID, resType)
		if err != nil {
			errList = append(errList, fmt.Errorf("search resources failed, resType=%d targetSpaceID=%d: %w", resType, targetSpaceID, err))
			continue
		}
		for _, res := range resources {
			if err := deleteFn(targetSpaceID, res); err != nil {
				errList = append(errList, fmt.Errorf("delete resource failed, resType=%d resourceID=%d targetSpaceID=%d: %w", resType, res.ResID, targetSpaceID, err))
			}
		}
	}

	return errors.Join(errList...)
}

func collectAssets(ctx context.Context, sourceUserID int64, sourceSpaces []*userEntity.Space) (*sourceAssets, error) {
	assets := &sourceAssets{RequiredSpaceIDs: make(map[int64]struct{})}
	for _, sourceSpace := range sourceSpaces {
		projects, err := searchProjectsByType(ctx, sourceUserID, sourceSpace.ID, intelligenceCommon.IntelligenceType_Project)
		if err != nil {
			return nil, err
		}
		assets.Projects = append(assets.Projects, projects...)

		bots, err := searchProjectsByType(ctx, sourceUserID, sourceSpace.ID, intelligenceCommon.IntelligenceType_Bot)
		if err != nil {
			return nil, err
		}
		assets.Bots = append(assets.Bots, bots...)

		plugins, err := searchLibraryResources(ctx, sourceUserID, sourceSpace.ID, resourceCommon.ResType_Plugin)
		if err != nil {
			return nil, err
		}
		assets.LibraryPlugins = append(assets.LibraryPlugins, plugins...)

		knowledge, err := searchLibraryResources(ctx, sourceUserID, sourceSpace.ID, resourceCommon.ResType_Knowledge)
		if err != nil {
			return nil, err
		}
		assets.LibraryKnowledge = append(assets.LibraryKnowledge, knowledge...)

		databases, err := searchLibraryResources(ctx, sourceUserID, sourceSpace.ID, resourceCommon.ResType_Database)
		if err != nil {
			return nil, err
		}
		assets.LibraryDatabases = append(assets.LibraryDatabases, databases...)

		flows, err := searchLibraryResources(ctx, sourceUserID, sourceSpace.ID, resourceCommon.ResType_Workflow)
		if err != nil {
			return nil, err
		}
		assets.LibraryWorkflows = append(assets.LibraryWorkflows, flows...)
	}

	for _, project := range assets.Projects {
		assets.RequiredSpaceIDs[project.GetSpaceID()] = struct{}{}
	}
	for _, resource := range assets.LibraryPlugins {
		if resource.SpaceID != nil {
			assets.RequiredSpaceIDs[*resource.SpaceID] = struct{}{}
		}
	}
	for _, resource := range assets.LibraryKnowledge {
		if resource.SpaceID != nil {
			assets.RequiredSpaceIDs[*resource.SpaceID] = struct{}{}
		}
	}
	for _, resource := range assets.LibraryDatabases {
		if resource.SpaceID != nil {
			assets.RequiredSpaceIDs[*resource.SpaceID] = struct{}{}
		}
	}
	for _, resource := range assets.LibraryWorkflows {
		if resource.SpaceID != nil {
			assets.RequiredSpaceIDs[*resource.SpaceID] = struct{}{}
		}
	}
	for _, bot := range assets.Bots {
		assets.RequiredSpaceIDs[bot.GetSpaceID()] = struct{}{}
	}

	return assets, nil
}

func searchProjectsByType(ctx context.Context, sourceUserID, sourceSpaceID int64, projectType intelligenceCommon.IntelligenceType) ([]*searchEntity.ProjectDocument, error) {
	var result []*searchEntity.ProjectDocument
	cursor := ""
	for {
		resp, err := searchApp.SearchSVC.DomainSVC.SearchProjects(ctx, &searchEntity.SearchProjectsRequest{
			SpaceID: sourceSpaceID,
			OwnerID: sourceUserID,
			Types:   []intelligenceCommon.IntelligenceType{projectType},
			Limit:   200,
			Cursor:  cursor,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, resp.Data...)
		if !resp.HasMore {
			break
		}
		cursor = resp.NextCursor
	}
	return result, nil
}

func searchLibraryResources(ctx context.Context, sourceUserID, sourceSpaceID int64, resType resourceCommon.ResType) ([]*searchModel.ResourceDocument, error) {
	var result []*searchModel.ResourceDocument
	cursor := ""
	for {
		resp, err := searchApp.SearchSVC.DomainSVC.SearchResources(ctx, &searchEntity.SearchResourcesRequest{
			SpaceID:       sourceSpaceID,
			OwnerID:       sourceUserID,
			APPID:         0,
			ResTypeFilter: []resourceCommon.ResType{resType},
			Limit:         200,
			Cursor:        cursor,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, resp.Data...)
		if !resp.HasMore {
			break
		}
		cursor = resp.NextCursor
	}
	return result, nil
}

func copyAssets(ctx context.Context, targetUserID int64, spaceIDMap map[int64]int64, assets *sourceAssets) error {
	knowledgeIDMap := make(map[int64]int64)
	workflowIDMap := make(map[int64]int64)

	for _, project := range assets.Projects {
		targetSpaceID, ok := spaceIDMap[project.GetSpaceID()]
		if !ok {
			return fmt.Errorf("target space not found for project=%d sourceSpace=%d", project.ID, project.GetSpaceID())
		}
		if _, err := applicationApp.APPApplicationSVC.CloneDraftProjectToUser(ctx, project.ID, targetUserID, targetSpaceID); err != nil {
			return fmt.Errorf("clone project failed, projectID=%d sourceSpaceID=%d targetSpaceID=%d: %w", project.ID, project.GetSpaceID(), targetSpaceID, err)
		}
	}

	for _, plugin := range assets.LibraryPlugins {
		if plugin.SpaceID == nil {
			return fmt.Errorf("plugin %d missing source space", plugin.ResID)
		}
		targetSpaceID := spaceIDMap[*plugin.SpaceID]
		if _, err := pluginApp.PluginApplicationSVC.CopyPlugin(ctx, &pluginDTO.CopyPluginRequest{
			PluginID:      plugin.ResID,
			UserID:        targetUserID,
			CopyScene:     pluginConsts.CopySceneOfToLibrary,
			TargetSpaceID: ptr.Of(targetSpaceID),
		}); err != nil {
			return fmt.Errorf("copy library plugin failed, pluginID=%d sourceSpaceID=%d targetSpaceID=%d: %w", plugin.ResID, *plugin.SpaceID, targetSpaceID, err)
		}
	}

	for _, knowledge := range assets.LibraryKnowledge {
		if knowledge.SpaceID == nil {
			return fmt.Errorf("knowledge %d missing source space", knowledge.ResID)
		}
		targetSpaceID := spaceIDMap[*knowledge.SpaceID]
		if _, err := cloneKnowledgeIfNeeded(ctx, knowledge.ResID, targetUserID, targetSpaceID, knowledgeIDMap); err != nil {
			return fmt.Errorf("copy library knowledge failed, knowledgeID=%d sourceSpaceID=%d targetSpaceID=%d: %w", knowledge.ResID, *knowledge.SpaceID, targetSpaceID, err)
		}
	}

	for _, database := range assets.LibraryDatabases {
		if database.SpaceID == nil {
			return fmt.Errorf("database %d missing source space", database.ResID)
		}
		targetSpaceID := spaceIDMap[*database.SpaceID]
		if _, err := memory.DatabaseApplicationSVC.CopyDatabase(ctx, &memory.CopyDatabaseRequest{
			DatabaseIDs:   []int64{database.ResID},
			TableType:     table.TableType_OnlineTable,
			CreatorID:     targetUserID,
			IsCopyData:    true,
			TargetSpaceID: ptr.Of(targetSpaceID),
			TargetAppID:   0,
			Suffix:        ptr.Of(""),
		}); err != nil {
			return fmt.Errorf("copy library database failed, databaseID=%d sourceSpaceID=%d targetSpaceID=%d: %w", database.ResID, *database.SpaceID, targetSpaceID, err)
		}
	}

	for _, workflow := range assets.LibraryWorkflows {
		if workflow.SpaceID == nil {
			return fmt.Errorf("workflow %d missing source space", workflow.ResID)
		}
		targetSpaceID := spaceIDMap[*workflow.SpaceID]
		if _, err := cloneLibraryWorkflowIfNeeded(ctx, workflow.ResID, targetUserID, targetSpaceID, workflowIDMap); err != nil {
			return fmt.Errorf("copy library workflow failed, workflowID=%d sourceSpaceID=%d targetSpaceID=%d: %w", workflow.ResID, *workflow.SpaceID, targetSpaceID, err)
		}
	}

	for _, bot := range assets.Bots {
		targetSpaceID, ok := spaceIDMap[bot.GetSpaceID()]
		if !ok {
			return fmt.Errorf("target space not found for bot=%d sourceSpace=%d", bot.ID, bot.GetSpaceID())
		}
		if err := cloneBotToUser(ctx, bot.ID, targetUserID, targetSpaceID, knowledgeIDMap, workflowIDMap); err != nil {
			return fmt.Errorf("clone bot failed, botID=%d sourceSpaceID=%d targetSpaceID=%d: %w", bot.ID, bot.GetSpaceID(), targetSpaceID, err)
		}
	}

	return nil
}

func filterSpaces(sourceSpaces []*userEntity.Space, requiredSpaceIDs map[int64]struct{}) []*userEntity.Space {
	if len(requiredSpaceIDs) == 0 {
		return nil
	}
	spaces := make([]*userEntity.Space, 0, len(requiredSpaceIDs))
	for _, sourceSpace := range sourceSpaces {
		if _, ok := requiredSpaceIDs[sourceSpace.ID]; ok {
			spaces = append(spaces, sourceSpace)
		}
	}
	return spaces
}

func userAppIsValidEmail(email string) bool {
	return strings.Contains(email, "@")
}

func cloneLibraryWorkflowIfNeeded(ctx context.Context, workflowID, targetUserID, targetSpaceID int64, workflowIDMap map[int64]int64) (int64, error) {
	if newWorkflowID, ok := workflowIDMap[workflowID]; ok {
		return newWorkflowID, nil
	}

	newWorkflowID, err := workflowApp.SVC.CloneLibraryWorkflowToSpace(ctx, workflowID, targetUserID, targetSpaceID)
	if err != nil {
		return 0, err
	}

	workflowIDMap[workflowID] = newWorkflowID
	return newWorkflowID, nil
}

func cloneKnowledgeIfNeeded(ctx context.Context, knowledgeID, targetUserID, targetSpaceID int64, knowledgeIDMap map[int64]int64) (int64, error) {
	if newKnowledgeID, ok := knowledgeIDMap[knowledgeID]; ok {
		return newKnowledgeID, nil
	}

	response, err := knowledgeApp.KnowledgeSVC.CopyKnowledge(ctx, &knowledgeModel.CopyKnowledgeRequest{
		KnowledgeID:   knowledgeID,
		TargetUserID:  targetUserID,
		TargetSpaceID: targetSpaceID,
		TaskUniqKey:   uuid.NewString(),
	})
	if err != nil {
		return 0, err
	}

	knowledgeIDMap[knowledgeID] = response.TargetKnowledgeID
	return response.TargetKnowledgeID, nil
}

func cloneBotToUser(ctx context.Context, botID, targetUserID, targetSpaceID int64, knowledgeIDMap, workflowIDMap map[int64]int64) error {
	sourceAgent, newAgent, err := singleagentApp.SingleAgentSVC.PrepareDraftBotClone(ctx, botID, targetSpaceID, targetUserID)
	if err != nil {
		return fmt.Errorf("prepare bot clone failed: %w", err)
	}

	if err = singleagentApp.SingleAgentSVC.CloneDraftBotVariables(ctx, sourceAgent, newAgent); err != nil {
		return fmt.Errorf("clone bot variables failed: %w", err)
	}

	pluginIDMap := make(map[int64]int64)
	toolIDMap := make(map[int64]int64)

	toolInfos, err := singleagentApp.SingleAgentSVC.GetDraftBotToolInfos(ctx, sourceAgent)
	if err != nil {
		return err
	}

	for _, toolInfo := range toolInfos {
		if toolInfo.GetPluginFrom() == bot_common.PluginFrom_FromSaas {
			continue
		}
		if _, ok := pluginIDMap[toolInfo.PluginID]; ok {
			continue
		}
		response, err := pluginApp.PluginApplicationSVC.CopyPlugin(ctx, &pluginDTO.CopyPluginRequest{
			PluginID:      toolInfo.PluginID,
			UserID:        targetUserID,
			CopyScene:     pluginConsts.CopySceneOfAPPDuplicate,
			TargetSpaceID: ptr.Of(targetSpaceID),
		})
		if err != nil {
			var statusErr errorx.StatusError
			if errors.As(err, &statusErr) && statusErr.Code() == errno.ErrPluginRecordNotFound {
				logs.CtxWarnf(ctx, "[account_clone] skip bot plugin copy, plugin draft not found, pluginID=%d source=%s", toolInfo.PluginID, toolInfo.GetPluginFrom().String())
				continue
			}
			return fmt.Errorf("copy bot plugin failed, pluginID=%d: %w", toolInfo.PluginID, err)
		}
		pluginIDMap[toolInfo.PluginID] = response.Plugin.ID
		for oldToolID, newTool := range response.Tools {
			toolIDMap[oldToolID] = newTool.ID
		}
	}

	if sourceAgent.Knowledge != nil {
		copiedKnowledge := *sourceAgent.Knowledge
		copiedItems := make([]*bot_common.KnowledgeInfo, 0, len(sourceAgent.Knowledge.KnowledgeInfo))
		for _, info := range sourceAgent.Knowledge.KnowledgeInfo {
			if info == nil || info.GetId() == "" {
				continue
			}
			knowledgeID, err := conv.StrToInt64(info.GetId())
			if err != nil {
				return err
			}
			newKnowledgeID, err := cloneKnowledgeIfNeeded(ctx, knowledgeID, targetUserID, targetSpaceID, knowledgeIDMap)
			if err != nil {
				return fmt.Errorf("copy bot knowledge failed, knowledgeID=%d: %w", knowledgeID, err)
			}
			copiedInfo := *info
			copiedInfo.Id = ptr.Of(conv.Int64ToStr(newKnowledgeID))
			copiedItems = append(copiedItems, &copiedInfo)
		}
		copiedKnowledge.KnowledgeInfo = copiedItems
		newAgent.Knowledge = &copiedKnowledge
	}

	if len(sourceAgent.Database) > 0 {
		databaseIDs := make([]int64, 0, len(sourceAgent.Database))
		for _, database := range sourceAgent.Database {
			if database == nil || database.GetTableId() == "" {
				continue
			}
			databaseID, err := conv.StrToInt64(database.GetTableId())
			if err != nil {
				return err
			}
			databaseIDs = append(databaseIDs, databaseID)
		}
		if len(databaseIDs) > 0 {
			response, err := memory.DatabaseApplicationSVC.CopyDatabase(ctx, &memory.CopyDatabaseRequest{
				DatabaseIDs:   databaseIDs,
				TableType:     table.TableType_OnlineTable,
				CreatorID:     targetUserID,
				IsCopyData:    true,
				TargetSpaceID: ptr.Of(targetSpaceID),
			})
			if err != nil {
				return fmt.Errorf("copy bot databases failed, databaseIDs=%v: %w", databaseIDs, err)
			}
			copiedDatabases := make([]*bot_common.Database, 0, len(sourceAgent.Database))
			for _, database := range sourceAgent.Database {
				if database == nil {
					continue
				}
				copiedDatabase := *database
				if newDatabase, ok := response.Databases[conv.StrToInt64D(database.GetTableId(), 0)]; ok {
					newDatabaseID := conv.Int64ToStr(newDatabase.ID)
					copiedDatabase.TableId = ptr.Of(newDatabaseID)
				}
				copiedDatabases = append(copiedDatabases, &copiedDatabase)
			}
			newAgent.Database = copiedDatabases
		}
	}

	if len(sourceAgent.Workflow) > 0 {
		copiedWorkflows := make([]*bot_common.WorkflowInfo, 0, len(sourceAgent.Workflow))
		for _, workflowInfo := range sourceAgent.Workflow {
			if workflowInfo == nil || workflowInfo.GetWorkflowId() <= 0 {
				continue
			}
			newWorkflowID, err := cloneLibraryWorkflowIfNeeded(ctx, workflowInfo.GetWorkflowId(), targetUserID, targetSpaceID, workflowIDMap)
			if err != nil {
				return fmt.Errorf("copy bot workflow failed, workflowID=%d: %w", workflowInfo.GetWorkflowId(), err)
			}
			copiedWorkflow := *workflowInfo
			copiedWorkflow.WorkflowId = ptr.Of(newWorkflowID)
			copiedWorkflows = append(copiedWorkflows, &copiedWorkflow)
		}
		newAgent.Workflow = copiedWorkflows
	}

	if len(sourceAgent.Plugin) > 0 {
		updatedPlugins := make([]*bot_common.PluginInfo, 0, len(sourceAgent.Plugin))
		for _, pluginInfo := range sourceAgent.Plugin {
			if pluginInfo == nil {
				continue
			}
			copiedPlugin := *pluginInfo
			if newPluginID, ok := pluginIDMap[pluginInfo.GetPluginId()]; ok {
				copiedPlugin.PluginId = ptr.Of(newPluginID)
			}
			if newToolID, ok := toolIDMap[pluginInfo.GetApiId()]; ok {
				copiedPlugin.ApiId = ptr.Of(newToolID)
			}
			updatedPlugins = append(updatedPlugins, &copiedPlugin)
		}
		newAgent.Plugin = updatedPlugins
	}

	if err = singleagentApp.SingleAgentSVC.CloneDraftBotShortcutCommands(ctx, sourceAgent, newAgent, workflowIDMap, pluginIDMap, toolIDMap); err != nil {
		return fmt.Errorf("clone bot shortcut commands failed: %w", err)
	}

	if err = singleagentApp.SingleAgentSVC.SaveClonedDraftBot(ctx, newAgent); err != nil {
		return fmt.Errorf("save cloned bot failed: %w", err)
	}

	return nil
}
