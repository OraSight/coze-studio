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

/** 与 aes-dashboard 跳转 Coze 时 query 的 environment 一致 */
export const DEPLOY_ENVIRONMENT_COOKIE = 'environment';

const AUTOLOGIN_PATH_SITE02 = '/core-api/autologin';
const AUTOLOGIN_PATH_MINGHANG = '/core-api/minghang/autologin';

/** 按 environment 选择同源代理路径（nginx / devServer 各路径固定转发） */
export const resolveAutologinPath = (
  environment: string | null | undefined,
): string => {
  if (environment?.trim().toLowerCase() === 'minghang') {
    return AUTOLOGIN_PATH_MINGHANG;
  }
  return AUTOLOGIN_PATH_SITE02;
};

export const readEnvironmentCookie = (): string | null => {
  const parts = document.cookie.split(';');
  for (const part of parts) {
    const trimmed = part.trim();
    const eq = trimmed.indexOf('=');
    if (eq === -1) {
      continue;
    }
    const name = trimmed.slice(0, eq);
    if (name !== DEPLOY_ENVIRONMENT_COOKIE) {
      continue;
    }
    const value = trimmed.slice(eq + 1);
    return value ? decodeURIComponent(value) : null;
  }
  return null;
};

export const writeEnvironmentCookie = (environment: string) => {
  const value = encodeURIComponent(environment.trim());
  const maxAge = 60 * 60 * 24 * 365;
  document.cookie = `${DEPLOY_ENVIRONMENT_COOKIE}=${value}; path=/; max-age=${maxAge}; SameSite=Lax`;
};

/** 优先 URL query，写入 cookie；否则读 cookie（供代理侧读取） */
export const resolveActiveEnvironment = (url: URL): string | null => {
  const fromQuery = url.searchParams.get('environment')?.trim();
  if (fromQuery) {
    writeEnvironmentCookie(fromQuery);
    return fromQuery;
  }
  return readEnvironmentCookie();
};
