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

package middleware

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

const fallbackForwardedPort = "30188"

func getForwardedHost(ctx *app.RequestContext) string {
	host := strings.TrimSpace(string(ctx.GetHeader("X-Forwarded-Host")))
	if host == "" {
		host = string(ctx.Host())
	}

	host = strings.TrimSpace(strings.Split(host, ",")[0])
	port := strings.TrimSpace(string(ctx.GetHeader("X-Forwarded-Port")))
	if strings.Contains(host, ":") {
		return host
	}
	if port == "" {
		port = fallbackForwardedPort
	}

	return host + ":" + port
}

func SetHostMW() app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		ctxcache.Store(c, consts.HostKeyInCtx, getForwardedHost(ctx))
		ctxcache.Store(c, consts.RequestSchemeKeyInCtx, string(ctx.GetRequest().Scheme()))
		ctx.Next(c)
	}
}
