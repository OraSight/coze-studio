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

package singleagent

import (
	"context"

	"github.com/coze-dev/coze-studio/backend/api/model/app/bot_common"
	intelligence "github.com/coze-dev/coze-studio/backend/api/model/app/intelligence/common"
	"github.com/coze-dev/coze-studio/backend/api/model/data/variable/project_memory"
	pluginModel "github.com/coze-dev/coze-studio/backend/crossdomain/plugin/model"
	"github.com/coze-dev/coze-studio/backend/domain/agent/singleagent/entity"
	pluginEntity "github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	searchEntity "github.com/coze-dev/coze-studio/backend/domain/search/entity"
	shortcutCMDEntity "github.com/coze-dev/coze-studio/backend/domain/shortcutcmd/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/conv"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/slices"
)

func (s *SingleAgentApplicationService) PrepareDraftBotClone(ctx context.Context, botID, targetSpaceID, targetUserID int64) (*entity.SingleAgent, *entity.SingleAgent, error) {
	draftAgent, err := s.ValidateAgentDraftAccess(ctx, botID)
	if err != nil {
		return nil, nil, err
	}

	newAgentID, err := s.appContext.IDGen.GenID(ctx)
	if err != nil {
		return nil, nil, err
	}

	newAgent, err := s.DomainSVC.DuplicateInMemory(ctx, &entity.DuplicateInfo{
		NewAgentID: newAgentID,
		SpaceID:    targetSpaceID,
		UserID:     targetUserID,
		DraftAgent: draftAgent,
	})
	if err != nil {
		return nil, nil, err
	}

	newAgent.Name = draftAgent.Name

	return draftAgent, newAgent, nil
}

func (s *SingleAgentApplicationService) CloneDraftBotVariables(ctx context.Context, oldAgent, newAgent *entity.SingleAgent) error {
	if oldAgent.VariablesMetaID == nil || *oldAgent.VariablesMetaID <= 0 {
		return nil
	}

	vars, err := s.appContext.VariablesDomainSVC.GetVariableMetaByID(ctx, *oldAgent.VariablesMetaID)
	if err != nil {
		return err
	}

	vars.ID = 0
	vars.BizID = conv.Int64ToStr(newAgent.AgentID)
	vars.BizType = project_memory.VariableConnector_Bot
	vars.Version = ""
	vars.CreatorID = newAgent.CreatorID

	varMetaID, err := s.appContext.VariablesDomainSVC.UpsertMeta(ctx, vars)
	if err != nil {
		return err
	}

	newAgent.VariablesMetaID = &varMetaID
	return nil
}

func (s *SingleAgentApplicationService) GetDraftBotToolInfos(ctx context.Context, agentInfo *entity.SingleAgent) ([]*pluginEntity.ToolInfo, error) {
	if len(agentInfo.Plugin) == 0 {
		return nil, nil
	}

	return s.appContext.PluginDomainSVC.MGetAgentTools(ctx, &pluginModel.MGetAgentToolsRequest{
		SpaceID: agentInfo.SpaceID,
		AgentID: agentInfo.AgentID,
		IsDraft: true,
		VersionAgentTools: slices.Transform(agentInfo.Plugin, func(info *bot_common.PluginInfo) pluginModel.VersionAgentTool {
			return pluginModel.VersionAgentTool{
				ToolID:     info.GetApiId(),
				PluginID:   info.GetPluginId(),
				PluginFrom: info.PluginFrom,
			}
		}),
	})
}

func (s *SingleAgentApplicationService) CloneDraftBotShortcutCommands(ctx context.Context, oldAgent, newAgent *entity.SingleAgent, workflowIDMap, pluginIDMap, toolIDMap map[int64]int64) error {
	metas, err := s.appContext.ShortcutCMDDomainSVC.ListCMD(ctx, &shortcutCMDEntity.ListMeta{
		SpaceID:  oldAgent.SpaceID,
		ObjectID: oldAgent.AgentID,
		IsOnline: 0,
		CommandIDs: slices.Transform(oldAgent.ShortcutCommand, func(a string) int64 {
			return conv.StrToInt64D(a, 0)
		}),
	})
	if err != nil {
		return err
	}

	shortcutCommandIDs := make([]string, 0, len(metas))
	for _, meta := range metas {
		meta.ObjectID = newAgent.AgentID
		meta.CreatorID = newAgent.CreatorID
		if newWorkflowID, ok := workflowIDMap[meta.WorkFlowID]; ok {
			meta.WorkFlowID = newWorkflowID
		}
		if newPluginID, ok := pluginIDMap[meta.PluginID]; ok {
			meta.PluginID = newPluginID
		}
		if newToolID, ok := toolIDMap[meta.PluginToolID]; ok {
			meta.PluginToolID = newToolID
		}
		do, err := s.appContext.ShortcutCMDDomainSVC.CreateCMD(ctx, meta)
		if err != nil {
			return err
		}
		shortcutCommandIDs = append(shortcutCommandIDs, conv.Int64ToStr(do.CommandID))
	}

	newAgent.ShortcutCommand = shortcutCommandIDs
	return nil
}

func (s *SingleAgentApplicationService) SaveClonedDraftBot(ctx context.Context, newAgent *entity.SingleAgent) error {
	userID := newAgent.CreatorID
	if _, err := s.DomainSVC.CreateSingleAgentDraftWithID(ctx, userID, newAgent.AgentID, newAgent); err != nil {
		return err
	}
	if err := s.DomainSVC.UpdateSingleAgentDraft(ctx, newAgent); err != nil {
		return err
	}

	return s.appContext.EventBus.PublishProject(ctx, &searchEntity.ProjectDomainEvent{
		OpType: searchEntity.Created,
		Project: &searchEntity.ProjectDocument{
			Status:  intelligence.IntelligenceStatus_Using,
			Type:    intelligence.IntelligenceType_Bot,
			ID:      newAgent.AgentID,
			SpaceID: &newAgent.SpaceID,
			OwnerID: &userID,
			Name:    &newAgent.Name,
		},
	})
}
