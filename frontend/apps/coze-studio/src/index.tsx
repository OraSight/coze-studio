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

import { createRoot } from 'react-dom/client';
import { initI18nInstance } from '@coze-arch/i18n/raw';
import { dynamicImportMdBoxStyle } from '@coze-arch/bot-md-box-adapter/style';
import { pullFeatureFlags, type FEATURE_FLAGS } from '@coze-arch/bot-flags';

import { App } from './app';
import './global.less';
import './index.less';

const showAutoLoginLoading = () => {
  const root = document.getElementById('root');
  if (!root) {
    return;
  }

  root.innerHTML = `
    <div style="height: 100vh; display: flex; align-items: center; justify-content: center; background: #f7f8fa;">
      <div style="min-width: 220px; padding: 24px 28px; border-radius: 12px; background: #fff; box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08); text-align: center;">
        <div style="width: 28px; height: 28px; margin: 0 auto 12px; border: 3px solid #e5e6eb; border-top-color: #3370ff; border-radius: 50%; animation: coze-autologin-spin 0.8s linear infinite;"></div>
        <div style="color: #1d2129; font-size: 14px; font-weight: 500;">正在加载中，请稍候...</div>
      </div>
    </div>
    <style>
      @keyframes coze-autologin-spin {
        from { transform: rotate(0deg); }
        to { transform: rotate(360deg); }
      }
    </style>
  `;
};

const runAutoLogin = async () => {
  try {
    const url = new URL(window.location.href);
    const token = url.searchParams.get('token');
    const name = url.searchParams.get('name');
    const type = url.searchParams.get('type');
    if (token) {
      document.cookie = `token=${encodeURIComponent(token)}; path=/`;
    }

    showAutoLoginLoading();

    const res = await fetch('/core-api/autologin', {
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ res_name: name, res_type: type }),
    });

    if (res.ok) {
      const payload = await res
        .clone()
        .json()
        .catch(() => null);
      const responseUrl =
        payload && typeof payload.url === 'string' ? payload.url.trim() : '';
      if (responseUrl) {
        // 返回 '/' 表示留在本站，直接挂载应用，避免 replace('/') 整页重载死循环
        if (responseUrl === '/') {
          return true;
        }
        try {
          const target = new URL(responseUrl, window.location.origin);
          const here = new URL(window.location.href);
          const samePage =
            target.pathname === here.pathname && target.search === here.search;
          if (samePage) {
            return true;
          }
        } catch {
          // 非法 URL 仍尝试跳转
        }
        window.location.replace(responseUrl);
        return false;
      }
      const redirect = url.searchParams.get('redirect');
      if (redirect && redirect.startsWith('/')) {
        window.location.replace(redirect);
        return false;
      }
      window.location.replace('/');
      return false;
    }
  } catch {
    // Ignore autologin failure and continue bootstrap.
  }

  return true;
};

const initFlags = () => {
  pullFeatureFlags({
    timeout: 1000 * 4,
    fetchFeatureGating: () => Promise.resolve({} as unknown as FEATURE_FLAGS),
  });
};

const main = () => {
  // Initialize the value of the function switch
  initFlags();
  // Initialize i18n
  initI18nInstance({
    lng: (localStorage.getItem('i18next') ?? (IS_OVERSEA ? 'en' : 'zh-CN')) as
      | 'en'
      | 'zh-CN',
  });
  // Import mdbox styles dynamically
  dynamicImportMdBoxStyle();

  const $root = document.getElementById('root');
  if (!$root) {
    throw new Error('root element not found');
  }
  const root = createRoot($root);

  root.render(<App />);
};

void runAutoLogin().then(shouldBootstrap => {
  if (shouldBootstrap) {
    main();
  }
});
